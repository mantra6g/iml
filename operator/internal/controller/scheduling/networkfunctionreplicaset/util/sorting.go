package util

import (
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
)

var nfPhaseToOrdinal = map[schedulingv1alpha1.NetworkFunctionPhase]int{
	schedulingv1alpha1.NetworkFunctionFailed: 0, schedulingv1alpha1.NetworkFunctionRunning: 1}

// ActiveNFsWithRanks is a sortable list of nfs and a list of corresponding
// ranks which will be considered during sorting.  The two lists must have equal
// length.  After sorting, the nfs will be ordered as follows, applying each
// rule in turn until one matches:
//
//  1. If only one of the nfs is assigned to a node, the nf that is not
//     assigned comes before the nf that is.
//  2. If the nfs' phases differ, a pending nf comes before a nf whose phase
//     is unknown, and a nf whose phase is unknown comes before a running nf.
//  3. If exactly one of the nfs is ready, the nf that is not ready comes
//     before the ready nf.
//  4. If controller.kubernetes.io/nf-deletion-cost annotation is set, then
//     the nf with the lower value will come first.
//  5. If the nfs' ranks differ, the nf with greater rank comes before the nf
//     with lower rank.
//  6. If both nfs are ready but have not been ready for the same amount of
//     time, the nf that has been ready for a shorter amount of time comes
//     before the nf that has been ready for longer.
//  7. If one nf has a container that has restarted more than any container in
//     the other nf, the nf with the container with more restarts comes
//     before the other nf.
//  8. If the nfs' creation times differ, the nf that was created more recently
//     comes before the older nf.
//
// In 6 and 8, times are compared in a logarithmic scale. This allows a level
// of randomness among equivalent NFs when sorting. If two nfs have the same
// logarithmic rank, they are sorted by UUID to provide a pseudorandom order.
//
// If none of these rules matches, the second nf comes before the first nf.
//
// The intention of this ordering is to put nfs that should be preferred for
// deletion first in the list.
type ActiveNFsWithRanks struct {
	// NFs is a list of nfs.
	NFs []*schedulingv1alpha1.NetworkFunction

	// Rank is a ranking of nfs.  This ranking is used during sorting when
	// comparing two nfs that are both scheduled, in the same phase, and
	// having the same ready status.
	Rank []int

	// Now is a reference timestamp for doing logarithmic timestamp comparisons.
	// If zero, comparison happens without scaling.
	Now metav1.Time
}

func (s ActiveNFsWithRanks) Len() int {
	return len(s.NFs)
}

func (s ActiveNFsWithRanks) Swap(i, j int) {
	s.NFs[i], s.NFs[j] = s.NFs[j], s.NFs[i]
	s.Rank[i], s.Rank[j] = s.Rank[j], s.Rank[i]
}

// Less compares two nfs with corresponding ranks and returns true if the first
// one should be preferred for deletion.
func (s ActiveNFsWithRanks) Less(i, j int) bool {
	// 1. Unassigned < assigned
	// If only one of the nfs is unassigned, the unassigned one is smaller
	if s.NFs[i].Spec.TargetName != s.NFs[j].Spec.TargetName &&
		(len(s.NFs[i].Spec.TargetName) == 0 || len(s.NFs[j].Spec.TargetName) == 0) {
		return len(s.NFs[i].Spec.TargetName) == 0
	}
	// 2. NetworkFunctionPending < NetworkFunctionRunning
	if nfPhaseToOrdinal[s.NFs[i].Status.Phase] != nfPhaseToOrdinal[s.NFs[j].Status.Phase] {
		return nfPhaseToOrdinal[s.NFs[i].Status.Phase] < nfPhaseToOrdinal[s.NFs[j].Status.Phase]
	}
	// 3. Not ready < ready
	// If only one of the nfs is not ready, the not ready one is smaller
	if IsNFReady(s.NFs[i]) != IsNFReady(s.NFs[j]) {
		return !IsNFReady(s.NFs[i])
	}
	// 4. Doubled up < not doubled up
	// If one of the two nfs is on the same node as one or more additional
	// ready nfs that belong to the same replicaset, whichever nf has more
	// colocated ready nfs is less
	if s.Rank[i] != s.Rank[j] {
		return s.Rank[i] > s.Rank[j]
	}
	// TODO: take availability into account when we push minReadySeconds information from deployment into nfs,
	//       see https://github.com/kubernetes/kubernetes/issues/22065
	// 5. Been ready for empty time < less time < more time
	// If both nfs are ready, the latest ready one is smaller
	if IsNFReady(s.NFs[i]) && IsNFReady(s.NFs[j]) {
		readyTime1 := nfReadyTime(s.NFs[i])
		readyTime2 := nfReadyTime(s.NFs[j])
		if !readyTime1.Equal(readyTime2) {
			if s.Now.IsZero() || readyTime1.IsZero() || readyTime2.IsZero() {
				return afterOrZero(readyTime1, readyTime2)
			}
			rankDiff := logarithmicRankDiff(*readyTime1, *readyTime2, s.Now)
			if rankDiff == 0 {
				return s.NFs[i].UID < s.NFs[j].UID
			}
			return rankDiff < 0
		}
	}
	// TODO: 6. NFs with with higher restart counts < lower restart counts
	// 7. Empty creation time nfs < newer nfs < older nfs
	if !s.NFs[i].CreationTimestamp.Equal(&s.NFs[j].CreationTimestamp) {
		if s.Now.IsZero() || s.NFs[i].CreationTimestamp.IsZero() || s.NFs[j].CreationTimestamp.IsZero() {
			return afterOrZero(&s.NFs[i].CreationTimestamp, &s.NFs[j].CreationTimestamp)
		}
		rankDiff := logarithmicRankDiff(s.NFs[i].CreationTimestamp, s.NFs[j].CreationTimestamp, s.Now)
		if rankDiff == 0 {
			return s.NFs[i].UID < s.NFs[j].UID
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

func nfReadyTime(nf *schedulingv1alpha1.NetworkFunction) *metav1.Time {
	if IsNFReady(nf) {
		for _, c := range nf.Status.Conditions {
			// we only care about nf ready conditions
			if c.Type == schedulingv1alpha1.NetworkFunctionReady && c.Status == metav1.ConditionTrue {
				return &c.LastTransitionTime
			}
		}
	}
	return &metav1.Time{}
}
