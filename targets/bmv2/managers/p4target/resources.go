package p4target

import (
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Resource names for P4Target capacity and allocatable reporting.
// These are the shared contract between the P4TargetManager and the NF manager's preCheck.
const (
	ResourceNFSlots      corev1.ResourceName = "loom.io/nf-slots"
	ResourceTableEntries corev1.ResourceName = "loom.io/table-entries"
)

// Additional P4TargetCondition types beyond the Ready condition defined in the API.
const (
	ConditionHealthy           corev1alpha1.P4TargetConditionType = "Healthy"
	ConditionNetworkConfigured corev1alpha1.P4TargetConditionType = "NetworkConfigured"
	ConditionOccupied          corev1alpha1.P4TargetConditionType = "Occupied"
)
