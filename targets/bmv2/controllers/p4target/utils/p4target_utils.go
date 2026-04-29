package utils

import (
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func StatusChanged(original, target *corev1alpha1.P4Target) bool {
	if original == nil || target == nil {
		return true
	}
	if ResourceListsChanged(original.Status.Capacity, target.Status.Capacity) {
		return true
	}
	if ResourceListsChanged(original.Status.Allocatable, target.Status.Allocatable) {
		return true
	}
	return ConditionsChanged(original.Status.Conditions, target.Status.Conditions)
}

func ResourceListsChanged(a, b corev1.ResourceList) bool {
	if len(a) != len(b) {
		return true
	}
	for key, valA := range a {
		valB, exists := b[key]
		if !exists {
			return true
		}

		if valA.Cmp(valB) != 0 {
			return true
		}
	}
	return false
}

func ConditionsChanged(a, b []corev1alpha1.P4TargetCondition) bool {
	if len(a) != len(b) {
		return true
	}
	mapA := make(map[corev1alpha1.P4TargetConditionType]corev1alpha1.P4TargetCondition, len(a))
	for _, c := range a {
		mapA[c.Type] = c
	}
	for _, cb := range b {
		ca, exists := mapA[cb.Type]
		if !exists {
			return true
		}
		// Compare meaningful fields
		if ConditionChanged(ca, cb) {
			return true
		}
	}
	return false
}

func ConditionChanged(original, target corev1alpha1.P4TargetCondition) bool {
	return original.Status != target.Status ||
		original.Reason != target.Reason ||
		original.Message != target.Message
}
