package p4target

import (
	"github.com/mantra6g/iml/api/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IsTargetReady returns true if a target is ready; false otherwise.
func IsTargetReady(target *v1alpha1.P4Target) bool {
	for _, c := range target.Status.Conditions {
		if c.Type == v1alpha1.P4_TARGET_CONDITION_READY {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}
