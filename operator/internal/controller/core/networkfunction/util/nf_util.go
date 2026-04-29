package util

import (
	schedulingv1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetNFCondition(conditionType schedulingv1alpha1.NetworkFunctionConditionType,
	nfStatus *schedulingv1alpha1.NetworkFunctionStatus) *schedulingv1alpha1.NetworkFunctionCondition {
	for _, cond := range nfStatus.Conditions {
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

func RemoveNFCondition(nfStatus *schedulingv1alpha1.NetworkFunctionStatus,
	conditionType schedulingv1alpha1.NetworkFunctionConditionType) []schedulingv1alpha1.NetworkFunctionCondition {
	var newConditions []schedulingv1alpha1.NetworkFunctionCondition
	for _, cond := range nfStatus.Conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
		}
	}
	return newConditions
}

func CopyConditions(nfStatus *schedulingv1alpha1.NetworkFunctionStatus,
) []schedulingv1alpha1.NetworkFunctionCondition {
	newConditions := make([]schedulingv1alpha1.NetworkFunctionCondition, len(nfStatus.Conditions))
	for _, cond := range nfStatus.Conditions {
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func NewScheduledCondition(status metav1.ConditionStatus, reason, message string) schedulingv1alpha1.NetworkFunctionCondition {
	return NewNFCondition(schedulingv1alpha1.NetworkFunctionScheduled, status, reason, message)
}

func UpdateNFCondition(nfStatus *schedulingv1alpha1.NetworkFunctionStatus,
	newCondition schedulingv1alpha1.NetworkFunctionCondition) []schedulingv1alpha1.NetworkFunctionCondition {
	existingCondition := GetNFCondition(newCondition.Type, nfStatus)
	if existingCondition != nil && existingCondition.Status == newCondition.Status {
		return CopyConditions(nfStatus) // If the status hasn't changed, we don't need to update the LastTransitionTime
	}
	newConditions := RemoveNFCondition(nfStatus, newCondition.Type)
	newConditions = append(newConditions, newCondition)
	return newConditions
}

func GetScheduledCondition(nfStatus *schedulingv1alpha1.NetworkFunctionStatus,
) *schedulingv1alpha1.NetworkFunctionCondition {
	return GetNFCondition(schedulingv1alpha1.NetworkFunctionScheduled, nfStatus)
}
