package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "builder/api/v1alpha1"
	"builder/pkg/events"
)

// NetworkFunctionReconciler reconciles a NetworkFunction object
type NetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Bus    *events.EventBus
}

type CNIArgs struct {
	AppType string `json:"app_type"`
	AppID   string `json:"app_id,omitempty"`
	NFID    string `json:"nf_id,omitempty"`
}

type CNIConfig struct {
	Name    string   `json:"name"`
	CNIArgs CNIArgs `json:"cni-args"`
}
func (c CNIConfig) String() string {
	return `{
		"name": "` + c.Name + `",
		"cni-args": {
			"app_type": "` + c.CNIArgs.AppType + `",
			"app_id": "` + c.CNIArgs.AppID + `",
			"nf_id": "` + c.CNIArgs.NFID + `"
		}
	}`
}

// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/finalizers,verbs=update

// RBAC permissions for Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunction object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	const finalizerName = "cache.desire6g.eu/finalizer"

	logger.Info("Reconciling NetworkFunction", "request", req)
	nf := &cachev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, nf); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NetworkFunction")
		return ctrl.Result{}, err
	}

	// Check if being deleted
	if !nf.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(nf.GetFinalizers(), finalizerName) {
			r.Bus.Publish(events.Event{
				Name:    events.EventNfPreDeleted,
				Payload: nf,
			})

			// Remove finalizer
			nf.SetFinalizers(removeString(nf.GetFinalizers(), finalizerName))
			if err := r.Update(ctx, nf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(nf.GetFinalizers(), finalizerName) {
		nf.SetFinalizers(append(nf.GetFinalizers(), finalizerName))
		if err := r.Update(ctx, nf); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Search for the found
	found := &appsv1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Deployment not found, create it
		dep := r.deploymentForNetworkFunction(nf)

		logger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err := r.Create(ctx, dep); err != nil {
			logger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10*time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Ensure the Deployment size matches the desired state
	size := nf.Spec.Replicas
	if *found.Spec.Replicas != *size {
		found.Spec.Replicas = size
		if err := r.Update(ctx, found); err != nil {
			logger.Error(err, "Failed to update Deployment size", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
			return ctrl.Result{}, err
		}
		// Requeue the request to ensure the correct state is achieved
		return ctrl.Result{Requeue: true}, nil
	}

	// Update NetworkFunction status to reflect that the Deployment is available
	nf.Status.AvailableReplicas = found.Status.AvailableReplicas
	if err := r.Status().Update(ctx, nf); err != nil {
		logger.Error(err, "Failed to update NetworkFunction status")
		return ctrl.Result{}, err
	}
	
	// All is well.
	// Publish creation/update event
	r.Bus.Publish(events.Event{
		Name:    events.EventNfPreUpdated,
		Payload: nf,
	})

	return ctrl.Result{}, nil
}

func (r *NetworkFunctionReconciler) deploymentForNetworkFunction(nf *cachev1alpha1.NetworkFunction) *appsv1.Deployment {	
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nf.Name,
			Namespace: nf.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: nf.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": nf.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": nf.Name,
					},
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks": "[" + CNIConfig{
							Name: "iml-cni",
							CNIArgs: CNIArgs{
								AppType: "network_function",
								NFID:   string(nf.UID),
							},
						}.String() + "]",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  nf.Name,
							Image: nf.Spec.Image,
						},
					},
				},
			},
		},
	}
	// Set the ownerRef for the Deployment, ensuring that the Deployment
	// will be deleted when the Busybox CR is deleted.
	controllerutil.SetControllerReference(nf, dep, r.Scheme)
	return dep
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NetworkFunction{}).
		Owns(&appsv1.Deployment{}).
		Named("networkfunction").
		Complete(r)
}
