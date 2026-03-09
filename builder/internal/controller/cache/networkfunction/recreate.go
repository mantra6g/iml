package networkfunction

import (
	cachev1alpha1 "builder/api/cache/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	nfutil "builder/internal/controller/cache/networkfunction/util"
	"context"
)

func (r *NetworkFunctionReconciler) scaleDownOldReplicaSetsForRecreate(ctx context.Context,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, nf *cachev1alpha1.NetworkFunction,
) (bool, error) {
	scaled := false
	for i := range oldRSs {
		rs := oldRSs[i]
		// Scaling not required.
		if *(rs.Spec.Replicas) == 0 {
			continue
		}
		scaledRS, updatedRS, err := r.scaleReplicaSet(ctx, rs, 0, nf, false)
		if err != nil {
			return false, err
		}
		if scaledRS {
			oldRSs[i] = updatedRS
			scaled = true
		}
	}
	return scaled, nil
}

func oldPodsRunning(currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
) bool {
	if oldPods := nfutil.GetActualReplicaCountForReplicaSets(oldRSs); oldPods > 0 {
		return true
	}
	//for rsUID, podList := range podMap {
	//	// If the pods belong to the new ReplicaSet, ignore.
	//	if currentRS != nil && currentRS.UID == rsUID {
	//		continue
	//	}
	//	for _, pod := range podList {
	//		switch pod.Status.Phase {
	//		case v1.PodFailed, v1.PodSucceeded:
	//			// Don't count pods in terminal state.
	//			continue
	//		case v1.PodUnknown:
	//			// v1.PodUnknown is a deprecated status.
	//			// This logic is kept for backward compatibility.
	//			// This used to happen in situation like when the node is temporarily disconnected from the cluster.
	//			// If we can't be sure that the pod is not running, we have to count it.
	//			return true
	//		default:
	//			// Pod is not in terminal phase.
	//			return true
	//		}
	//	}
	//}
	return false
}

func (r *NetworkFunctionReconciler) scaleUpNewReplicaSetForRecreate(ctx context.Context,
	currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet, nf *cachev1alpha1.NetworkFunction,
) (bool, error) {
	scaled, _, err := r.scaleReplicaSet(ctx, currentRS, *(nf.Spec.Replicas), nf, false)
	return scaled, err
}

func (r *NetworkFunctionReconciler) applyRecreate(ctx context.Context,
	nf *cachev1alpha1.NetworkFunction, allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
) error {
	// scale down old replica sets.
	scaledDown, err := r.scaleDownOldReplicaSetsForRecreate(ctx, oldRSs, nf)
	if err != nil || scaledDown {
		return err // Is nil if scaledDown is true
	}

	// Do not process a deployment when it has old pods running.
	podsRunning := oldPodsRunning(currentRS, oldRSs)
	if podsRunning {
		return err // Is nil if podsRunning is true
	}

	// scale up new replica set.
	scaledUp, err := r.scaleUpNewReplicaSetForRecreate(ctx, currentRS, nf)
	if err != nil || scaledUp {
		return err // Is nil if scaledUp is true
	}

	// Check if the deployment is complete and if so, clean up old replica sets.
	if nfutil.NFDeploymentComplete(nf) {
		return r.cleanupOldReplicaSets(ctx, oldRSs, nf)
	}

	return nil
}
