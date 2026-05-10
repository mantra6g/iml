package nf

import (
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetNFCondition(conditionType corev1alpha1.NetworkFunctionConditionType,
	nfStatus *corev1alpha1.NetworkFunctionStatus) *corev1alpha1.NetworkFunctionCondition {
	for _, cond := range nfStatus.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func NewNFCondition(conditionType corev1alpha1.NetworkFunctionConditionType,
	status metav1.ConditionStatus, reason, message string) corev1alpha1.NetworkFunctionCondition {
	return corev1alpha1.NetworkFunctionCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func RemoveNFCondition(nfStatus *corev1alpha1.NetworkFunctionStatus,
	conditionType corev1alpha1.NetworkFunctionConditionType) []corev1alpha1.NetworkFunctionCondition {
	var newConditions []corev1alpha1.NetworkFunctionCondition
	for _, cond := range nfStatus.Conditions {
		if cond.Type != conditionType {
			newConditions = append(newConditions, cond)
		}
	}
	return newConditions
}

func CopyConditions(nfStatus *corev1alpha1.NetworkFunctionStatus,
) []corev1alpha1.NetworkFunctionCondition {
	newConditions := make([]corev1alpha1.NetworkFunctionCondition, 0, len(nfStatus.Conditions))
	for _, cond := range nfStatus.Conditions {
		newConditions = append(newConditions, cond)
	}
	return newConditions
}

func NewScheduledCondition(status metav1.ConditionStatus, reason, message string) corev1alpha1.NetworkFunctionCondition {
	return NewNFCondition(corev1alpha1.NetworkFunctionScheduled, status, reason, message)
}

func UpdateNFCondition(nfStatus *corev1alpha1.NetworkFunctionStatus,
	newCondition corev1alpha1.NetworkFunctionCondition) []corev1alpha1.NetworkFunctionCondition {
	existingCondition := GetNFCondition(newCondition.Type, nfStatus)
	if existingCondition != nil && ConditionsAreEqual(*existingCondition, newCondition) {
		return CopyConditions(nfStatus) // If the status hasn't changed, we don't need to update the LastTransitionTime
	}
	newConditions := RemoveNFCondition(nfStatus, newCondition.Type)
	newConditions = append(newConditions, newCondition)
	return newConditions
}

func GetScheduledCondition(nfStatus *corev1alpha1.NetworkFunctionStatus,
) *corev1alpha1.NetworkFunctionCondition {
	return GetNFCondition(corev1alpha1.NetworkFunctionScheduled, nfStatus)
}

func StatusesAreEqual(oldStatus, newStatus *corev1alpha1.NetworkFunctionStatus) bool {
	if oldStatus == nil || newStatus == nil {
		return oldStatus == nil && newStatus == nil
	}
	if oldStatus.ObservedGeneration != newStatus.ObservedGeneration {
		return false
	}
	if oldStatus.AssignedIP != newStatus.AssignedIP {
		return false
	}
	if oldStatus.Phase != newStatus.Phase {
		return false
	}
	if oldStatus.Reason != newStatus.Reason {
		return false
	}
	return ConditionsSlicesAreEqual(oldStatus.Conditions, newStatus.Conditions)
}

func ConditionsSlicesAreEqual(conds1, conds2 []corev1alpha1.NetworkFunctionCondition) bool {
	if len(conds1) != len(conds2) {
		return false
	}
	// They might be in different order, so we semantically compare them
	for _, cond1 := range conds1 {
		found := false
		var foundCond = &corev1alpha1.NetworkFunctionCondition{}
		for _, cond2 := range conds2 {
			if cond1.Type == cond2.Type {
				found = true
				foundCond = &cond2
			}
		}
		if !found {
			return false
		}
		if !ConditionsAreEqual(cond1, *foundCond) {
			return false
		}
	}
	return true
}

func ConditionsAreEqual(cond1, cond2 corev1alpha1.NetworkFunctionCondition) bool {
	return cond1.Type == cond2.Type &&
		cond1.Status == cond2.Status &&
		cond1.Reason == cond2.Reason &&
		cond1.Message == cond2.Message
}
