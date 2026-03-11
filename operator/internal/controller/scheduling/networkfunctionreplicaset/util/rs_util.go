package util

import (
	"fmt"
	"loom/api/core/v1alpha1"
	"sort"
	"time"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	"loom/pkg/util/ptr"

	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func IsNFReady(nf *v1alpha1.NetworkFunction) bool {
	cond := GetNFCondition(nf, v1alpha1.NetworkFunctionReady)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

func IsNFAvailable(nf *v1alpha1.NetworkFunction, minReadySeconds int32, now time.Time) bool {
	readyCondition := GetNFCondition(nf, v1alpha1.NetworkFunctionReady)
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

func GetNFCondition(nf *v1alpha1.NetworkFunction,
	conditionType v1alpha1.NetworkFunctionConditionType) *v1alpha1.NetworkFunctionCondition {
	for _, cond := range nf.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func GetNFLabelSet(template *v1alpha1.NetworkFunctionTemplate) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.Labels {
		desiredLabels[k] = v
	}
	return desiredLabels
}

func GetNFFinalizers(template *v1alpha1.NetworkFunctionTemplate) []string {
	desiredFinalizers := make([]string, len(template.Finalizers))
	copy(desiredFinalizers, template.Finalizers)
	return desiredFinalizers
}

func GetNFAnnotationSet(template *v1alpha1.NetworkFunctionTemplate) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

func GetNFPrefix(rsName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", rsName)
	if len(validation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = rsName
	}
	return prefix
}

func GetNFsToDelete(filteredNFs, relatedNFs []*v1alpha1.NetworkFunction, diff int,
) []*v1alpha1.NetworkFunction {
	// No need to sort pods if we are about to delete all of them.
	// diff will always be <= len(filteredNFs), so not need to handle > case.
	if diff < len(filteredNFs) {
		podsWithRanks := GetNFsRankedByRelatedNFsOnSameTarget(filteredNFs, relatedNFs)
		sort.Sort(podsWithRanks)
	}
	return filteredNFs[:diff]
}

// GetNFsRankedByRelatedNFsOnSameTarget returns an ActiveNFsWithRanks value
// that wraps nfsToRank and assigns each nf a rank equal to the number of
// active nf in relatedNFs that are colocated on the same node with the pod.
// relatedNFs generally should be a superset of nfsToRank.
func GetNFsRankedByRelatedNFsOnSameTarget(
	nfsToRank, relatedNFs []*v1alpha1.NetworkFunction,
) ActiveNFsWithRanks {
	nfsOnTarget := make(map[string]int)
	for _, nf := range relatedNFs {
		if IsNFActive(nf) {
			nfsOnTarget[nf.Spec.TargetName]++
		}
	}
	ranks := make([]int, len(nfsToRank))
	for i, b := range nfsToRank {
		ranks[i] = nfsOnTarget[b.Spec.TargetName]
	}
	return ActiveNFsWithRanks{NFs: nfsToRank, Rank: ranks, Now: metav1.Now()}
}

func GetNFKeys(pods []*v1alpha1.NetworkFunction) []string {
	podKeys := make([]string, 0, len(pods))
	for _, pod := range pods {
		podKeys = append(podKeys, NFKey(pod))
	}
	return podKeys
}

func IsNFActive(b *v1alpha1.NetworkFunction) bool {
	return v1alpha1.NetworkFunctionFailed != b.Status.Phase &&
		b.DeletionTimestamp == nil
}

// FilterActiveNFs returns nfs that have not terminated.
func FilterActiveNFs(
	nfs []*v1alpha1.NetworkFunction,
) []*v1alpha1.NetworkFunction {
	var result []*v1alpha1.NetworkFunction
	for _, nf := range nfs {
		if IsNFActive(nf) {
			result = append(result, nf)
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
	activeNFs []*v1alpha1.NetworkFunction,
	now time.Time,
) (fullyLabeledReplicasCount, readyReplicasCount, availableReplicasCount int) {
	fullyLabeledReplicasCount = 0
	readyReplicasCount = 0
	availableReplicasCount = 0
	templateLabel := labels.Set(rs.Spec.Template.Labels).AsSelectorPreValidated()
	for _, b := range activeNFs {
		if templateLabel.Matches(labels.Set(b.Labels)) {
			fullyLabeledReplicasCount++
		}
		if IsNFReady(b) {
			readyReplicasCount++
			if IsNFAvailable(b, rs.Spec.MinReadySeconds, now) {
				availableReplicasCount++
			}
		}
	}
	return fullyLabeledReplicasCount, readyReplicasCount, availableReplicasCount
}

// FindMinNextNFAvailabilityCheck finds a duration when the next availability check should occur.
// We should check for the availability at the same time as the status evaluation/update occurs (e.g. .status.availableReplicas) by
// passing lastOwnerStatusEvaluation. This ensures that we will not skip any pods that might become available
// (findMinNextPodAvailabilitySimpleCheck would return nil in the future time), since the owner status evaluation.
// clock is then used to calculate the precise time for the next availability check.
func FindMinNextNFAvailabilityCheck(nfs []*v1alpha1.NetworkFunction,
	minReadySeconds int32, lastOwnerStatusEvaluation time.Time,
) *time.Duration {
	nextCheckAccordingToOwnerStatusEvaluation, checkPod := findMinNextNFAvailabilitySimpleCheck(nfs,
		minReadySeconds, lastOwnerStatusEvaluation)
	if nextCheckAccordingToOwnerStatusEvaluation == nil || checkPod == nil {
		return nil
	}
	// There must be a nextCheck. We try to calculate a more precise value for the next availability check.
	// Check the earliest nf to avoid being preempted by a later nf.
	if updatedNextCheck := nextNFAvailabilityCheck(checkPod, minReadySeconds, time.Now()); updatedNextCheck != nil {
		// There is a delay since the last Now() call (lastOwnerStatusEvaluation). Use the updatedNextCheck.
		return updatedNextCheck
	}
	// Fall back to 0 (immediate check) in case the last nextNFAvailabilityCheck call (with a refreshed Now)
	// returns nil, as we might be past the check.
	return ptr.To(time.Duration(0))
}

// findMinNextNFAvailabilitySimpleCheck finds a duration when the next availability check should occur.
// It also returns the first nf affected by the future availability recalculation (there might be more
// nfs if they became ready at the same time; this helps to implement FindMinNextNFAvailabilityCheck).
func findMinNextNFAvailabilitySimpleCheck(nfs []*v1alpha1.NetworkFunction,
	minReadySeconds int32, now time.Time) (*time.Duration, *v1alpha1.NetworkFunction) {
	var minAvailabilityCheck *time.Duration
	var checkNF *v1alpha1.NetworkFunction
	for _, p := range nfs {
		nextCheck := nextNFAvailabilityCheck(p, minReadySeconds, now)
		if nextCheck != nil && (minAvailabilityCheck == nil || *nextCheck < *minAvailabilityCheck) {
			minAvailabilityCheck = nextCheck
			checkNF = p
		}
	}
	return minAvailabilityCheck, checkNF
}

// nextNFAvailabilityCheck implements similar logic to IsNFAvailable
func nextNFAvailabilityCheck(nf *v1alpha1.NetworkFunction, minReadySeconds int32,
	now time.Time) *time.Duration {
	if !IsNFReady(nf) || minReadySeconds <= 0 {
		return nil
	}

	c := GetNFReadyCondition(&nf.Status)
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

func GetNFReadyCondition(status *v1alpha1.NetworkFunctionStatus,
) *v1alpha1.NetworkFunctionCondition {
	for _, cond := range status.Conditions {
		if cond.Type == v1alpha1.NetworkFunctionReady {
			return &cond
		}
	}
	return nil
}
