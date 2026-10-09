package endpointslice

import (
	"context"
	"net/netip"
	"sort"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/loomservice"
	"github.com/mantra6g/iml/operator/pkg/util/ptr"
)

// Reconciler reconciles the EndpointSlices of the Kubernetes Services generated for loom Services.
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.loom.io,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete

// Reconcile publishes the loom-cni addresses of the pods selected by a loom Service as
// EndpointSlices of its generated Kubernetes Service in the loom-system namespace.
// One EndpointSlice is kept per address family.
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
	if !service.DeletionTimestamp.IsZero() {
		// EndpointSlices are garbage collected along with the Kubernetes Service that owns them.
		return ctrl.Result{}, nil
	}

	kubeService := &corev1.Service{}
	kubeServiceKey := client.ObjectKey{
		Namespace: corev1alpha1.ServiceTargetNamespace,
		Name:      loomservice.KubeServiceName(service.Namespace, service.Name),
	}
	if err := r.Get(ctx, kubeServiceKey, kubeService); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Kubernetes Service not created yet", "service", kubeServiceKey)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !loomservice.IsOwnedBy(kubeService.Labels, service) {
		logger.Info("Kubernetes Service belongs to another loom Service, skipping", "service", kubeServiceKey)
		return ctrl.Result{}, nil
	}

	pods, err := r.selectPods(ctx, service)
	if err != nil {
		logger.Error(err, "Failed to list pods selected by Service")
		return ctrl.Result{}, err
	}

	desired := desiredEndpointSlices(service, kubeService, pods)

	existing := &discoveryv1.EndpointSliceList{}
	if err := r.List(ctx, existing,
		client.InNamespace(corev1alpha1.ServiceTargetNamespace),
		client.MatchingLabels(managedSliceLabels(service, kubeService))); err != nil {
		return ctrl.Result{}, err
	}
	for i := range existing.Items {
		slice := &existing.Items[i]
		if _, ok := desired[slice.Name]; ok {
			continue
		}
		logger.V(1).Info("Deleting stale EndpointSlice", "endpointslice", slice.Name)
		if err := r.Delete(ctx, slice); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	for name, want := range desired {
		slice := &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: corev1alpha1.ServiceTargetNamespace},
		}
		_, err := controllerutil.CreateOrPatch(ctx, r.Client, slice, func() error {
			if slice.Labels == nil {
				slice.Labels = map[string]string{}
			}
			for key, value := range want.Labels {
				slice.Labels[key] = value
			}
			slice.AddressType = want.AddressType
			slice.Ports = want.Ports
			slice.Endpoints = want.Endpoints
			return controllerutil.SetControllerReference(kubeService, slice, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "Failed to ensure EndpointSlice", "endpointslice", name)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// selectPods returns the pods in the Service namespace matching its selector.
func (r *Reconciler) selectPods(ctx context.Context, service *corev1alpha1.Service) ([]corev1.Pod, error) {
	// An empty selector would match every pod in the namespace, so treat it as selecting none.
	if len(service.Spec.Selector) == 0 {
		return nil, nil
	}
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(service.Namespace),
		client.MatchingLabels(service.Spec.Selector)); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// desiredEndpointSlices builds one EndpointSlice per address family with the loom-cni addresses of the pods,
// keyed by name. Families without endpoints are left out. A single slice per family caps a Service at the
// 1000 endpoints an EndpointSlice can hold.
func desiredEndpointSlices(service *corev1alpha1.Service, kubeService *corev1.Service,
	pods []corev1.Pod) map[string]*discoveryv1.EndpointSlice {
	endpoints := map[discoveryv1.AddressType][]discoveryv1.Endpoint{}
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		status, ok := loomservice.LoomNetworkStatus(pod)
		if !ok {
			continue
		}
		for _, ip := range status.IPs {
			addr, err := parseAddr(ip)
			if err != nil {
				continue
			}
			addressType := discoveryv1.AddressTypeIPv6
			if addr.Is4() {
				addressType = discoveryv1.AddressTypeIPv4
			}
			endpoints[addressType] = append(endpoints[addressType], podEndpoint(pod, addr))
		}
	}

	slices := map[string]*discoveryv1.EndpointSlice{}
	for addressType, eps := range endpoints {
		sort.Slice(eps, func(i, j int) bool { return eps[i].Addresses[0] < eps[j].Addresses[0] })
		name := sliceName(kubeService.Name, addressType)
		slices[name] = &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: corev1alpha1.ServiceTargetNamespace,
				Labels:    managedSliceLabels(service, kubeService),
			},
			AddressType: addressType,
			Endpoints:   eps,
			Ports:       endpointPorts(service),
		}
	}
	return slices
}

func sliceName(kubeServiceName string, addressType discoveryv1.AddressType) string {
	if addressType == discoveryv1.AddressTypeIPv4 {
		return kubeServiceName + "-ipv4"
	}
	return kubeServiceName + "-ipv6"
}

// parseAddr parses an address as found in the network-status annotation, tolerating a prefix length.
func parseAddr(ip string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(ip); err == nil {
		return addr.Unmap(), nil
	}
	prefix, err := netip.ParsePrefix(ip)
	if err != nil {
		return netip.Addr{}, err
	}
	return prefix.Addr().Unmap(), nil
}

