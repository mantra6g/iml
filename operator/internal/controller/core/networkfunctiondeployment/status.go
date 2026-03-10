package networkfunctiondeployment

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "loom/api/core/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	deploymentutil "loom/internal/controller/core/networkfunctiondeployment/util"
)

func (r *NetworkFunctionDeploymentReconciler) updateNFDeploymentStatus(ctx context.Context,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, newRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	nfDeployment *corev1alpha1.NetworkFunctionDeployment) error {
	newStatus := calculateStatus(allRSs, newRS, nfDeployment)

	original := nfDeployment.DeepCopy()
	nfDeployment.Status = newStatus

	err := r.Client.Status().Patch(ctx, nfDeployment, client.MergeFrom(original))
	return err
}

func calculateStatus(allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	newRS *schedulingv1alpha1.NetworkFunctionReplicaSet, nfDeployment *corev1alpha1.NetworkFunctionDeployment,
) corev1alpha1.NetworkFunctionDeploymentStatus {
	updatedReplicas := int32(0)
	if newRS != nil {
		updatedReplicas = deploymentutil.GetActualReplicaCountForReplicaSets([]*schedulingv1alpha1.NetworkFunctionReplicaSet{newRS})
	}
	availableReplicas := deploymentutil.GetAvailableReplicaCountForReplicaSets(allRSs)
	totalReplicas := deploymentutil.GetReplicaCountForReplicaSets(allRSs)
	unavailableReplicas := totalReplicas - availableReplicas
	// If unavailableReplicas is negative, then that means the Deployment has more available replicas running than
	// desired, e.g. whenever it scales down. In such a case we should simply default unavailableReplicas to zero.
	if unavailableReplicas < 0 {
		unavailableReplicas = 0
	}

	status := corev1alpha1.NetworkFunctionDeploymentStatus{
		ObservedGeneration:  nfDeployment.Generation,
		Replicas:            deploymentutil.GetActualReplicaCountForReplicaSets(allRSs),
		UpdatedReplicas:     updatedReplicas,
		ReadyReplicas:       deploymentutil.GetReadyReplicaCountForReplicaSets(allRSs),
		AvailableReplicas:   availableReplicas,
		UnavailableReplicas: unavailableReplicas,
	}
	// Copy conditions from old status
	for i := range nfDeployment.Status.Conditions {
		status.Conditions = append(status.Conditions, nfDeployment.Status.Conditions[i])
	}
	if availableReplicas >= *(nfDeployment.Spec.Replicas)-deploymentutil.MaxUnavailable(nfDeployment) {
		minAvailability := deploymentutil.NewNfDeploymentCondition(
			corev1alpha1.NFDeploymentAvailable, metav1.ConditionTrue,
			deploymentutil.MinimumReplicasAvailable, "Network Function has minimum availability.")
		deploymentutil.SetNfDeploymentCondition(&status, *minAvailability)
	} else {
		noMinAvailability := deploymentutil.NewNfDeploymentCondition(
			corev1alpha1.NFDeploymentAvailable, metav1.ConditionFalse,
			deploymentutil.MinimumReplicasUnavailable, "Network Function does not have minimum availability.")
		deploymentutil.SetNfDeploymentCondition(&status, *noMinAvailability)
	}

	return status
}
