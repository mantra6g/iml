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

package networkfunctionreplicaset

import (
	"context"
	"loom/api/core/v1alpha1"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	"loom/internal/controller/scheduling/networkfunctionreplicaset/util"
	rsutil "loom/internal/controller/scheduling/networkfunctionreplicaset/util"
	"loom/pkg/util/ptr"
)

const (
	NetworkFunctionReplicaSetPhaseUpscaling   = "Upscaling"
	NetworkFunctionReplicaSetPhaseDownscaling = "Downscaling"
)

// NetworkFunctionReplicaSetReconciler reconciles a NetworkFunctionReplicaSet object
type NetworkFunctionReplicaSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	expectations *util.UIDTrackingControllerExpectations
}

// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionreplicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionreplicasets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionreplicasets/finalizers,verbs=update
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReplicaSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	rs := &schedulingv1alpha1.NetworkFunctionReplicaSet{}
	if err := r.Get(ctx, req.NamespacedName, rs); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionReplicaSet resource not found. Ignoring since object must be deleted.")
			r.expectations.DeleteExpectations(logger, req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunctionReplicaSet")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling NetworkFunctionReplicaSet",
		"name", rs.Name, "namespace", rs.Namespace)

	// Check if all nfs that are expected to be created or deleted were fulfilled
	rsNeedsSync := r.expectations.SatisfiedExpectations(logger, req.NamespacedName.String())

	// Lists all active nfs that match the selector of the ReplicaSet.
	// It will also list nfs that haven't been claimed by the ReplicaSet yet, but match the selector
	// and are expected to be claimed by the ReplicaSet.
	allActiveNFs, err := r.listActiveNFs(ctx, rs)
	if err != nil {
		logger.Error(err, "unable to list NetworkFunctions for NetworkFunctionReplicaSet",
			"nfReplicaSet", rs)
		return ctrl.Result{}, err
	}

	// filter nfs to those owned by this ReplicaSet
	// TODO: we should be able to adopt nfs that match the selector but aren't owned by any ReplicaSet,
	//   and orphan nfs that don't match the selector but are owned by this ReplicaSet.
	//   However, it would also require more complex logic to handle adoption and orphaning,
	//   and we should be careful to avoid edge cases where we accidentally adopt or orphan
	//   nfs.
	ownedActiveNFs, err := r.filterOwnedPods(ctx, rs, allActiveNFs)
	if err != nil {
		logger.Error(err, "unable to claim NetworkFunctions for NetworkFunctionReplicaSet",
			"nfReplicaSet", rs)
		return ctrl.Result{}, err
	}

	var manageReplicasErr error
	if rsNeedsSync && rs.DeletionTimestamp == nil {
		manageReplicasErr = r.manageReplicas(ctx, ownedActiveNFs, rs)
	}
	rs = rs.DeepCopy()
	// Use the same time for calculating status and timeUntilNextRequeue.
	now := time.Now()
	newStatus := calculateStatus(rs, ownedActiveNFs, manageReplicasErr, now)

	// Always updates status as nfs come up or die.
	updatedRS, err := r.updateReplicaSetStatus(ctx, rs, newStatus)
	if err != nil {
		// Multiple things could lead to this update failing. Requeuing the replica set ensures
		// Returning an error causes a requeue without forcing a hotloop
		return ctrl.Result{}, err
	}
	if manageReplicasErr != nil {
		return ctrl.Result{}, manageReplicasErr
	}
	timeUntilNextRequeue := calculateTimeUntilNextRequeue(updatedRS, ownedActiveNFs, now)
	if timeUntilNextRequeue != nil {
		return ctrl.Result{RequeueAfter: *timeUntilNextRequeue}, nil
	}

	return ctrl.Result{}, nil
}

func calculateTimeUntilNextRequeue(
	updatedRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	activeNFs []*v1alpha1.NetworkFunction,
	now time.Time) *time.Duration {
	// Plan the next availability check as a last line of defense against queue preemption (we have one queue key for checking availability of all the pods)
	// or early sync (see https://github.com/kubernetes/kubernetes/issues/39785#issuecomment-279959133 for more info).
	var timeUntilNextRequeue *time.Duration
	if updatedRS.Spec.MinReadySeconds > 0 &&
		updatedRS.Status.ReadyReplicas != updatedRS.Status.AvailableReplicas {
		// Safeguard fallback to the .spec.minReadySeconds to ensure that we always end up with .status.availableReplicas updated.
		timeUntilNextRequeue = ptr.To(time.Duration(updatedRS.Spec.MinReadySeconds) * time.Second)
		// Use the same point in time (now) for calculating status and nextSyncDuration to get matching availability for the pods.
		if nextCheck := rsutil.FindMinNextNFAvailabilityCheck(activeNFs, updatedRS.Spec.MinReadySeconds, now); nextCheck != nil {
			timeUntilNextRequeue = nextCheck
		}
	}
	return timeUntilNextRequeue
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReplicaSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.expectations = util.NewUIDTrackingControllerExpectations(util.NewControllerExpectations())

	// Index the nfs by its controller UID, so that we can easily list the nfs for a given ReplicaSet.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.NetworkFunction{},
		controllerUIDIndex,
		func(obj client.Object) []string {
			nf := obj.(*v1alpha1.NetworkFunction)
			controllerRef := metav1.GetControllerOf(nf)
			if controllerRef == nil {
				return []string{}
			}
			return []string{string(controllerRef.UID)}
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionReplicaSet{}).
		Owns(&v1alpha1.NetworkFunction{}).
		Named("scheduling-networkfunctionreplicaset").
		Complete(r)
}
