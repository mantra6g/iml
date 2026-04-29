package servicechain

import corev1alpha1 "iml-daemon/api/core/v1alpha1"

func GetReadyCondition(nf *corev1alpha1.NetworkFunction) *corev1alpha1.NetworkFunctionCondition {
	for _, condition := range nf.Status.Conditions {
		if condition.Type == "Ready" {
			return &condition
		}
	}
	return nil
}
