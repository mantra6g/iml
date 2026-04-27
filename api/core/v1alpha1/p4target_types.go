/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const P4TargetLeaseNamespace = "p4target-leases"

const P4TargetArchitectureLabel = "p4target.loom.io/arch"
const P4TargetNameLabel = "p4target.loom.io/name"

type TaintEffect string

const (
	// TaintEffectNoSchedule does not allow new nfs to schedule onto the target
	// unless they tolerate the taint, but allow all nfs submitted to Kubelet
	// without going through the scheduler to start, and allow all already-running
	// nfs to continue running. Enforced by the scheduler.
	TaintEffectNoSchedule TaintEffect = "NoSchedule"
	// TaintEffectPreferNoSchedule is like TaintEffectNoSchedule, but the scheduler
	// tries not to schedule new nfs onto the node, rather than prohibiting new
	// nfs from scheduling onto the node entirely. Enforced by the scheduler.
	TaintEffectPreferNoSchedule TaintEffect = "PreferNoSchedule"
	// TaintEffectNoExecute evicts any already-running nfs that do not tolerate
	// the taint. Currently enforced by NodeController.
	TaintEffectNoExecute TaintEffect = "NoExecute"
)

type Taint struct {
	Key       string      `json:"key"`
	Value     string      `json:"value,omitempty"`
	Effect    TaintEffect `json:"effect"`
	TimeAdded metav1.Time `json:"timeAdded,omitempty"`
}

// P4TargetSpec defines the desired state of P4Target
type P4TargetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Taints represents the taints applied to the P4 target, which can affect
	// scheduling and operation of network functions on it.
	// +optional
	Taints []Taint `json:"taints"`

	// Unschedulable indicates whether the P4 target is unschedulable for new network functions.
	// +optional
	Unschedulable bool `json:"unschedulable,omitempty"`

	// NfCIDR is the range assigned to the network functions running on this target.
	// +optional
	NfCIDR string `json:"nfCIDR,omitempty"`
}

// Taints that can be applied to P4Targets to indicate their state or
// conditions that may affect scheduling or operation of network functions on them.
const (
	TaintP4TargetUnreachable   = "p4target.loom.io/unreachable"
	TaintP4TargetNotReady      = "p4target.loom.io/not-ready"
	TaintP4TargetUnschedulable = "p4target.loom.io/unschedulable"
	TaintP4TargetOutOfService  = "p4target.loom.io/out-of-service"
)

type P4TargetConditionType string

const (
	P4TargetConditionReady P4TargetConditionType = "Ready"
)

// P4TargetCondition represents the state of a specific aspect of the P4 target,
// such as its readiness or health status.
type P4TargetCondition struct {
	Type               P4TargetConditionType  `json:"type"`
	Status             metav1.ConditionStatus `json:"status"`
	LastHeartbeatTime  metav1.Time            `json:"lastHeartbeatTime,omitempty,omitzero"`
	LastTransitionTime metav1.Time            `json:"lastTransitionTime,omitempty,omitzero"`
	Reason             string                 `json:"reason,omitempty,omitzero"`
	Message            string                 `json:"message,omitempty,omitzero"`
}

// P4TargetStatus defines the observed state of P4Target.
type P4TargetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// NodeName is the name of the Kubernetes node that the P4 target is associated with, if any.
	NodeName string `json:"nodeName,omitempty"`

	// TargetIPs are the detected IPs of the programmable target. They can be either in-cluster or public-facing IPs.
	TargetIPs []string `json:"targetIPs,omitempty"`

	// DriverIPs are the detected IPs that were assigned to the programmable target's driver.
	DriverIPs []string `json:"driverIPs,omitempty"`

	// Capacity represents the total resources of the P4 target
	Capacity corev1.ResourceList `json:"capacity,omitempty"`

	// Allocatable represents the resources of the P4 target that are available for allocation
	Allocatable corev1.ResourceList `json:"allocatable,omitempty"`

	Conditions []P4TargetCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// P4Target is the Schema for the p4targets API
type P4Target struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of P4Target
	// +required
	Spec P4TargetSpec `json:"spec"`

	// status defines the observed state of P4Target
	// +optional
	Status P4TargetStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// P4TargetList contains a list of P4Target
type P4TargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []P4Target `json:"items"`
}

func init() {
	SchemeBuilder.Register(&P4Target{}, &P4TargetList{})
}
