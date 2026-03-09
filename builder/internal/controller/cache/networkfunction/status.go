package networkfunction

import (
	cachev1alpha1 "builder/api/cache/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	nfutil "builder/internal/controller/cache/networkfunction/util"
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *NetworkFunctionReconciler) updateNFStatus(ctx context.Context,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, newRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	nf *cachev1alpha1.NetworkFunction) error {
	newStatus := calculateStatus(allRSs, newRS, nf)

	original := nf.DeepCopy()
	nf.Status = newStatus

	err := r.Client.Status().Patch(ctx, nf, client.MergeFrom(original))
	return err
}

func calculateStatus(allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	newRS *schedulingv1alpha1.NetworkFunctionReplicaSet, nf *cachev1alpha1.NetworkFunction,
) cachev1alpha1.NetworkFunctionStatus {
	updatedReplicas := int32(0)
	if newRS != nil {
		updatedReplicas = nfutil.GetActualReplicaCountForReplicaSets([]*schedulingv1alpha1.NetworkFunctionReplicaSet{newRS})
	}
	availableReplicas := nfutil.GetAvailableReplicaCountForReplicaSets(allRSs)
	totalReplicas := nfutil.GetReplicaCountForReplicaSets(allRSs)
	unavailableReplicas := totalReplicas - availableReplicas
	// If unavailableReplicas is negative, then that means the Deployment has more available replicas running than
	// desired, e.g. whenever it scales down. In such a case we should simply default unavailableReplicas to zero.
	if unavailableReplicas < 0 {
		unavailableReplicas = 0
	}

	status := cachev1alpha1.NetworkFunctionStatus{
		ObservedGeneration:  nf.Generation,
		Replicas:            nfutil.GetActualReplicaCountForReplicaSets(allRSs),
		UpdatedReplicas:     updatedReplicas,
		ReadyReplicas:       nfutil.GetReadyReplicaCountForReplicaSets(allRSs),
		AvailableReplicas:   availableReplicas,
		UnavailableReplicas: unavailableReplicas,
	}
	// Copy conditions from old status
	for i := range nf.Status.Conditions {
		status.Conditions = append(status.Conditions, nf.Status.Conditions[i])
	}
	if availableReplicas >= *(nf.Spec.Replicas)-nfutil.MaxUnavailable(nf) {
		minAvailability := nfutil.NewNfCondition(
			cachev1alpha1.NFAvailable, metav1.ConditionTrue,
			nfutil.MinimumReplicasAvailable, "Network Function has minimum availability.")
		nfutil.SetNfCondition(&status, *minAvailability)
	} else {
		noMinAvailability := nfutil.NewNfCondition(
			cachev1alpha1.NFAvailable, metav1.ConditionFalse,
			nfutil.MinimumReplicasUnavailable, "Network Function does not have minimum availability.")
		nfutil.SetNfCondition(&status, *noMinAvailability)
	}

	return status
}
