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

const P4TargetLeaseNamespace = "loom-system"

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

// SRv6Mode selects who applies the SRv6 End behavior for the network functions running on a P4 target.
// +kubebuilder:validation:Enum=Native;Proxy
type SRv6Mode string

const (
	// SRv6ModeNative means the target or its network functions update the SRv6 headers themselves.
	SRv6ModeNative SRv6Mode = "Native"
	// SRv6ModeProxy means the host applies the End behavior on behalf of the network functions.
	SRv6ModeProxy SRv6Mode = "Proxy"
)

type SRv6Config struct {
	// Mode selects who applies the SRv6 End behavior. Either way, IPv6 must be enabled
	// on the P4 target for SRv6 to work.
	// +kubebuilder:default=Native
	// +optional
	Mode SRv6Mode `json:"mode,omitempty"`
}

// HostInterfaceStrategy defines how the host-side interfaces connecting to a P4 target are obtained.
// +kubebuilder:validation:Enum=Create;Reuse
type HostInterfaceStrategy string

const (
	// HostInterfaceStrategyCreate makes the daemon create the host-side interfaces.
	HostInterfaceStrategyCreate HostInterfaceStrategy = "Create"
	// HostInterfaceStrategyReuse makes the daemon attach to already existing host-side interfaces.
	HostInterfaceStrategyReuse HostInterfaceStrategy = "Reuse"
)

// +kubebuilder:validation:XValidation:rule="self.strategy == 'Reuse' ? has(self.reuse) && !has(self.create) : !has(self.reuse)",message="reuse must be set if and only if strategy is Reuse"
type HostInterfaces struct {
	// Strategy defines whether the daemon creates new host-side interfaces or reuses existing ones.
	// +kubebuilder:default=Create
	// +optional
	Strategy HostInterfaceStrategy `json:"strategy,omitempty"`

	// Create configures the host-side interfaces created by the daemon. Only used when Strategy is Create.
	// +optional
	Create *CreateHostInterfaces `json:"create,omitempty"`

	// Reuse lists the existing host-side interfaces to attach to. Only used when Strategy is Reuse.
	// +optional
	Reuse *ReuseHostInterfaces `json:"reuse,omitempty"`
}

type CreateHostInterfaces struct {
	// Count is the number of host-side interfaces that will connect to the P4 target's namespace.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Count int32 `json:"count,omitempty"`
}

type ReuseHostInterfaces struct {
	// Names of the existing host-side interfaces used to route the traffic to the target.
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	// +required
	Names []string `json:"names"`
}

type P4TargetNetwork struct {
	// SRv6 configures how SRv6 is handled for the network functions running on the target.
	// +optional
	SRv6 *SRv6Config `json:"srv6,omitempty"`

	// HostInterfaces configures the host-side interfaces connecting to the target.
	// Defaults to a single interface created by the daemon. It is immutable, since the daemon
	// cannot change the interfaces of a running target.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="hostInterfaces is immutable"
	// +optional
	HostInterfaces *HostInterfaces `json:"hostInterfaces,omitempty"`
}

// P4TargetSpec defines the desired state of P4Target
// +kubebuilder:validation:XValidation:rule="(has(self.network) && has(self.network.hostInterfaces)) == (has(oldSelf.network) && has(oldSelf.network.hostInterfaces))",message="network.hostInterfaces cannot be added or removed after creation"
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

	// Network configures how the host connects to the P4 target and handles its SRv6 traffic.
	// +optional
	Network *P4TargetNetwork `json:"network,omitempty"`
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
// +kubebuilder:resource:scope=Cluster,shortName={targets,target}
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

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