func podEndpoint(pod *corev1.Pod, addr netip.Addr) discoveryv1.Endpoint {
	ready := isPodReady(pod)
	terminating := pod.DeletionTimestamp != nil
	endpoint := discoveryv1.Endpoint{
		Addresses: []string{addr.String()},
		Conditions: discoveryv1.EndpointConditions{
			Ready:       ptr.To(ready && !terminating),
			Serving:     ptr.To(ready),
			Terminating: ptr.To(terminating),
		},
		TargetRef: &corev1.ObjectReference{
			Kind:      "Pod",
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
	}
	if pod.Spec.NodeName != "" {
		endpoint.NodeName = ptr.To(pod.Spec.NodeName)
	}
	return endpoint
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func endpointPorts(service *corev1alpha1.Service) []discoveryv1.EndpointPort {
	ports := make([]discoveryv1.EndpointPort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		ports = append(ports, discoveryv1.EndpointPort{
			Name:     ptr.To(port.Name),
			Protocol: ptr.To(protocol),
			Port:     ptr.To(port.Port),
		})
	}
	return ports
}

// managedSliceLabels returns the labels of the EndpointSlices this controller manages for a Service.
func managedSliceLabels(service *corev1alpha1.Service, kubeService *corev1.Service) map[string]string {
	sliceLabels := loomservice.OwnerLabels(service)
	sliceLabels[discoveryv1.LabelServiceName] = kubeService.Name
	sliceLabels[discoveryv1.LabelManagedBy] = corev1alpha1.EndpointSliceManagedBy
	return sliceLabels
}

// enqueueForPod enqueues every loom Service in the pod namespace whose selector matches the pod.
func (r *Reconciler) enqueueForPod(ctx context.Context, pod client.Object,
	queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	services := &corev1alpha1.ServiceList{}
	if err := r.List(ctx, services, client.InNamespace(pod.GetNamespace())); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list Services for pod", "pod", client.ObjectKeyFromObject(pod))
		return
	}
	podLabels := labels.Set(pod.GetLabels())
	for i := range services.Items {
		selector := services.Items[i].Spec.Selector
		if len(selector) == 0 || !labels.SelectorFromSet(selector).Matches(podLabels) {
			continue
		}
		queue.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&services.Items[i])})
	}
}

// podHandler enqueues the Services selecting a pod. Updates enqueue the Services matching either the old or
// the new pod, so a pod whose labels stop matching gets removed from its former Services.
func (r *Reconciler) podHandler() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent,
			queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.enqueueForPod(ctx, e.Object, queue)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent,
			queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.enqueueForPod(ctx, e.ObjectOld, queue)
			r.enqueueForPod(ctx, e.ObjectNew, queue)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent,
			queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.enqueueForPod(ctx, e.Object, queue)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent,
			queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.enqueueForPod(ctx, e.Object, queue)
		},
	}
}

// podPredicate filters out pods that can't be endpoints (no network-status annotation) and
// updates that don't affect the published endpoints.
func podPredicate() predicate.Predicate {
	hasNetworkStatus := func(obj client.Object) bool {
		_, ok := obj.GetAnnotations()[netdefv1.NetworkStatusAnnot]
		return ok
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return hasNetworkStatus(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasNetworkStatus(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return hasNetworkStatus(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !hasNetworkStatus(e.ObjectOld) && !hasNetworkStatus(e.ObjectNew) {
				return false
			}
			oldPod, okOld := e.ObjectOld.(*corev1.Pod)
			newPod, okNew := e.ObjectNew.(*corev1.Pod)
			if !okOld || !okNew {
				return true
			}
			return !labels.Equals(oldPod.Labels, newPod.Labels) ||
				oldPod.Annotations[netdefv1.NetworkStatusAnnot] != newPod.Annotations[netdefv1.NetworkStatusAnnot] ||
				isPodReady(oldPod) != isPodReady(newPod) ||
				(oldPod.DeletionTimestamp == nil) != (newPod.DeletionTimestamp == nil) ||
				oldPod.Status.Phase != newPod.Status.Phase ||
				oldPod.Spec.NodeName != newPod.Spec.NodeName
		},
	}
}

// mapOwnedToRequests enqueues the loom Service referenced by the labels of a generated object.
func mapOwnedToRequests(_ context.Context, obj client.Object) []reconcile.Request {
	key, ok := loomservice.OwnerKeyFromLabels(obj.GetLabels())
	if !ok {
		return nil
	}
	return []reconcile.Request{{NamespacedName: key}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	inLoomNamespace := func(obj client.Object) bool {
		return obj.GetNamespace() == corev1alpha1.ServiceTargetNamespace
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.Service{}).
		Watches(&corev1.Pod{}, r.podHandler(), builder.WithPredicates(podPredicate())).
		// Generated Services: reconcile once the Service controller has created them.
		Watches(&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(mapOwnedToRequests),
			builder.WithPredicates(predicate.NewPredicateFuncs(inLoomNamespace)),
		).
		// Managed EndpointSlices: repair drift and deletions.
		Watches(&discoveryv1.EndpointSlice{},
			handler.EnqueueRequestsFromMapFunc(mapOwnedToRequests),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				return inLoomNamespace(obj) &&
					obj.GetLabels()[discoveryv1.LabelManagedBy] == corev1alpha1.EndpointSliceManagedBy
			})),
		).
		Named("core-endpointslice").
		Complete(r)
}
