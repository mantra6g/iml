package networkfunctiondeployment

import (
	"context"

	corev1alpha1 "loom/api/core/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	deploymentutil "loom/internal/controller/core/networkfunctiondeployment/util"
)

func (r *NetworkFunctionDeploymentReconciler) scaleDownOldReplicaSetsForRecreate(ctx context.Context,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, nfDeployment *corev1alpha1.NetworkFunctionDeployment,
) (bool, error) {
	scaled := false
	for i := range oldRSs {
		rs := oldRSs[i]
		// Scaling not required.
		if *(rs.Spec.Replicas) == 0 {
			continue
		}
		scaledRS, updatedRS, err := r.scaleReplicaSet(ctx, rs, 0, nfDeployment, false)
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
	if oldPods := deploymentutil.GetActualReplicaCountForReplicaSets(oldRSs); oldPods > 0 {
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

func (r *NetworkFunctionDeploymentReconciler) scaleUpNewReplicaSetForRecreate(ctx context.Context,
	currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet, nfDeployment *corev1alpha1.NetworkFunctionDeployment,
) (bool, error) {
	scaled, _, err := r.scaleReplicaSet(ctx, currentRS, *(nfDeployment.Spec.Replicas), nfDeployment, false)
	return scaled, err
}

func (r *NetworkFunctionDeploymentReconciler) applyRecreate(ctx context.Context,
	nfDeployment *corev1alpha1.NetworkFunctionDeployment, allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
) error {
	// scale down old replica sets.
	scaledDown, err := r.scaleDownOldReplicaSetsForRecreate(ctx, oldRSs, nfDeployment)
	if err != nil || scaledDown {
		return err // Is nil if scaledDown is true
	}

	// Do not process a deployment when it has old pods running.
	podsRunning := oldPodsRunning(currentRS, oldRSs)
	if podsRunning {
		return err // Is nil if podsRunning is true
	}

	// scale up new replica set.
	scaledUp, err := r.scaleUpNewReplicaSetForRecreate(ctx, currentRS, nfDeployment)
	if err != nil || scaledUp {
		return err // Is nil if scaledUp is true
	}

	// Check if the deployment is complete and if so, clean up old replica sets.
	if deploymentutil.NFDeploymentComplete(nfDeployment) {
		return r.cleanupOldReplicaSets(ctx, oldRSs, nfDeployment)
	}

	return nil
}
