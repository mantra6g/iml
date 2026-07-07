// the Kubernetes EndpointSlice controller was used as reference material for this controller: https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/endpointslice/endpointslice_controller.go
// I put this apache license here just in case
/*
Copyright 2019 The Kubernetes Authors.

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

package nsendpoint

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	endpointsliceutil "github.com/mantra6g/iml/operator/internal/controller/core/nsendpoint/util"
)

const (
	ControllerName       = "core-network-service"
	MaxEndpointsPerSlice = 1000
)

// Reconciler reconciles a NetworkService object
type Reconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	triggerTimeTracker   *endpointsliceutil.TriggerTimeTracker
	endpointSliceTracker *endpointsliceutil.EndpointSliceTracker
}

// +kubebuilder:rbac:groups=discoveryv1.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=discoveryv1.k8s.io,resources=endpointslices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=discoveryv1.k8s.io,resources=endpointslices/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.loom.io,resources=networkservices,verbs=get;list;watch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	ns := &corev1alpha1.NetworkService{}
	if err := r.Get(ctx, req.NamespacedName, ns); apierrors.IsNotFound(err) {
		// Network service has been deleted, proceed to delete endpoint slices
		logger.V(1).Info("NetworkService resource not found. Cleaning up associated EndpointSlices")
		return ctrl.Result{}, nil
	}

	nsSelector := labels.ValidatedSetSelector(ns.Spec.Selector)
	nfList := &corev1alpha1.NetworkFunctionList{}
	if err := r.List(ctx, nfList, client.MatchingLabelsSelector{Selector: nsSelector}); err != nil {
		logger.Error(err, "Failed to list NetworkFunctions")
		return ctrl.Result{}, err
	}

	esLabelSelector := labels.Set(map[string]string{
		corev1alpha1.LabelNetworkServiceName: ns.Name,
		corev1alpha1.LabelManagedBy:          "nsendpoint-controller",
	}).AsSelectorPreValidated()
	endpointSliceList := &discoveryv1.EndpointSliceList{}
	if err := r.List(ctx, endpointSliceList, client.MatchingLabelsSelector{Selector: esLabelSelector}); err != nil {
		logger.Error(err, "Failed to list EndpointSlice")
		return ctrl.Result{}, err
	}

	existingSlices := dropEndpointSlicesPendingDeletion(endpointSliceList.Items)
	slicesToDelete := []*discoveryv1.EndpointSlice{}
	errs := []error{}

	// loop through slices identifying their address type.
	// slices that no longer match address type supported by services
	// go to delete, other slices goes to the Reconciler machinery
	// for further adjustment
	for _, existingSlice := range existingSlices {
		// Only IPv6 is allowed for NF addresses
		if existingSlice.AddressType != discoveryv1.AddressTypeIPv6 {
			slicesToDelete = append(slicesToDelete, existingSlice)
			continue
		}
	}

	// We call ComputeEndpointLastChangeTriggerTime here to make sure that the
	// state of the trigger time tracker gets updated even if the sync turns out
	// to be no-op and we don't update the EndpointSlice objects.
	lastChangeTriggerTime := r.triggerTimeTracker.
		ComputeEndpointLastChangeTriggerTime(req.Namespace, ns, nfList.Items)

	// reconcile for existing.
	err := r.syncEndpointSlices(ctx, ns, nfList.Items, existingSlices, lastChangeTriggerTime)
	if err != nil {
		errs = append(errs, err)
	}

	// delete those which are of addressType that is not supported by the network service
	for _, sliceToDelete := range slicesToDelete {
		err = r.Delete(ctx, sliceToDelete)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("failed to delete EndpointSlice %s for Service %s/%s: %w",
					sliceToDelete.Name, ns.Namespace, ns.Name, err))
		} else {
			r.endpointSliceTracker.ExpectDeletion(sliceToDelete)
		}
	}

	return ctrl.Result{}, utilerrors.NewAggregate(errs)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.NetworkService{},
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					// 1. Mandatory nil checks
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					// 2. Ignore informer resyncs where the object hasn't actually changed
					if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
						return false
					}
					// TODO: Change this so that it queues a reconcile when the selector changes but not the status changes.
					_, okOld := e.ObjectOld.(*corev1alpha1.NetworkService)
					_, okNew := e.ObjectNew.(*corev1alpha1.NetworkService)
					if !okOld || !okNew {
						return false // Not a network service, shouldn't happen if watch is configured correctly
					}
					return true
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object != nil
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object != nil
				},
			})).
		Watches(&discoveryv1.EndpointSlice{},
			handler.EnqueueRequestsFromMapFunc(r.mapEndpointSlicesToRequests),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					// 1. Mandatory nil checks
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					// 2. Ignore informer resyncs where the resource version hasn't changed
					if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
						return false
					}
					oldSlice, okOld := e.ObjectOld.(*discoveryv1.EndpointSlice)
					newSlice, okNew := e.ObjectNew.(*discoveryv1.EndpointSlice)
					if !okOld || !okNew {
						return false // Not an EndpointSlice
					}
					// 3. Check for structural changes to the Endpoints
					// (Captures added/removed IPs, changed Conditions, changed NodeNames)
					if !reflect.DeepEqual(oldSlice.Endpoints, newSlice.Endpoints) {
						return true
					}
					// 4. Check for structural changes to the exposed Ports
					if !reflect.DeepEqual(oldSlice.Ports, newSlice.Ports) {
						return true
					}
					// 5. Check if the parent Service mapping changed
					// TODO: If the service name changes, we should issue a reconcile for the old service too.
					oldSvcName := oldSlice.Labels[discoveryv1.LabelServiceName]
					newSvcName := newSlice.Labels[discoveryv1.LabelServiceName]
					if oldSvcName != newSvcName {
						return true
					}
					// If the endpoints, ports, and service mapping are identical,
					// this is likely a metadata update (e.g., managed fields) that we can ignore.
					return false
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object != nil
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object != nil
				},
			})).
		Watches(&corev1alpha1.NetworkFunction{},
			handler.EnqueueRequestsFromMapFunc(r.mapNFsToRequests),
			// We need a big optimization for these requests as the enqueue mapfunc lists all the network services to see which one they match
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					// 1. Mandatory nil checks
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					// 2. Ignore informer resyncs where the object hasn't actually changed
					if e.ObjectOld.GetResourceVersion() == e.ObjectNew.GetResourceVersion() {
						return false
					}
					oldNF, okOld := e.ObjectOld.(*corev1alpha1.NetworkFunction)
					newNF, okNew := e.ObjectNew.(*corev1alpha1.NetworkFunction)
					if !okOld || !okNew {
						return false // Not a nf, shouldn't happen if watch is configured correctly
					}
					// 3. Check for Label changes (might affect selector matching)
					if !reflect.DeepEqual(oldNF.Labels, newNF.Labels) {
						return true
					}
					// 4. Check for IP Address allocations or changes
					if oldNF.Status.AssignedIP != newNF.Status.AssignedIP {
						return true
					}
					// 5. Check for Deletion Timestamp (start of termination)
					if (oldNF.DeletionTimestamp == nil) != (newNF.DeletionTimestamp == nil) {
						return true
					}
					// 6. Check for Phase changes (e.g., entering Succeeded or Failed)
					if oldNF.Status.Phase != newNF.Status.Phase {
						return true
					}
					// 7. Check for Readiness/Serving Condition changes
					if nfReadyConditionChanged(oldNF, newNF) {
						return true
					}
					// If none of the routing-critical fields changed, safely drop the event
					return false
				},
				// Create and Delete should generally return true (after nil checks),
				// as new or removed NFs always alter the routing topology.
				CreateFunc: func(e event.CreateEvent) bool { return e.Object != nil },
				DeleteFunc: func(e event.DeleteEvent) bool { return e.Object != nil },
			})).
		Named(ControllerName).
		Complete(r)
}

func (r *Reconciler) mapNFsToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj == nil {
		return nil
	}
	nsList := &corev1alpha1.NetworkServiceList{}
	if err := r.List(ctx, nsList); err != nil {
		return nil
	}
	requests := []reconcile.Request{}
	for i := range nsList.Items {
		ns := &nsList.Items[i]
		nsSelector := labels.ValidatedSetSelector(ns.Spec.Selector)
		if nsSelector.Matches(labels.Set(obj.GetLabels())) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(ns)})
		}
	}
	return requests
}

func (r *Reconciler) mapEndpointSlicesToRequests(_ context.Context, obj client.Object) []reconcile.Request {
	if obj == nil {
		return nil
	}
	objLabels := obj.GetLabels()
	serviceNameLabel, ok := objLabels[corev1alpha1.LabelNetworkServiceName]
	if !ok {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Name:      serviceNameLabel,
			Namespace: obj.GetNamespace(),
		},
	}}
}

func nfReadyConditionChanged(oldNF, newNF *corev1alpha1.NetworkFunction) bool {
	oldReady := getNFReadyCondition(oldNF)
	newReady := getNFReadyCondition(newNF)
	if oldReady == nil || newReady == nil {
		return oldReady != newReady
	}
	return oldReady.Status != newReady.Status || oldReady.Reason != newReady.Reason || oldReady.Message != newReady.Message
}

func getNFReadyCondition(nf *corev1alpha1.NetworkFunction) *corev1alpha1.NetworkFunctionCondition {
	if nf == nil {
		return nil
	}
	for i := range nf.Status.Conditions {
		c := &nf.Status.Conditions[i]
		if c.Type == corev1alpha1.NetworkFunctionReady {
			return c
		}
	}
	return nil
}

func dropEndpointSlicesPendingDeletion(endpointSlices []discoveryv1.EndpointSlice) []*discoveryv1.EndpointSlice {
	slices := make([]*discoveryv1.EndpointSlice, 0)
	for i := range endpointSlices {
		endpointSlice := &endpointSlices[i]
		if endpointSlice.DeletionTimestamp == nil {
			slices = append(slices, endpointSlice)
		}
	}
	return slices
}

// syncEndpointSlices takes a set of nfs currently matching a network service selector and
// compares them with the endpoints already present in any existing endpoint
// slices for the given network service. It creates, updates, or deletes endpoint slices
// to ensure the desired set of nfs are represented by endpoint slices.
func (r *Reconciler) syncEndpointSlices(ctx context.Context, ns *corev1alpha1.NetworkService, nfs []corev1alpha1.NetworkFunction, existingSlices []*discoveryv1.EndpointSlice, triggerTime time.Time) error {
	logger := logf.FromContext(ctx)
	errs := make([]error, 0)

	slicesToCreate := make([]*discoveryv1.EndpointSlice, 0)
	slicesToUpdate := make([]*discoveryv1.EndpointSlice, 0)
	slicesToDelete := make([]*discoveryv1.EndpointSlice, 0)

	// Build data structures for existing state.
	var ownedExistingSlices []*discoveryv1.EndpointSlice
	for _, existingSlice := range existingSlices {
		if endpointsliceutil.OwnedBy(existingSlice, ns) {
			ownedExistingSlices = append(ownedExistingSlices, existingSlice)
		} else {
			slicesToDelete = append(slicesToDelete, existingSlice)
		}
	}

	// Build data structures for desired state.
	desiredEndpoints := endpointsliceutil.EndpointSet{}

	for i := range nfs {
		nf := &nfs[i]
		if !endpointsliceutil.ShouldNFBeInEndpoints(nf, true) {
			continue
		}

		target := &corev1alpha1.P4Target{}
		err := r.Get(ctx, types.NamespacedName{Name: nf.Spec.TargetName}, target)
		if err != nil {
			// we are getting the information from the local informer,
			// an error different than IsNotFound should not happen
			if !apierrors.IsNotFound(err) {
				return err
			}
			// If the Node specified by the Pod doesn't exist we want to requeue the Service so we
			// retry later, but also update the EndpointSlice without the problematic Pod.
			// Theoretically, the pod Garbage Collector will remove the Pod, but we want to avoid
			// situations where a reference from a Pod to a missing node can leave the EndpointSlice
			// stuck forever.
			// On the other side, if the service.Spec.PublishNotReadyAddresses is set we just add the
			// Pod, since the user is explicitly indicating that the Pod address should be published.
			//if !ns.Spec.PublishNotReadyAddresses {
			logger.Info("skipping NF for NetworkService, P4Target not found", "network-function", client.ObjectKeyFromObject(nf), "network-service", client.ObjectKeyFromObject(ns), "p4target", nf.Spec.TargetName)
			errs = append(errs, fmt.Errorf("skipping NF %s for NetworkService %s/%s: P4Target %s Not Found", nf.Name, ns.Namespace, ns.Name, nf.Spec.TargetName))
			continue
			//}
		}
		endpoint := endpointsliceutil.NFToEndpoint(nf, target, ns)
		if len(endpoint.Addresses) > 0 {
			desiredEndpoints.Insert(&endpoint)
		}
	}

	totalAdded := 0
	totalRemoved := 0

	// Determine changes necessary for each group of slices by port map.
	pmSlicesToCreate, pmSlicesToUpdate, pmSlicesToDelete, added, removed := r.splitSlices(
		logger, ns, ownedExistingSlices, desiredEndpoints)

	totalAdded += added
	totalRemoved += removed

	slicesToCreate = append(slicesToCreate, pmSlicesToCreate...)
	slicesToUpdate = append(slicesToUpdate, pmSlicesToUpdate...)
	slicesToDelete = append(slicesToDelete, pmSlicesToDelete...)

	// When no endpoint slices would usually exist, we need to add a placeholder.
	if len(existingSlices) == len(slicesToDelete) && len(slicesToCreate) < 1 {
		// Check for existing placeholder slice outside of the core control flow
		placeholderSlice := endpointsliceutil.NewEndpointSlice(logger, ns, ControllerName)
		if len(slicesToDelete) == 1 && placeholderSliceCompare.DeepEqual(slicesToDelete[0], placeholderSlice) {
			// We are about to unnecessarily delete/recreate the placeholder, remove it now.
			slicesToDelete = slicesToDelete[:0]
		} else {
			slicesToCreate = append(slicesToCreate, placeholderSlice)
		}
	}

	err := r.finalize(ctx, ns, slicesToCreate, slicesToUpdate, slicesToDelete, triggerTime)
	if err != nil {
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// placeholderSliceCompare is a conversion func for comparing two placeholder endpoint slices.
// It only compares the specific fields we care about.
var placeholderSliceCompare = conversion.EqualitiesOrDie(
	func(a, b metav1.OwnerReference) bool {
		return a.String() == b.String()
	},
	func(a, b metav1.ObjectMeta) bool {
		if a.Namespace != b.Namespace {
			return false
		}
		for k, v := range a.Labels {
			if b.Labels[k] != v {
				return false
			}
		}
		for k, v := range b.Labels {
			if a.Labels[k] != v {
				return false
			}
		}
		return true
	},
)

// finalize creates, updates, and deletes slices as specified
func (r *Reconciler) finalize(
	ctx context.Context,
	ns *corev1alpha1.NetworkService,
	slicesToCreate,
	slicesToUpdate,
	slicesToDelete []*discoveryv1.EndpointSlice,
	triggerTime time.Time,
) error {
	// If there are slices to create and delete, change the creates to updates
	// of the slices that would otherwise be deleted.
	for i := 0; i < len(slicesToDelete); {
		if len(slicesToCreate) == 0 {
			break
		}
		sliceToDelete := slicesToDelete[i]
		slice := slicesToCreate[len(slicesToCreate)-1]
		// Only update EndpointSlices that are owned by this Service and have
		// the same AddressType. We need to avoid updating EndpointSlices that
		// are being garbage collected for an old Service with the same name.
		// The AddressType field is immutable. Since Services also consider
		// IPFamily immutable, the only case where this should matter will be
		// the migration from IP to IPv4 and IPv6 AddressTypes, where there's a
		// chance EndpointSlices with an IP AddressType would otherwise be
		// updated to IPv4 or IPv6 without this check.
		if sliceToDelete.AddressType == slice.AddressType && endpointsliceutil.OwnedBy(sliceToDelete, ns) {
			slice.Name = sliceToDelete.Name
			slicesToCreate = slicesToCreate[:len(slicesToCreate)-1]
			slicesToUpdate = append(slicesToUpdate, slice)
			slicesToDelete = append(slicesToDelete[:i], slicesToDelete[i+1:]...)
		} else {
			i++
		}
	}

	// Don't create new EndpointSlices if the Service is pending deletion. This
	// is to avoid a potential race condition with the garbage collector where
	// it tries to delete EndpointSlices as this controller replaces them.
	if ns.DeletionTimestamp == nil {
		for _, endpointSlice := range slicesToCreate {
			endpointsliceutil.AddTriggerTimeAnnotation(endpointSlice, triggerTime)
			err := r.Create(ctx, endpointSlice)
			if err != nil {
				// If the namespace is terminating, creates will continue to fail. Simply drop the item.
				if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
					return nil
				}
				return fmt.Errorf("failed to create EndpointSlice for Service %s/%s: %v", ns.Namespace, ns.Name, err)
			}
		}
	}

	for _, endpointSlice := range slicesToUpdate {
		endpointsliceutil.AddTriggerTimeAnnotation(endpointSlice, triggerTime)
		err := r.Update(ctx, endpointSlice)
		if err != nil {
			return fmt.Errorf("failed to update %s EndpointSlice for Service %s/%s: %v", endpointSlice.Name, ns.Namespace, ns.Name, err)
		}
	}

	for _, endpointSlice := range slicesToDelete {
		err := r.Delete(ctx, endpointSlice)
		if err != nil {
			return fmt.Errorf("failed to delete %s EndpointSlice for Service %s/%s: %v", endpointSlice.Name, ns.Namespace, ns.Name, err)
		}
	}

	return nil
}

// splitSlices compares the endpoints found in existing slices with
// the list of desired endpoints and returns lists of slices to create, update,
// and delete. It also checks that the slices mirror the parent services labels.
// The logic is split up into several main steps:
//  1. Iterate through existing slices, delete endpoints that are no longer
//     desired and update matching endpoints that have changed. It also checks
//     if the slices have the labels of the parent services, and updates them if not.
//  2. Iterate through slices that have been modified in 1 and fill them up with
//     any remaining desired endpoints.
//  3. If there still desired endpoints left, try to fit them into a previously
//     unchanged slice and/or create new ones.
func (r *Reconciler) splitSlices(
	logger klog.Logger,
	service *corev1alpha1.NetworkService,
	existingSlices []*discoveryv1.EndpointSlice,
	desiredSet endpointsliceutil.EndpointSet,
) ([]*discoveryv1.EndpointSlice, []*discoveryv1.EndpointSlice, []*discoveryv1.EndpointSlice, int, int) {
	slicesByName := map[string]*discoveryv1.EndpointSlice{}
	sliceNamesUnchanged := sets.New[string]()
	sliceNamesToUpdate := sets.New[string]()
	sliceNamesToDelete := sets.New[string]()
	numRemoved := 0

	// 1. Iterate through existing slices to delete endpoints no longer desired
	//    and update endpoints that have changed
	for _, existingSlice := range existingSlices {
		slicesByName[existingSlice.Name] = existingSlice
		newEndpoints := make([]discoveryv1.Endpoint, 0)
		endpointUpdated := false
		for _, endpoint := range existingSlice.Endpoints {
			got := desiredSet.Get(&endpoint)
			// If endpoint is desired add it to list of endpoints to keep.
			if got != nil {
				newEndpoints = append(newEndpoints, *got)
				// If existing version of endpoint doesn't match desired version
				// set endpointUpdated to ensure endpoint changes are persisted.
				if !endpointsliceutil.EndpointsEqualBeyondHash(got, &endpoint) {
					endpointUpdated = true
				}
				// once an endpoint has been placed/found in a slice, it no
				// longer needs to be handled
				desiredSet.Delete(&endpoint)
			}
		}

		// generate the slice labels and check if parent labels have changed
		sliceLabels, labelsChanged := endpointsliceutil.SetEndpointSliceLabels(logger, existingSlice, service, ControllerName)

		// If an endpoint was updated or removed, mark for update or delete
		if endpointUpdated || len(existingSlice.Endpoints) != len(newEndpoints) {
			if len(existingSlice.Endpoints) > len(newEndpoints) {
				numRemoved += len(existingSlice.Endpoints) - len(newEndpoints)
			}
			if len(newEndpoints) == 0 {
				// if no endpoints desired in this slice, mark for deletion
				sliceNamesToDelete.Insert(existingSlice.Name)
			} else {
				// otherwise, copy and mark for update
				epSlice := existingSlice.DeepCopy()
				epSlice.Endpoints = newEndpoints
				epSlice.Labels = sliceLabels
				slicesByName[existingSlice.Name] = epSlice
				sliceNamesToUpdate.Insert(epSlice.Name)
			}
		} else if labelsChanged {
			// if labels have changed, copy and mark for update
			epSlice := existingSlice.DeepCopy()
			epSlice.Labels = sliceLabels
			slicesByName[existingSlice.Name] = epSlice
			sliceNamesToUpdate.Insert(epSlice.Name)
		} else {
			// slices with no changes will be useful if there are leftover endpoints
			sliceNamesUnchanged.Insert(existingSlice.Name)
		}
	}

	numAdded := desiredSet.Len()

	// 2. If we still have desired endpoints to add and slices marked for update,
	//    iterate through the slices and fill them up with the desired endpoints.
	if desiredSet.Len() > 0 && sliceNamesToUpdate.Len() > 0 {
		slices := []*discoveryv1.EndpointSlice{}
		for _, sliceName := range sliceNamesToUpdate.UnsortedList() {
			slices = append(slices, slicesByName[sliceName])
		}
		// Sort endpoint slices by length so we're filling up the fullest ones
		// first.
		sort.Sort(endpointsliceutil.EndpointSliceEndpointLen(slices))

		// Iterate through slices and fill them up with desired endpoints.
		for _, slice := range slices {
			for desiredSet.Len() > 0 && len(slice.Endpoints) < MaxEndpointsPerSlice {
				endpoint, _ := desiredSet.PopAny()
				slice.Endpoints = append(slice.Endpoints, *endpoint)
			}
		}
	}

	// 3. If there are still desired endpoints left at this point, we try to fit
	//    the endpoints in a single existing slice. If there are no slices with
	//    that capacity, we create new slices for the endpoints.
	slicesToCreate := []*discoveryv1.EndpointSlice{}

	for desiredSet.Len() > 0 {
		var sliceToFill *discoveryv1.EndpointSlice

		// If the remaining amounts of endpoints is smaller than the max
		// endpoints per slice and we have slices that haven't already been
		// filled, try to fit them in one.
		if desiredSet.Len() < MaxEndpointsPerSlice && sliceNamesUnchanged.Len() > 0 {
			unchangedSlices := []*discoveryv1.EndpointSlice{}
			for _, sliceName := range sliceNamesUnchanged.UnsortedList() {
				unchangedSlices = append(unchangedSlices, slicesByName[sliceName])
			}
			sliceToFill = endpointsliceutil.GetSliceToFill(unchangedSlices, desiredSet.Len(), MaxEndpointsPerSlice)
		}

		// If we didn't find a sliceToFill, generate a new empty one.
		if sliceToFill == nil {
			sliceToFill = endpointsliceutil.NewEndpointSlice(logger, service, ControllerName)
		} else {
			// deep copy required to modify this slice.
			sliceToFill = sliceToFill.DeepCopy()
			slicesByName[sliceToFill.Name] = sliceToFill
		}

		// Fill the slice up with remaining endpoints.
		for desiredSet.Len() > 0 && len(sliceToFill.Endpoints) < MaxEndpointsPerSlice {
			endpoint, _ := desiredSet.PopAny()
			sliceToFill.Endpoints = append(sliceToFill.Endpoints, *endpoint)
		}

		// New slices will not have a Name set, use this to determine whether
		// this should be an update or create.
		if sliceToFill.Name != "" {
			sliceNamesToUpdate.Insert(sliceToFill.Name)
			sliceNamesUnchanged.Delete(sliceToFill.Name)
		} else {
			slicesToCreate = append(slicesToCreate, sliceToFill)
		}
	}

	// Build slicesToUpdate from slice names.
	slicesToUpdate := []*discoveryv1.EndpointSlice{}
	for _, sliceName := range sliceNamesToUpdate.UnsortedList() {
		slicesToUpdate = append(slicesToUpdate, slicesByName[sliceName])
	}

	// Build slicesToDelete from slice names.
	slicesToDelete := []*discoveryv1.EndpointSlice{}
	for _, sliceName := range sliceNamesToDelete.UnsortedList() {
		slicesToDelete = append(slicesToDelete, slicesByName[sliceName])
	}

	return slicesToCreate, slicesToUpdate, slicesToDelete, numAdded, numRemoved
}
