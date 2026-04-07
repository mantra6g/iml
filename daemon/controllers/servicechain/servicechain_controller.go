/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package servicechain

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"time"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	"iml-daemon/env"
	"iml-daemon/pkg/dataplane"
	netutils "iml-daemon/pkg/utils/net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a ServiceChain object
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Config    *env.GlobalConfig
	Dataplane dataplane.Dataplane
}

// +kubebuilder:rbac:groups=core.loom.io,resources=servicechains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.loom.io,resources=servicechains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.loom.io,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=applications/status,verbs=get
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions/status,verbs=get
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Reconciling ServiceChain", "request", req)
	var serviceChain corev1alpha1.ServiceChain
	if err := r.Get(ctx, req.NamespacedName, &serviceChain); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ServiceChain resource not found. Deleting SRv6 routes")
			err = r.Dataplane.DeleteAllServiceChainRoutes(req.NamespacedName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get ServiceChain")
		return ctrl.Result{}, err
	}

	var srcApp = &corev1alpha1.Application{}
	err := r.Get(ctx, serviceChain.Spec.From.ToNamespacedName(), srcApp)
	if apierrors.IsNotFound(err) {
		logger.V(1).Info("source application resource not found. Requeueing event")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	if err != nil {
		logger.Error(err, "Failed to get source Application")
		return ctrl.Result{}, err
	}

	_, exists := srcApp.Status.Subnets[r.Config.NodeName]
	if !exists {
		logger.V(1).Info("source application does not have a local subnet. Omitting route calculation")
		return ctrl.Result{}, nil
	}

	var dstApp = &corev1alpha1.Application{}
	err = r.Get(ctx, serviceChain.Spec.To.ToNamespacedName(), dstApp)
	if apierrors.IsNotFound(err) {
		logger.V(1).Info("destination application resource not found. Requeueing event")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}
	if err != nil {
		logger.Error(err, "Failed to get source Application")
		return ctrl.Result{}, err
	}

	if len(dstApp.Status.Subnets) == 0 {
		logger.V(1).Info("destination application does not have any subnet. Omitting route calculation")
		return ctrl.Result{}, nil
	}

	matchingNFsPerStage := make([][]*corev1alpha1.NetworkFunction, 0)
	for stage := range serviceChain.Spec.Functions {
		matchingNFs, err := r.listMatchingNFs(ctx, serviceChain.Spec.Functions[stage])
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to list matching NFs: %w", err)
		}
		matchingNFsPerStage = append(matchingNFsPerStage, matchingNFs)
	}

	// Filter NFs to only use those with assigned p4Targets, ips, and in running phase
	for stage := range matchingNFsPerStage {
		matchingNFsPerStage[stage] = r.filterNFs(matchingNFsPerStage[stage])
	}

	routes, err := r.calculateSRv6Routes(ctx, &serviceChain, srcApp, dstApp, matchingNFsPerStage)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to calculate SRv6 Routes: %w", err)
	}

	err = r.Dataplane.AddServiceChainRoutes(&serviceChain, routes)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to add Route: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) calculateSRv6Routes(ctx context.Context, serviceChain *corev1alpha1.ServiceChain,
	srcApp *corev1alpha1.Application, dstApp *corev1alpha1.Application,
	matchingNFs [][]*corev1alpha1.NetworkFunction) ([]dataplane.SRv6Route, error) {
	logger := logf.FromContext(ctx)
	segments := make([]net.IP, 0)
	for _, nfs := range matchingNFs {
		parsedIP := net.ParseIP(nfs[0].Status.AssignedIP)
		if parsedIP == nil {
			logger.V(1).Error(fmt.Errorf("failed to parse assigned IP for NF"),
				"ip", nfs[0].Status.AssignedIP, "nf", nfs[0].Namespace+"/"+nfs[0].Name)
			continue
		}
		segments = append(segments, parsedIP) // For now, just take the first matching NF for each stage
	}
	routes := make([]dataplane.SRv6Route, 0)
	for _, dstSubnets := range dstApp.Status.Subnets {
		for _, subnet := range dstSubnets {
			routes = append(routes, dataplane.SRv6Route{
				SourceApp:      client.ObjectKeyFromObject(srcApp),
				DestinationApp: client.ObjectKeyFromObject(dstApp),
				DestNet:        netutils.DualStackNetwork{IPv4Net: subnet.IPv4Net, IPv6Net: subnet.IPv6Net},
				FunctionIPs:    segments,
			})
		}
	}
	return routes, nil
}

func (r *Reconciler) filterNFs(originalNFs []*corev1alpha1.NetworkFunction) []*corev1alpha1.NetworkFunction {
	var filteredNFs = make([]*corev1alpha1.NetworkFunction, 0)
	for _, nf := range originalNFs {
		if nf.Spec.TargetName == "" {
			continue
		}
		readyCondition := GetReadyCondition(nf)
		if readyCondition == nil || readyCondition.Status != v1.ConditionTrue {
			continue
		}
	}
	return filteredNFs
}

