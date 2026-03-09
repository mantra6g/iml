package networkfunction

import (
	cachev1alpha1 "builder/api/cache/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	nfutil "builder/internal/controller/cache/networkfunction/util"
	"context"
	"fmt"
	"sort"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *NetworkFunctionReconciler) applyRollingUpdate(ctx context.Context,
	nf *cachev1alpha1.NetworkFunction, allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
) error {
	scaledUp, err := r.reconcileNewReplicaSet(ctx, allRSs, currentRS, nf)
	if err != nil || scaledUp {
		return err
	}
	scaledDown, err := r.reconcileOldReplicaSets(ctx, allRSs, oldRSs, currentRS, nf)
	if err != nil || scaledDown {
		return err
	}

	if nfutil.NFDeploymentComplete(nf) {
		return r.cleanupOldReplicaSets(ctx, oldRSs, nf)
	}
	return nil
}

func (r *NetworkFunctionReconciler) cleanupUnhealthyReplicas(ctx context.Context,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, nf *cachev1alpha1.NetworkFunction, maxCleanupCount int32,
) (remainingOldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, cleanupCount int32, err error) {
	logger := logf.FromContext(ctx)
	sort.Sort(nfutil.ReplicaSetsByCreationTimestamp(oldRSs))
	// Safely scale down all old replica sets with unhealthy replicas. Replica set will sort the pods in the order
	// such that not-ready < ready, unscheduled < scheduled, and pending < running. This ensures that unhealthy
	// replicas will be deleted first and won't increase unavailability.
	totalScaledDown := int32(0)
	for i, targetRS := range oldRSs {
		if totalScaledDown >= maxCleanupCount {
			break
		}
		if *(targetRS.Spec.Replicas) == 0 {
			// cannot scale down this replica set.
			continue
		}
		logger.V(4).Info("Found available pods in old RS",
			"replicaSet", targetRS, "availableReplicas", targetRS.Status.AvailableReplicas)
		if *(targetRS.Spec.Replicas) == targetRS.Status.AvailableReplicas {
			// no unhealthy replicas found, no scaling required.
			continue
		}

		scaledDownCount := min(maxCleanupCount-totalScaledDown, *(targetRS.Spec.Replicas)-targetRS.Status.AvailableReplicas)
		newReplicasCount := *(targetRS.Spec.Replicas) - scaledDownCount
		if newReplicasCount > *(targetRS.Spec.Replicas) {
			return nil, 0,
				fmt.Errorf("when cleaning up unhealthy replicas, got invalid request to scale down %s/%s %d -> %d",
					targetRS.Namespace, targetRS.Name, *(targetRS.Spec.Replicas), newReplicasCount)
		}
		_, updatedOldRS, err := r.scaleReplicaSet(ctx, targetRS, newReplicasCount, nf, false)
		if err != nil {
			return nil, totalScaledDown, err
		}
		totalScaledDown += scaledDownCount
		oldRSs[i] = updatedOldRS
	}
	return oldRSs, totalScaledDown, nil
}

func (r *NetworkFunctionReconciler) reconcileNewReplicaSet(ctx context.Context,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	nf *cachev1alpha1.NetworkFunction,
) (bool, error) {
	if *(currentRS.Spec.Replicas) == *(nf.Spec.Replicas) {
		// Scaling not required.
		return false, nil
	}
	if *(currentRS.Spec.Replicas) > *(nf.Spec.Replicas) {
		// Scale down.
		scaled, _, err := r.scaleReplicaSet(ctx, currentRS, *(nf.Spec.Replicas), nf, false)
		return scaled, err
	}
	newReplicasCount, err := nfutil.NewRSNewReplicas(nf, allRSs, currentRS)
	if err != nil {
		return false, err
	}
	scaled, _, err := r.scaleReplicaSet(ctx, currentRS, newReplicasCount, nf, false)
	return scaled, err
}

