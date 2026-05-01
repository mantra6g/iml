package nf

import (
	"github.com/mantra6g/iml/api/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateNFCondition updates existing nf condition or creates a new one. Sets LastTransitionTime to now if the
// status has changed.
// Returns true if nf condition has changed or has been added.
func UpdateNFCondition(status *v1alpha1.NetworkFunctionStatus, condition *v1alpha1.NetworkFunctionCondition) bool {
	condition.LastTransitionTime = metav1.Now()
	// Try to find this nf condition.
	conditionIndex, oldCondition := GetNFCondition(status, condition.Type)

	if oldCondition == nil {
		// We are adding new nf condition.
		status.Conditions = append(status.Conditions, *condition)
		return true
	}
	// We are updating an existing condition, so we need to check if it has changed.
	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	isEqual := condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason &&
		condition.Message == oldCondition.Message &&
		condition.LastProbeTime.Equal(&oldCondition.LastProbeTime) &&
		condition.LastTransitionTime.Equal(&oldCondition.LastTransitionTime)

	status.Conditions[conditionIndex] = *condition
	// Return true if one of the fields have changed.
	return !isEqual
}

// GetNFCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func GetNFCondition(status *v1alpha1.NetworkFunctionStatus, conditionType v1alpha1.NetworkFunctionConditionType) (int, *v1alpha1.NetworkFunctionCondition) {
	if status == nil {
		return -1, nil
	}
	return GetNFConditionFromList(status.Conditions, conditionType)
}

// GetNFConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func GetNFConditionFromList(conditions []v1alpha1.NetworkFunctionCondition, conditionType v1alpha1.NetworkFunctionConditionType) (int, *v1alpha1.NetworkFunctionCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}
