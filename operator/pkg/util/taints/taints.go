package taints

import "github.com/mantra6g/iml/api/core/v1alpha1"

// TaintKeyExists checks if the given taint key exists in list of taints. Returns true if exists false otherwise.
func TaintKeyExists(taints []v1alpha1.Taint, taintKeyToMatch string) bool {
	for _, taint := range taints {
		if taint.Key == taintKeyToMatch {
			return true
		}
	}
	return false
}
