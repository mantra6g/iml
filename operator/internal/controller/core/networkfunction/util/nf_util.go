package util

import (
	schedulingv1alpha1 "github.com/mantra6g/iml/operator/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetNFCondition(conditionType schedulingv1alpha1.NetworkFunctionConditionType,
	nf *schedulingv1alpha1.NetworkFunction) *schedulingv1alpha1.NetworkFunctionCondition {
	for _, cond := range nf.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func NewNFCondition(conditionType schedulingv1alpha1.NetworkFunctionConditionType,
	status metav1.ConditionStatus, reason, message string) schedulingv1alpha1.NetworkFunctionCondition {
	return schedulingv1alpha1.NetworkFunctionCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func RemoveNFCondition(nf *schedulingv1alpha1.NetworkFunction,
	conditionType schedulingv1alpha1.NetworkFunctionConditionType) []schedulingv1alpha1.NetworkFunctionCondition {
	var newConditions []schedulingv1alpha1.NetworkFunctionCondition
	for _, cond := range nf.Status.Conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
		}
	}
	return newConditions
}

func CopyConditions(nf *schedulingv1alpha1.NetworkFunction,
) []schedulingv1alpha1.NetworkFunctionCondition {
	newConditions := make([]schedulingv1alpha1.NetworkFunctionCondition, len(nf.Status.Conditions))
	for _, cond := range nf.Status.Conditions {
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func NewScheduledCondition(status metav1.ConditionStatus, reason, message string) schedulingv1alpha1.NetworkFunctionCondition {
	return NewNFCondition(schedulingv1alpha1.NetworkFunctionScheduled, status, reason, message)
}

func UpdateNFCondition(nf *schedulingv1alpha1.NetworkFunction,
	newCondition schedulingv1alpha1.NetworkFunctionCondition) []schedulingv1alpha1.NetworkFunctionCondition {
	existingCondition := GetNFCondition(newCondition.Type, nf)
	if existingCondition != nil && existingCondition.Status == newCondition.Status {
		return CopyConditions(nf) // If the status hasn't changed, we don't need to update the LastTransitionTime
	}
	newConditions := RemoveNFCondition(nf, newCondition.Type)
	newConditions = append(newConditions, newCondition)
	return newConditions
}

func GetScheduledCondition(nf *schedulingv1alpha1.NetworkFunction,
) *schedulingv1alpha1.NetworkFunctionCondition {
	return GetNFCondition(schedulingv1alpha1.NetworkFunctionScheduled, nf)
}
