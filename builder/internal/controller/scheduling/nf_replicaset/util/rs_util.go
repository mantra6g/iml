package util

import (
	"fmt"
	"sort"
	"time"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	"loom/pkg/util/ptr"

	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func IsBindingReady(b *schedulingv1alpha1.NetworkFunctionBinding) bool {
	cond := GetBindingCondition(b, schedulingv1alpha1.BindingReady)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func IsBindingAvailable(b *schedulingv1alpha1.NetworkFunctionBinding, minReadySeconds int32, now time.Time) bool {
	readyCondition := GetBindingCondition(b, schedulingv1alpha1.BindingReady)
	if readyCondition == nil || readyCondition.Status != metav1.ConditionTrue {
		return false
	}
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	timeSinceReady := now.Sub(readyCondition.LastTransitionTime.Time)
	if timeSinceReady < minReadySecondsDuration {
		return false
	}
	return true
}

func GetBindingCondition(b *schedulingv1alpha1.NetworkFunctionBinding,
	conditionType schedulingv1alpha1.BindingConditionType) *schedulingv1alpha1.BindingCondition {
	for _, cond := range b.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func GetBindingLabelSet(template *schedulingv1alpha1.NetworkFunctionBindingTemplate) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.Labels {
		desiredLabels[k] = v
	}
	return desiredLabels
}

func GetBindingFinalizers(template *schedulingv1alpha1.NetworkFunctionBindingTemplate) []string {
	desiredFinalizers := make([]string, len(template.Finalizers))
	copy(desiredFinalizers, template.Finalizers)
	return desiredFinalizers
}

func GetBindingAnnotationSet(template *schedulingv1alpha1.NetworkFunctionBindingTemplate) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

func GetBindingPrefix(rsName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", rsName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = rsName
	}
	return prefix
}

func GetBindingsToDelete(filteredBdgs, relatedBdgs []*schedulingv1alpha1.NetworkFunctionBinding, diff int,
) []*schedulingv1alpha1.NetworkFunctionBinding {
	// No need to sort pods if we are about to delete all of them.
	// diff will always be <= len(filteredBdgs), so not need to handle > case.
	if diff < len(filteredBdgs) {
		podsWithRanks := GetBindingsRankedByRelatedBindingsOnSameTarget(filteredBdgs, relatedBdgs)
		sort.Sort(podsWithRanks)
	}
	return filteredBdgs[:diff]
}

// GetBindingsRankedByRelatedBindingsOnSameTarget returns an ActiveBindingsWithRanks value
// that wraps bindingsToRank and assigns each binding a rank equal to the number of
// active binding in relatedBindings that are colocated on the same node with the pod.
// relatedBindings generally should be a superset of bindingsToRank.
func GetBindingsRankedByRelatedBindingsOnSameTarget(
	bindingsToRank, relatedBindings []*schedulingv1alpha1.NetworkFunctionBinding,
) ActiveBindingsWithRanks {
	bindingsOnTarget := make(map[string]int)
	for _, b := range relatedBindings {
		if IsBindingActive(b) {
			bindingsOnTarget[b.Spec.TargetName]++
		}
	}
	ranks := make([]int, len(bindingsToRank))
	for i, b := range bindingsToRank {
		ranks[i] = bindingsOnTarget[b.Spec.TargetName]
	}
	return ActiveBindingsWithRanks{Bindings: bindingsToRank, Rank: ranks, Now: metav1.Now()}
}

func GetBindingKeys(pods []*schedulingv1alpha1.NetworkFunctionBinding) []string {
	podKeys := make([]string, 0, len(pods))
	for _, pod := range pods {
		podKeys = append(podKeys, BindingKey(pod))
	}
	return podKeys
}

func IsBindingActive(b *schedulingv1alpha1.NetworkFunctionBinding) bool {
	return schedulingv1alpha1.BindingFailed != b.Status.Phase &&
		b.DeletionTimestamp == nil
}

// FilterActiveBindings returns bindings that have not terminated.
func FilterActiveBindings(
	bindings []*schedulingv1alpha1.NetworkFunctionBinding,
) []*schedulingv1alpha1.NetworkFunctionBinding {
	var result []*schedulingv1alpha1.NetworkFunctionBinding
	for _, b := range bindings {
		if IsBindingActive(b) {
			result = append(result, b)
		}
	}
	return result
}

func GetCondition(
	status schedulingv1alpha1.NetworkFunctionReplicaSetStatus,
	condType schedulingv1alpha1.ReplicaSetConditionType,
) *schedulingv1alpha1.ReplicaSetCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == condType {
			return &status.Conditions[i]
		}
	}
	return nil
}

func SetCondition(
	status *schedulingv1alpha1.NetworkFunctionReplicaSetStatus,
	condition schedulingv1alpha1.ReplicaSetCondition) {
	preExistingCondition := GetCondition(*status, condition.Type)
	if preExistingCondition != nil && EqualConditions(&condition, preExistingCondition) {
		// No update is needed since the condition is the same as the pre-existing one.
		return
	} else if preExistingCondition != nil { // We are updating an existing condition
		// Keep the last transition time of the pre-existing condition
		condition.LastTransitionTime = preExistingCondition.LastTransitionTime
		// Remove the pre-existing condition before adding the updated one.
		RemoveCondition(status, condition.Type)
	} else { // We are adding a new condition
		// Check if the transition time exists, and if it doesn't, set the transition time to now.
		if condition.LastTransitionTime.IsZero() {
			condition.LastTransitionTime = metav1.Now()
		}
	}
	status.Conditions = append(status.Conditions, condition)
}

