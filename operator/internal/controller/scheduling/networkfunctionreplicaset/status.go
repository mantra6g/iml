package networkfunctionreplicaset

import (
	"context"
	"fmt"
	"reflect"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	rsutil "loom/internal/controller/scheduling/networkfunctionreplicaset/util"
)

const (
	// StatusUpdateRetries is the number of times we will retry updating the status of a NetworkFunctionReplicaSet before
	// giving up and requeuing with a rate limit. This is needed to handle transient update conflicts.
	StatusUpdateRetries = 1
)

func calculateStatus(rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
	activeNFs []*schedulingv1alpha1.NetworkFunction, manageReplicasErr error, now time.Time,
) schedulingv1alpha1.NetworkFunctionReplicaSetStatus {
	newStatus := rs.Status
	// Count the number of nfs that have labels matching the labels of the pod
	// template of the replica set, the matching nfs may have more
	// labels than are in the template. Because the label of podTemplateSpec is
	// a superset of the selector of the replica set, so the possible
	// matching nfs must be part of the activeNFs.
	fullyLabeledReplicasCount, readyReplicasCount, availableReplicasCount := rsutil.CountReplicas(rs, activeNFs, now)

	failureCond := rsutil.GetCondition(rs.Status, schedulingv1alpha1.ReplicaSetReplicaFailure)
	if manageReplicasErr != nil && failureCond == nil {
		var reason string
		if diff := len(activeNFs) - int(*(rs.Spec.Replicas)); diff < 0 {
			reason = "FailedCreate"
		} else if diff > 0 {
			reason = "FailedDelete"
		}
		cond := rsutil.NewReplicaSetCondition(
			schedulingv1alpha1.ReplicaSetReplicaFailure, metav1.ConditionTrue, reason, manageReplicasErr.Error())
		rsutil.SetCondition(&newStatus, cond)
	} else if manageReplicasErr == nil && failureCond != nil {
		rsutil.RemoveCondition(&newStatus, schedulingv1alpha1.ReplicaSetReplicaFailure)
	}

	newStatus.Replicas = int32(len(activeNFs))
	newStatus.FullyLabeledReplicas = int32(fullyLabeledReplicasCount)
	newStatus.ReadyReplicas = int32(readyReplicasCount)
	newStatus.AvailableReplicas = int32(availableReplicasCount)
	return newStatus
}

// updateReplicaSetStatus attempts to update the Status.Replicas of the given ReplicaSet, with a single GET/PUT retry.
// Returns the updated ReplicaSet, or an error if the update failed after retries.
// The caller is expected to requeue with a rate limit if an error is returned.
func (r *NetworkFunctionReplicaSetReconciler) updateReplicaSetStatus(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
	newStatus schedulingv1alpha1.NetworkFunctionReplicaSetStatus,
) (*schedulingv1alpha1.NetworkFunctionReplicaSet, error) {
	logger := logf.FromContext(ctx)

	// This is the steady state. It happens when the ReplicaSet doesn't have any expectations, since
	// we do a periodic relist every 30s. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if rs.Status.Replicas == newStatus.Replicas &&
		rs.Status.FullyLabeledReplicas == newStatus.FullyLabeledReplicas &&
		rs.Status.ReadyReplicas == newStatus.ReadyReplicas &&
		rs.Status.AvailableReplicas == newStatus.AvailableReplicas &&
		rs.Generation == rs.Status.ObservedGeneration &&
		reflect.DeepEqual(rs.Status.Conditions, newStatus.Conditions) {
		return rs, nil
	}

	// Save the generation number we acted on, otherwise we might wrongfully indicate
	// that we've seen a spec update when we retry.
	// TODO: This can clobber an update if we allow multiple agents to write to the
	//   same status.
	newStatus.ObservedGeneration = rs.Generation

	var getErr, updateErr error
	for i, rs := 0, rs; ; i++ {
		logger.V(4).Info(fmt.Sprintf("Updating status for %v: %s/%s, ", rs.Kind, rs.Namespace, rs.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", rs.Status.Replicas, newStatus.Replicas, *(rs.Spec.Replicas)) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", rs.Status.FullyLabeledReplicas, newStatus.FullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", rs.Status.ReadyReplicas, newStatus.ReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", rs.Status.AvailableReplicas, newStatus.AvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", rs.Status.ObservedGeneration, newStatus.ObservedGeneration))

		rs.Status = newStatus
		updateErr = r.Status().Update(ctx, rs)
		if updateErr == nil {
			return rs, nil
		}
		// Stop retrying if we exceed statusUpdateRetries - the replicaSet will be requeued with a rate limit.
		if i >= StatusUpdateRetries {
			break
		}
		// Update the ReplicaSet with the latest resource version for the next poll
		rs = &schedulingv1alpha1.NetworkFunctionReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rs.Name,
				Namespace: rs.Namespace,
			},
		}
		if getErr = r.Get(ctx, client.ObjectKeyFromObject(rs), rs); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			return rs, getErr
		}
	}

	return rs, updateErr
}
