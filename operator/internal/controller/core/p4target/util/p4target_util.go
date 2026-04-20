package util

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

// NewReadyCondition creates a new Condition of type Ready with the given status, reason, and message.
// The condition type is set to corev1alpha1.P4_TARGET_CONDITION_READY,
// and the LastTransitionTime is set to the current time.
func NewReadyCondition(status metav1.ConditionStatus, reason, message string) corev1alpha1.P4TargetCondition {
	return NewCondition(
		corev1alpha1.P4_TARGET_CONDITION_READY,
		status, reason, message)
}

// NewCondition creates a new Condition with the given type, status, reason, and message.
// The LastTransitionTime is set to the current time.
func NewCondition(
	conditionType corev1alpha1.P4TargetConditionType, status metav1.ConditionStatus, reason, message string,
) corev1alpha1.P4TargetCondition {
	return corev1alpha1.P4TargetCondition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func GetReadyCondition(p4target *corev1alpha1.P4Target) *corev1alpha1.P4TargetCondition {
	for i := range p4target.Status.Conditions {
		if p4target.Status.Conditions[i].Type == corev1alpha1.P4_TARGET_CONDITION_READY {
			return &p4target.Status.Conditions[i]
		}
	}
	return nil
}

// ConditionsAreEqual compares two conditions and returns true if they are equal, and false otherwise.
// It compares the Type, Status, Reason, and Message fields of the conditions,
// but ignores the LastTransitionTime field since that can change even if the
// condition is effectively the same.
func ConditionsAreEqual(cond1, cond2 corev1alpha1.P4TargetCondition) bool {
	return cond1.Type == cond2.Type &&
		cond1.Status == cond2.Status &&
		cond1.Reason == cond2.Reason &&
		cond1.Message == cond2.Message
}

// TaintsAreEqual compares two taints and returns true if they are equal, and false otherwise.
// It compares the Key, Value, and Effect fields of the taints, but ignores the TimeAdded field
// since that can change even if the taint is effectively the same.
func TaintsAreEqual(taint1, taint2 corev1alpha1.Taint) bool {
	return taint1.Key == taint2.Key &&
		taint1.Value == taint2.Value &&
		taint1.Effect == taint2.Effect
}

// AddTaints adds the given taints to the P4Target if they don't already exist,
// or updates them if they do exist but are different.
// It returns true if any taints were added or updated, and false otherwise.
func AddTaints(
	existingTaints []corev1alpha1.Taint, newTaints ...corev1alpha1.Taint,
) (updatedTaints []corev1alpha1.Taint, updated bool) {
	if existingTaints == nil || len(existingTaints) == 0 {
		return newTaints, len(newTaints) > 0
	}
	updatedTaints = make([]corev1alpha1.Taint, len(existingTaints))
	copy(updatedTaints, existingTaints)
	for i := range newTaints {
		// Check if the taint already exists
		existingTaint := GetTaint(updatedTaints, newTaints[i].Key)
		// If the taint already exists and is different, then we should update it with the new values
		if existingTaint != nil && !TaintsAreEqual(*existingTaint, newTaints[i]) {
			existingTaint.Value = newTaints[i].Value
			existingTaint.Effect = newTaints[i].Effect
			existingTaint.TimeAdded = newTaints[i].TimeAdded
			updated = true
			continue
		}
		// If the taint doesn't exist, then we should add it to the P4Target
		existingTaints = append(existingTaints, newTaints[i])
		updated = true
	}
	return updatedTaints, updated
}

// RemoveTaints removes the taints with the given keys from the P4Target if they exist.
// It returns true if any taints were removed, and false otherwise.
func RemoveTaints(taints []corev1alpha1.Taint, taintKeys ...string) (newTaints []corev1alpha1.Taint, updated bool) {
	if len(taints) == 0 {
		return []corev1alpha1.Taint{}, false
	}
	newTaints = make([]corev1alpha1.Taint, 0)
	for i := range taints {
		toRemove := false
		for j := range taintKeys {
			if taints[i].Key == taintKeys[j] {
				toRemove = true
				break
			}
		}
		if !toRemove {
			updated = true
			newTaints = append(newTaints, taints[i])
		}
	}
	return newTaints, updated
}

// GetTaint returns a pointer to the taint with the given key if it exists on the slice, and nil otherwise.
func GetTaint(taints []corev1alpha1.Taint, taintKey string) *corev1alpha1.Taint {
	for i := range taints {
		if taints[i].Key == taintKey {
			return &taints[i]
		}
	}
	return nil
}

// HasTaintWithEffect returns true if the P4Target has a taint with the effect
// passed as an argument, and false otherwise.
func HasTaintWithEffect(p4target *corev1alpha1.P4Target, effect corev1alpha1.TaintEffect) bool {
	return HasTaintWithEffects(p4target, effect)
}

// HasTaintWithEffects returns true if the P4Target has at least one taint with any of the effects
// passed as an argument, and false otherwise.
func HasTaintWithEffects(p4target *corev1alpha1.P4Target, effects ...corev1alpha1.TaintEffect) bool {
	for i := range p4target.Spec.Taints {
		for j := range effects {
			if p4target.Spec.Taints[i].Effect == effects[j] {
				return true
			}
		}
	}
	return false
}

func GetCondition(
	conditions []corev1alpha1.P4TargetCondition, conditionType corev1alpha1.P4TargetConditionType,
) *corev1alpha1.P4TargetCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func AddConditions(
	conditions []corev1alpha1.P4TargetCondition, newConditions ...corev1alpha1.P4TargetCondition,
) (updatedConditions []corev1alpha1.P4TargetCondition, updated bool) {
	if len(conditions) == 0 {
		return newConditions, len(newConditions) > 0
	}
	updatedConditions = make([]corev1alpha1.P4TargetCondition, len(conditions))
	copy(updatedConditions, conditions)
	for i := range newConditions {
		previousCondition := GetCondition(updatedConditions, newConditions[i].Type)
		if previousCondition == nil {
			// If the condition doesn't exist, then we should add it to the P4Target
			updatedConditions = append(updatedConditions, newConditions[i])
			updated = true
		} else if !ConditionsAreEqual(*previousCondition, newConditions[i]) {
			// If the condition already exists and is different, then we should update it with the new values
			previousCondition.Status = newConditions[i].Status
			previousCondition.Reason = newConditions[i].Reason
			previousCondition.Message = newConditions[i].Message
			previousCondition.LastTransitionTime = newConditions[i].LastTransitionTime
			updated = true
		}
	}
	return updatedConditions, updated
}

func RemoveConditions(
	conditions []corev1alpha1.P4TargetCondition, conditionTypes ...corev1alpha1.P4TargetConditionType,
) (newConditions []corev1alpha1.P4TargetCondition, updated bool) {
	if len(conditions) == 0 {
		return []corev1alpha1.P4TargetCondition{}, false
	}
	newConditions = make([]corev1alpha1.P4TargetCondition, 0)
	for i := range conditions {
		toRemove := false
		for j := range conditionTypes {
			if conditions[i].Type == conditionTypes[j] {
				toRemove = true
				break
			}
		}
		if !toRemove {
			updated = true
			newConditions = append(newConditions, conditions[i])
		}
	}
	return newConditions, updated
}