func (r *NetworkFunctionReconciler) reconcileOldReplicaSets(ctx context.Context,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	currentRS *schedulingv1alpha1.NetworkFunctionReplicaSet, nf *cachev1alpha1.NetworkFunction,
) (bool, error) {
	logger := logf.FromContext(ctx)
	oldPodsCount := nfutil.GetReplicaCountForReplicaSets(oldRSs)
	if oldPodsCount == 0 {
		// Can't scale down further
		return false, nil
	}
	allPodsCount := nfutil.GetReplicaCountForReplicaSets(allRSs)
	logger.V(4).Info("New replica set",
		"replicaSet", currentRS, "availableReplicas", currentRS.Status.AvailableReplicas)
	maxUnavailable := nfutil.MaxUnavailable(nf)

	// Check if we can scale down. We can scale down in the following 2 cases:
	// * Some old replica sets have unhealthy replicas, we could safely scale down those unhealthy replicas since
	//  that won't further increase unavailability.
	// * New replica set has scaled up and its replicas become ready, then we can scale down old replica sets
	//  in a further step.
	//
	// maxScaledDown := allPodsCount - minAvailable - newReplicaSetPodsUnavailable
	// take into account not only maxUnavailable and any surge pods that have been created, but also unavailable pods from
	// the newRS, so that the unavailable pods from the newRS would not make us scale down old replica sets in a further
	// step(that will increase unavailability).
	//
	// Concrete example:
	//
	// * 10 replicas
	// * 2 maxUnavailable (absolute number, not percent)
	// * 3 maxSurge (absolute number, not percent)
	//
	// case 1:
	// * Deployment is updated, newRS is created with 3 replicas, oldRS is scaled down to 8, and newRS is scaled up to 5.
	// * The new replica set pods crashloop and never become available.
	// * allPodsCount is 13. minAvailable is 8. newRSPodsUnavailable is 5.
	// * A node fails and causes one of the oldRS pods to become unavailable. However, 13 - 8 - 5 = 0, so the oldRS won't be scaled down.
	// * The user notices the crashloop and does kubectl rollout undo to rollback.
	// * newRSPodsUnavailable is 1, since we rolled back to the good replica set, so maxScaledDown = 13 - 8 - 1 = 4. 4 of the crashlooping pods will be scaled down.
	// * The total number of pods will then be 9 and the newRS can be scaled up to 10.
	//
	// case 2:
	// Same example, but pushing a new pod template instead of rolling back (aka "roll over"):
	// * The new replica set created must start with 0 replicas because allPodsCount is already at 13.
	// * However, newRSPodsUnavailable would also be 0, so the 2 old replica sets could be scaled down by 5 (13 - 8 - 0), which would then
	// allow the new replica set to be scaled up by 5.
	minAvailable := *(nf.Spec.Replicas) - maxUnavailable
	newRSUnavailablePodCount := *(currentRS.Spec.Replicas) - currentRS.Status.AvailableReplicas
	maxScaledDown := allPodsCount - minAvailable - newRSUnavailablePodCount
	if maxScaledDown <= 0 {
		return false, nil
	}

	// Clean up unhealthy replicas first, otherwise unhealthy replicas will block deployment
	// and cause timeout. See https://github.com/kubernetes/kubernetes/issues/16737
	oldRSs, cleanupCount, err := r.cleanupUnhealthyReplicas(ctx, oldRSs, nf, maxScaledDown)
	if err != nil {
		return false, nil
	}
	logger.V(4).Info("Cleaned up unhealthy replicas from old RSes", "count", cleanupCount)

	// Scale down old replica sets, need check maxUnavailable to ensure we can scale down
	allRSs = append(oldRSs, currentRS)
	scaledDownCount, err := r.scaleDownOldReplicaSetsForRollingUpdate(ctx, allRSs, oldRSs, nf)
	if err != nil {
		return false, nil
	}
	logger.V(4).Info("Scaled down old RSes",
		"NetworkFunction", nf, "count", scaledDownCount)

	totalScaledDown := cleanupCount + scaledDownCount
	return totalScaledDown > 0, nil
}

func (r *NetworkFunctionReconciler) scaleDownOldReplicaSetsForRollingUpdate(ctx context.Context,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	nf *cachev1alpha1.NetworkFunction,
) (scaledDownCount int32, err error) {
	logger := logf.FromContext(ctx)
	maxUnavailable := nfutil.MaxUnavailable(nf)

	// Check if we can scale down.
	minAvailable := *(nf.Spec.Replicas) - maxUnavailable
	// Find the number of available pods.
	availableReplicaCount := nfutil.GetAvailableReplicaCountForReplicaSets(allRSs)
	if availableReplicaCount <= minAvailable {
		// Cannot scale down.
		return 0, nil
	}
	logger.V(4).Info("Found available pods in deployment, scaling down old RSes",
		"NetworkFunction", nf, "availableReplicas", availableReplicaCount)

	sort.Sort(nfutil.ReplicaSetsByCreationTimestamp(oldRSs))

	totalScaledDown := int32(0)
	totalScaleDownCount := availableReplicaCount - minAvailable
	for _, targetRS := range oldRSs {
		if totalScaledDown >= totalScaleDownCount {
			// No further scaling required.
			break
		}
		if *(targetRS.Spec.Replicas) == 0 {
			// cannot scale down this ReplicaSet.
			continue
		}
		// Scale down.
		scaleDownCount := min(*(targetRS.Spec.Replicas), totalScaleDownCount-totalScaledDown)
		newReplicasCount := *(targetRS.Spec.Replicas) - scaleDownCount
		if newReplicasCount > *(targetRS.Spec.Replicas) {
			return 0,
				fmt.Errorf("when scaling down old RS, got invalid request to scale down %s/%s %d -> %d",
					targetRS.Namespace, targetRS.Name, *(targetRS.Spec.Replicas), newReplicasCount)
		}
		_, _, err := r.scaleReplicaSet(ctx, targetRS, newReplicasCount, nf, false)
		if err != nil {
			return totalScaledDown, err
		}

		totalScaledDown += scaleDownCount
	}

	return totalScaledDown, nil
}
