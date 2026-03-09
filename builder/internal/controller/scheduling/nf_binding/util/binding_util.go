package util

import (
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetBindingCondition(conditionType schedulingv1alpha1.BindingConditionType,
	binding *schedulingv1alpha1.NetworkFunctionBinding) *schedulingv1alpha1.BindingCondition {
	for _, cond := range binding.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func NewBindingCondition(conditionType schedulingv1alpha1.BindingConditionType,
	status metav1.ConditionStatus, reason, message string) schedulingv1alpha1.BindingCondition {
	return schedulingv1alpha1.BindingCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func RemoveBindingCondition(networkFunctionBinding *schedulingv1alpha1.NetworkFunctionBinding,
	conditionType schedulingv1alpha1.BindingConditionType) []schedulingv1alpha1.BindingCondition {
	var newConditions []schedulingv1alpha1.BindingCondition
	for _, cond := range networkFunctionBinding.Status.Conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
		}
	}
	return newConditions
}

func CopyConditions(networkFunctionBinding *schedulingv1alpha1.NetworkFunctionBinding,
) []schedulingv1alpha1.BindingCondition {
	newConditions := make([]schedulingv1alpha1.BindingCondition, len(networkFunctionBinding.Status.Conditions))
	for _, cond := range networkFunctionBinding.Status.Conditions {
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func NewScheduledCondition(status metav1.ConditionStatus, reason, message string) schedulingv1alpha1.BindingCondition {
	return NewBindingCondition(schedulingv1alpha1.BindingScheduled, status, reason, message)
}

func UpdateBindingCondition(binding *schedulingv1alpha1.NetworkFunctionBinding,
	newCondition schedulingv1alpha1.BindingCondition) []schedulingv1alpha1.BindingCondition {
	existingCondition := GetBindingCondition(newCondition.Type, binding)
	if existingCondition != nil && existingCondition.Status == newCondition.Status {
		return CopyConditions(binding) // If the status hasn't changed, we don't need to update the LastTransitionTime
	}
	newConditions := RemoveBindingCondition(binding, newCondition.Type)
	newConditions = append(newConditions, newCondition)
	return newConditions
}

func GetScheduledCondition(networkFunctionBinding *schedulingv1alpha1.NetworkFunctionBinding,
) *schedulingv1alpha1.BindingCondition {
	return GetBindingCondition(schedulingv1alpha1.BindingScheduled, networkFunctionBinding)
}