func RemoveCondition(
	status *schedulingv1alpha1.NetworkFunctionReplicaSetStatus,
	condType schedulingv1alpha1.ReplicaSetConditionType) {
	newConditions := make([]schedulingv1alpha1.ReplicaSetCondition, 0, len(status.Conditions)-1)
	for i := range status.Conditions {
		if status.Conditions[i].Type != condType {
			newConditions = append(newConditions, status.Conditions[i])
		}
	}
	status.Conditions = newConditions
}

func NewReplicaSetCondition(
	condType schedulingv1alpha1.ReplicaSetConditionType,
	status metav1.ConditionStatus, reason, message string,
) schedulingv1alpha1.ReplicaSetCondition {
	return schedulingv1alpha1.ReplicaSetCondition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

// EqualConditions returns true if the two conditions are equal, ignoring the LastTransitionTime field.
func EqualConditions(c1, c2 *schedulingv1alpha1.ReplicaSetCondition) bool {
	if c1 == nil && c2 == nil {
		return true
	}
	if c1 == nil || c2 == nil {
		return false
	}
	return c1.Type == c2.Type &&
		c1.Status == c2.Status &&
		c1.Reason == c2.Reason &&
		c1.Message == c2.Message
}

func CountReplicas(rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
	activeBindings []*schedulingv1alpha1.NetworkFunctionBinding,
	now time.Time,
) (fullyLabeledReplicasCount, readyReplicasCount, availableReplicasCount int) {
	fullyLabeledReplicasCount = 0
	readyReplicasCount = 0
	availableReplicasCount = 0
	templateLabel := labels.Set(rs.Spec.Template.Labels).AsSelectorPreValidated()
	for _, b := range activeBindings {
		if templateLabel.Matches(labels.Set(b.Labels)) {
			fullyLabeledReplicasCount++
		}
		if IsBindingReady(b) {
			readyReplicasCount++
			if IsBindingAvailable(b, rs.Spec.MinReadySeconds, now) {
				availableReplicasCount++
			}
		}
	}
	return fullyLabeledReplicasCount, readyReplicasCount, availableReplicasCount
}

// FindMinNextBindingAvailabilityCheck finds a duration when the next availability check should occur.
// We should check for the availability at the same time as the status evaluation/update occurs (e.g. .status.availableReplicas) by
// passing lastOwnerStatusEvaluation. This ensures that we will not skip any pods that might become available
// (findMinNextPodAvailabilitySimpleCheck would return nil in the future time), since the owner status evaluation.
// clock is then used to calculate the precise time for the next availability check.
func FindMinNextBindingAvailabilityCheck(bindings []*schedulingv1alpha1.NetworkFunctionBinding,
	minReadySeconds int32, lastOwnerStatusEvaluation time.Time,
) *time.Duration {
	nextCheckAccordingToOwnerStatusEvaluation, checkPod := findMinNextBindingAvailabilitySimpleCheck(bindings,
		minReadySeconds, lastOwnerStatusEvaluation)
	if nextCheckAccordingToOwnerStatusEvaluation == nil || checkPod == nil {
		return nil
	}
	// There must be a nextCheck. We try to calculate a more precise value for the next availability check.
	// Check the earliest binding to avoid being preempted by a later binding.
	if updatedNextCheck := nextBindingAvailabilityCheck(checkPod, minReadySeconds, time.Now()); updatedNextCheck != nil {
		// There is a delay since the last Now() call (lastOwnerStatusEvaluation). Use the updatedNextCheck.
		return updatedNextCheck
	}
	// Fall back to 0 (immediate check) in case the last nextBindingAvailabilityCheck call (with a refreshed Now)
	// returns nil, as we might be past the check.
	return ptr.To(time.Duration(0))
}

// findMinNextBindingAvailabilitySimpleCheck finds a duration when the next availability check should occur.
// It also returns the first binding affected by the future availability recalculation (there might be more
// bindings if they became ready at the same time; this helps to implement FindMinNextBindingAvailabilityCheck).
func findMinNextBindingAvailabilitySimpleCheck(bindings []*schedulingv1alpha1.NetworkFunctionBinding,
	minReadySeconds int32, now time.Time) (*time.Duration, *schedulingv1alpha1.NetworkFunctionBinding) {
	var minAvailabilityCheck *time.Duration
	var checkBinding *schedulingv1alpha1.NetworkFunctionBinding
	for _, p := range bindings {
		nextCheck := nextBindingAvailabilityCheck(p, minReadySeconds, now)
		if nextCheck != nil && (minAvailabilityCheck == nil || *nextCheck < *minAvailabilityCheck) {
			minAvailabilityCheck = nextCheck
			checkBinding = p
		}
	}
	return minAvailabilityCheck, checkBinding
}

// nextBindingAvailabilityCheck implements similar logic to IsBindingAvailable
func nextBindingAvailabilityCheck(binding *schedulingv1alpha1.NetworkFunctionBinding, minReadySeconds int32,
	now time.Time) *time.Duration {
	if !IsBindingReady(binding) || minReadySeconds <= 0 {
		return nil
	}

	c := GetBindingReadyCondition(&binding.Status)
	if c == nil {
		return nil
	}
	if c.LastTransitionTime.IsZero() {
		return nil
	}
	minReadySecondsDuration := time.Duration(minReadySeconds) * time.Second
	nextCheck := c.LastTransitionTime.Add(minReadySecondsDuration).Sub(now)
	if nextCheck > 0 {
		return ptr.To(nextCheck)
	}
	return nil
}

func GetBindingReadyCondition(status *schedulingv1alpha1.NetworkFunctionBindingStatus,
) *schedulingv1alpha1.BindingCondition {
	for _, cond := range status.Conditions {
		if cond.Type == schedulingv1alpha1.BindingReady {
			return &cond
		}
	}
	return nil
}
