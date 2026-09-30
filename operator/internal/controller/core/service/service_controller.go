package service

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/loomservice"
)

const (
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "loom-operator"
)

// errNameConflict is returned when the generated Service name is already taken by
// a Service that does not belong to the loom Service being reconciled.
var errNameConflict = errors.New("kubernetes Service name is already in use")

// Reconciler reconciles a Service object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.loom.io,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.loom.io,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.loom.io,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile keeps a selectorless ClusterIP Kubernetes Service in the loom-system namespace for
// every loom Service. The Kubernetes Service is named "<name>--<namespace>", so the loom Service
// is reachable at <name>--<namespace>.loom-system.svc.cluster.local. Its EndpointSlices are
// maintained by the endpointslice controller.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	service := &corev1alpha1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Service resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Service")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	kubeServiceName := loomservice.KubeServiceName(service.Namespace, service.Name)

	if !service.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(service, corev1alpha1.ServiceFinalizer) {
			return ctrl.Result{}, nil
		}
		if err := r.deleteKubeService(ctx, service, kubeServiceName); err != nil {
			logger.Error(err, "Failed to delete Kubernetes Service", "service", kubeServiceName)
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(service, corev1alpha1.ServiceFinalizer)
		return ctrl.Result{}, r.Update(ctx, service)
	}

	if controllerutil.AddFinalizer(service, corev1alpha1.ServiceFinalizer) {
		if err := r.Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
	}

	if reason, message := validateService(service); reason != "" {
		logger.Info("Service is invalid", "reason", reason, "message", message)
		return ctrl.Result{}, r.updateStatus(ctx, service, nil, metav1.ConditionFalse, reason, message)
	}

	kubeService, err := r.ensureKubeService(ctx, service, kubeServiceName)
	if errors.Is(err, errNameConflict) {
		logger.Info("Kubernetes Service name is taken by another Service", "service", kubeServiceName)
		return ctrl.Result{}, r.updateStatus(ctx, service, nil, metav1.ConditionFalse, "NameConflict",
			fmt.Sprintf("Service %s/%s already exists and belongs to another loom Service",
				corev1alpha1.ServiceTargetNamespace, kubeServiceName))
	}
	if err != nil {
		logger.Error(err, "Failed to ensure Kubernetes Service", "service", kubeServiceName)
		_ = r.updateStatus(ctx, service, nil, metav1.ConditionFalse, "ServiceError", err.Error()) // best effort
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.updateStatus(ctx, service, kubeService, metav1.ConditionTrue, "ServiceCreated",
		fmt.Sprintf("Service is reachable at %s", loomservice.Hostname(kubeServiceName)))
}

// validateService returns a reason and message if the Service can't be translated into
// a Kubernetes Service. The CRD schema already rejects these, so this is purely defensive.
func validateService(service *corev1alpha1.Service) (string, string) {
	if service.Spec.Type != "" && service.Spec.Type != corev1alpha1.ServiceTypeClusterIP {
		return "UnsupportedType", fmt.Sprintf("Service type %q is not supported", service.Spec.Type)
	}
	if len(service.Spec.Ports) == 0 {
		return "NoPorts", "A ClusterIP Service requires at least one port"
	}
	return "", ""
}

// ensureKubeService creates or updates the Kubernetes Service backing the loom Service.
func (r *Reconciler) ensureKubeService(ctx context.Context, service *corev1alpha1.Service,
	kubeServiceName string) (*corev1.Service, error) {
	kubeService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeServiceName,
			Namespace: corev1alpha1.ServiceTargetNamespace,
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.Client, kubeService, func() error {
		if !kubeService.CreationTimestamp.IsZero() && !loomservice.IsOwnedBy(kubeService.Labels, service) {
			return errNameConflict
		}
		if kubeService.Labels == nil {
			kubeService.Labels = map[string]string{}
		}
		for key, value := range loomservice.OwnerLabels(service) {
			kubeService.Labels[key] = value
		}
		kubeService.Labels[managedByLabel] = managedByValue

		// Leave ClusterIP/ClusterIPs alone: they are allocated by the API server.
		ipFamilyPolicy := corev1.IPFamilyPolicyPreferDualStack
		kubeService.Spec.Type = corev1.ServiceTypeClusterIP
		kubeService.Spec.IPFamilyPolicy = &ipFamilyPolicy
		kubeService.Spec.Selector = nil
		kubeService.Spec.Ports = kubeServicePorts(service)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return kubeService, nil
}

func kubeServicePorts(service *corev1alpha1.Service) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Protocol:   protocol,
			Port:       port.Port,
			TargetPort: intstr.FromInt32(port.Port),
		})
	}
	return ports
}

// deleteKubeService deletes the Kubernetes Service backing the loom Service, unless it
// belongs to another loom Service.
func (r *Reconciler) deleteKubeService(ctx context.Context, service *corev1alpha1.Service,
	kubeServiceName string) error {
	kubeService := &corev1.Service{}
	key := client.ObjectKey{Namespace: corev1alpha1.ServiceTargetNamespace, Name: kubeServiceName}
	if err := r.Get(ctx, key, kubeService); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !loomservice.IsOwnedBy(kubeService.Labels, service) {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, kubeService,
		client.Preconditions{UID: &kubeService.UID}))
}

// updateStatus records the generated Service (if any) and the Ready condition in the loom Service status.
func (r *Reconciler) updateStatus(ctx context.Context, service *corev1alpha1.Service, kubeService *corev1.Service,
	status metav1.ConditionStatus, reason, message string) error {
	original := service.DeepCopy()

	service.Status.ObservedGeneration = service.Generation
	if kubeService != nil {
		service.Status.ServiceName = kubeService.Name
		service.Status.Hostname = loomservice.Hostname(kubeService.Name)
		service.Status.ClusterIPs = kubeService.Spec.ClusterIPs
	} else {
		service.Status.ServiceName = ""
		service.Status.Hostname = ""
		service.Status.ClusterIPs = nil
	}
	meta.SetStatusCondition(&service.Status.Conditions, metav1.Condition{
		Type:               corev1alpha1.ServiceConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: service.Generation,
	})

	if equality.Semantic.DeepEqual(original.Status, service.Status) {
		return nil
	}
	return r.Status().Patch(ctx, service, client.MergeFrom(original))
}

// mapKubeServiceToRequests enqueues the loom Service owning a generated Kubernetes Service.
func (r *Reconciler) mapKubeServiceToRequests(_ context.Context, obj client.Object) []reconcile.Request {
	key, ok := loomservice.OwnerKeyFromLabels(obj.GetLabels())
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: key}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Service{}).
		// Owner references can't cross namespaces, so generated Services are mapped back
		// to their loom Service through labels to repair drift and deletions.
		Watches(&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(r.mapKubeServiceToRequests),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				if obj.GetNamespace() != corev1alpha1.ServiceTargetNamespace {
					return false
				}
				_, ok := loomservice.OwnerKeyFromLabels(obj.GetLabels())
				return ok
			})),
		).
		Named("core-service").
		Complete(r)
}