func (r *Reconciler) listMatchingNFs(ctx context.Context, labelSelector v1.LabelSelector,
) ([]*corev1alpha1.NetworkFunction, error) {
	selector, err := v1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return nil, err
	}
	var matchingNFsList = corev1alpha1.NetworkFunctionList{}
	err = r.List(ctx, &matchingNFsList, client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}
	var matchingNFs = make([]*corev1alpha1.NetworkFunction, len(matchingNFsList.Items))
	for i := range matchingNFsList.Items {
		matchingNFs[i] = &matchingNFsList.Items[i]
	}
	return matchingNFs, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index the ServiceChain by its source application name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1alpha1.ServiceChain{},
		"spec.from",
		func(obj client.Object) []string {
			sc := obj.(*corev1alpha1.ServiceChain)
			if sc.Spec.From.Name == "" {
				return nil
			}
			ns := sc.Spec.From.Namespace
			if ns == "" {
				ns = sc.Namespace
			}
			return []string{ns + "/" + sc.Spec.From.Name}
		},
	); err != nil {
		return err
	}

	// Index the ServiceChain by its destination application name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1alpha1.ServiceChain{},
		"spec.to",
		func(obj client.Object) []string {
			sc := obj.(*corev1alpha1.ServiceChain)
			if sc.Spec.From.Name == "" {
				return nil
			}
			ns := sc.Spec.From.Namespace
			if ns == "" {
				ns = sc.Namespace
			}
			return []string{ns + "/" + sc.Spec.From.Name}
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.ServiceChain{}).
		Watches(&corev1alpha1.Application{},
			handler.EnqueueRequestsFromMapFunc(r.mapApplicationsToRequests)).
		Watches(&infrav1alpha1.LoomNode{},
			handler.EnqueueRequestsFromMapFunc(r.reconcileAllChains),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldNode := e.ObjectOld.(*infrav1alpha1.LoomNode)
					newNode := e.ObjectNew.(*infrav1alpha1.LoomNode)
					return !reflect.DeepEqual(oldNode.Spec, newNode.Spec) // Only update if spec changed
				},
				DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool { return true },
			})).
		Watches(&corev1alpha1.NetworkFunction{},
			handler.EnqueueRequestsFromMapFunc(r.mapNFsToRequests),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool { return false }, // NFs shouldn't be updated
				DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool { return true },
			})).
		Watches(&corev1alpha1.P4Target{},
			handler.EnqueueRequestsFromMapFunc(r.reconcileAllChains),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldTarget := e.ObjectOld.(*corev1alpha1.P4Target)
					newTarget := e.ObjectNew.(*corev1alpha1.P4Target)
					return oldTarget.Status.NodeName != newTarget.Status.NodeName
				},
				DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool { return true },
			})).
		Named("servicechain-daemon").
		Complete(r)
}

func (r *Reconciler) mapApplicationsToRequests(ctx context.Context, object client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	app := object.(*corev1alpha1.Application)

	// Find all ServiceChains that use this Application as source...
	var fromList corev1alpha1.ServiceChainList
	if err := r.List(ctx, &fromList,
		client.MatchingFields{
			"spec.from": app.Namespace + "/" + app.Name,
		},
		client.InNamespace(app.Namespace),
	); err != nil {
		logger.Error(err, "could not list ServiceChains. "+
			"change to Application %s.%s will not be reconciled.",
			app.Name, app.Namespace)
		return nil
	}

	// ... now find all that use this App as destination...
	var toList corev1alpha1.ServiceChainList
	if err := r.List(ctx, &toList,
		client.MatchingFields{
			"spec.to": app.Namespace + "/" + app.Name,
		},
		client.InNamespace(app.Namespace),
	); err != nil {
		logger.Error(err, "could not list ServiceChains. "+
			"change to Application %s.%s will not be reconciled.",
			app.Name, app.Namespace)
		return nil
	}

	// ... merge the service chain lists and transform them into requests.
	requests := make([]reconcile.Request, len(fromList.Items)+len(toList.Items))
	for i, sc := range fromList.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&sc),
		}
	}
	for i, sc := range toList.Items {
		requests[i+len(fromList.Items)] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&sc),
		}
	}
	return requests
}

func (r *Reconciler) reconcileAllChains(ctx context.Context, _ client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	serviceChains := &corev1alpha1.ServiceChainList{}
	if err := r.List(ctx, serviceChains); err != nil {
		logger.Error(err, "could not list ServiceChains. ")
	}
	requests := make([]reconcile.Request, len(serviceChains.Items))
	for i := range serviceChains.Items {
		requests[i] = reconcile.Request{}
	}
	return requests
}

func (r *Reconciler) mapNFsToRequests(ctx context.Context, object client.Object) []reconcile.Request {
	logger := logf.FromContext(ctx)
	nf := object.(*corev1alpha1.NetworkFunction)

	var chainList corev1alpha1.ServiceChainList
	err := r.List(ctx, &chainList, client.InNamespace(nf.Namespace))
	if err != nil {
		logger.Error(err, "could not list ServiceChains. ")
	}
	requests := make([]reconcile.Request, 0)
	for i := range chainList.Items {
		sc := &chainList.Items[i]
		for j := range sc.Spec.Functions {
			selector, _ := v1.LabelSelectorAsSelector(&sc.Spec.Functions[j])
			if selector.Matches(labels.Set(nf.Labels)) {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(sc)})
				break
			}
		}
	}
	return requests
}
