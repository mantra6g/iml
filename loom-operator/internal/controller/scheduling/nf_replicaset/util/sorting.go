package util

import (
	"math"
	
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
)

var bindingPhaseToOrdinal = map[schedulingv1alpha1.BindingPhase]int{
	schedulingv1alpha1.BindingFailed: 0, schedulingv1alpha1.BindingRunning: 1}

// ActiveBindingsWithRanks is a sortable list of bindings and a list of corresponding
// ranks which will be considered during sorting.  The two lists must have equal
// length.  After sorting, the bindings will be ordered as follows, applying each
// rule in turn until one matches:
//
//  1. If only one of the bindings is assigned to a node, the binding that is not
//     assigned comes before the binding that is.
//  2. If the bindings' phases differ, a pending binding comes before a binding whose phase
//     is unknown, and a binding whose phase is unknown comes before a running binding.
//  3. If exactly one of the bindings is ready, the binding that is not ready comes
//     before the ready binding.
//  4. If controller.kubernetes.io/binding-deletion-cost annotation is set, then
//     the binding with the lower value will come first.
//  5. If the bindings' ranks differ, the binding with greater rank comes before the binding
//     with lower rank.
//  6. If both bindings are ready but have not been ready for the same amount of
//     time, the binding that has been ready for a shorter amount of time comes
//     before the binding that has been ready for longer.
//  7. If one binding has a container that has restarted more than any container in
//     the other binding, the binding with the container with more restarts comes
//     before the other binding.
//  8. If the bindings' creation times differ, the binding that was created more recently
//     comes before the older binding.
//
// In 6 and 8, times are compared in a logarithmic scale. This allows a level
// of randomness among equivalent Bindings when sorting. If two bindings have the same
// logarithmic rank, they are sorted by UUID to provide a pseudorandom order.
//
// If none of these rules matches, the second binding comes before the first binding.
//
// The intention of this ordering is to put bindings that should be preferred for
// deletion first in the list.
type ActiveBindingsWithRanks struct {
	// Bindings is a list of bindings.
	Bindings []*schedulingv1alpha1.NetworkFunctionBinding

	// Rank is a ranking of bindings.  This ranking is used during sorting when
	// comparing two bindings that are both scheduled, in the same phase, and
	// having the same ready status.
	Rank []int

	// Now is a reference timestamp for doing logarithmic timestamp comparisons.
	// If zero, comparison happens without scaling.
	Now metav1.Time
}

func (s ActiveBindingsWithRanks) Len() int {
	return len(s.Bindings)
}

func (s ActiveBindingsWithRanks) Swap(i, j int) {
	s.Bindings[i], s.Bindings[j] = s.Bindings[j], s.Bindings[i]
	s.Rank[i], s.Rank[j] = s.Rank[j], s.Rank[i]
}

// Less compares two bindings with corresponding ranks and returns true if the first
// one should be preferred for deletion.
func (s ActiveBindingsWithRanks) Less(i, j int) bool {
	// 1. Unassigned < assigned
	// If only one of the bindings is unassigned, the unassigned one is smaller
	if s.Bindings[i].Spec.TargetName != s.Bindings[j].Spec.TargetName &&
		(len(s.Bindings[i].Spec.TargetName) == 0 || len(s.Bindings[j].Spec.TargetName) == 0) {
		return len(s.Bindings[i].Spec.TargetName) == 0
	}
	// 2. BindingPending < BindingUnknown < BindingRunning
	if bindingPhaseToOrdinal[s.Bindings[i].Status.Phase] != bindingPhaseToOrdinal[s.Bindings[j].Status.Phase] {
		return bindingPhaseToOrdinal[s.Bindings[i].Status.Phase] < bindingPhaseToOrdinal[s.Bindings[j].Status.Phase]
	}
	// 3. Not ready < ready
	// If only one of the bindings is not ready, the not ready one is smaller
	if IsBindingReady(s.Bindings[i]) != IsBindingReady(s.Bindings[j]) {
		return !IsBindingReady(s.Bindings[i])
	}
	// 4. Doubled up < not doubled up
	// If one of the two bindings is on the same node as one or more additional
	// ready bindings that belong to the same replicaset, whichever binding has more
	// colocated ready bindings is less
	if s.Rank[i] != s.Rank[j] {
		return s.Rank[i] > s.Rank[j]
	}
	// TODO: take availability into account when we push minReadySeconds information from deployment into bindings,
	//       see https://github.com/kubernetes/kubernetes/issues/22065
	// 5. Been ready for empty time < less time < more time
	// If both bindings are ready, the latest ready one is smaller
	if IsBindingReady(s.Bindings[i]) && IsBindingReady(s.Bindings[j]) {
		readyTime1 := bindingReadyTime(s.Bindings[i])
		readyTime2 := bindingReadyTime(s.Bindings[j])
		if !readyTime1.Equal(readyTime2) {
			if s.Now.IsZero() || readyTime1.IsZero() || readyTime2.IsZero() {
				return afterOrZero(readyTime1, readyTime2)
			}
			rankDiff := logarithmicRankDiff(*readyTime1, *readyTime2, s.Now)
			if rankDiff == 0 {
				return s.Bindings[i].UID < s.Bindings[j].UID
			}
			return rankDiff < 0
		}
	}
	// TODO: 6. Bindings with with higher restart counts < lower restart counts
	// 7. Empty creation time bindings < newer bindings < older bindings
	if !s.Bindings[i].CreationTimestamp.Equal(&s.Bindings[j].CreationTimestamp) {
		if s.Now.IsZero() || s.Bindings[i].CreationTimestamp.IsZero() || s.Bindings[j].CreationTimestamp.IsZero() {
			return afterOrZero(&s.Bindings[i].CreationTimestamp, &s.Bindings[j].CreationTimestamp)
		}
		rankDiff := logarithmicRankDiff(s.Bindings[i].CreationTimestamp, s.Bindings[j].CreationTimestamp, s.Now)
		if rankDiff == 0 {
			return s.Bindings[i].UID < s.Bindings[j].UID
		}
		return rankDiff < 0
	}
	return false
}

// afterOrZero checks if time t1 is after time t2; if one of them
// is zero, the zero time is seen as after non-zero time.
func afterOrZero(t1, t2 *metav1.Time) bool {
	if t1.Time.IsZero() || t2.Time.IsZero() {
		return t1.Time.IsZero()
	}
	return t1.After(t2.Time)
}

// logarithmicRankDiff calculates the base-2 logarithmic ranks of 2 timestamps,
// compared to the current timestamp
func logarithmicRankDiff(t1, t2, now metav1.Time) int64 {
	d1 := now.Sub(t1.Time)
	d2 := now.Sub(t2.Time)
	r1 := int64(-1)
	r2 := int64(-1)
	if d1 > 0 {
		r1 = int64(math.Log2(float64(d1)))
	}
	if d2 > 0 {
		r2 = int64(math.Log2(float64(d2)))
	}
	return r1 - r2
}

func bindingReadyTime(binding *schedulingv1alpha1.NetworkFunctionBinding) *metav1.Time {
	if IsBindingReady(binding) {
		for _, c := range binding.Status.Conditions {
			// we only care about binding ready conditions
			if c.Type == schedulingv1alpha1.BindingReady && c.Status == metav1.ConditionTrue {
				return &c.LastTransitionTime
			}
		}
	}
	return &metav1.Time{}
}
