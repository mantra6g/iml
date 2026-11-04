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

	// Attachment describes how the target is attached to loom's dataplane on its node.
	// +optional
	Attachment P4TargetAttachment `json:"attachment,omitempty,omitzero"`
}

// P4TargetAttachmentType selects how the daemon steers the target's NfCIDR traffic towards it.
// +kubebuilder:validation:Enum=Container;HostVRF
type P4TargetAttachmentType string

const (
	// P4TargetAttachmentContainer attaches a containerized target through loom's CNI. See P4TargetAttachment.Type.
	P4TargetAttachmentContainer P4TargetAttachmentType = "Container"

	// P4TargetAttachmentHostVRF attaches the target as a daemon-owned host VRF. See P4TargetAttachment.Type.
	P4TargetAttachmentHostVRF P4TargetAttachmentType = "HostVRF"
)

// P4TargetAttachment describes how the target is attached to loom's dataplane on its node.
type P4TargetAttachment struct {
	// Type is the kind of attachment.
	//
	// Container (default): the target is a container attached to the node's routing bridge through
	// loom's CNI, and the daemon routes the NfCIDR towards status.targetIPs.
	//
	// HostVRF: the target is a VRF in the host network namespace. The daemon creates and owns the VRF,
	// connects it to the routing VRF with a veth pair, and reports it in status.hostVRF. It routes the
	// NfCIDR into the VRF, installs an unreachable route for the NfCIDR inside it (so unprogrammed
	// addresses are dropped instead of looping) and a default route back to the routing VRF.
	// Contract for the plugin: everything inside the VRF is the plugin's business, and everything wired
	// to the VRF shares its lifetime. When the VRF is torn down (P4Target deletion or daemon restart)
	// the daemon deletes every interface whose chain of masters leads to the VRF (e.g. the ports of a
	// bridge inside the VRF), releasing physical ones instead, and flushes its routing table. The plugin
	// must therefore wire every interface it creates to the VRF, need not clean them up itself, and must
	// re-create its interfaces and routes whenever the VRF is re-created (e.g. when status.hostVRF
	// changes). Interfaces in the VRF other than the daemon's veth must have a conntrack zone rule
	// matching status.hostVRF.table, added by the plugin.
	// +kubebuilder:default=Container
	// +optional
	Type P4TargetAttachmentType `json:"type,omitempty"`
}

// P4TargetHostVRFStatus reports the host VRF the daemon created for a HostVRF-attached target.
type P4TargetHostVRFStatus struct {
	// Name is the name of the VRF device.
	Name string `json:"name"`

	// Table is the routing table bound to the VRF.
	Table uint32 `json:"table"`

	// Interface is the VRF-side end of the veth pair connecting the VRF to the routing VRF.
	Interface string `json:"interface"`

	// Addresses are the IPs of Interface.
	// +optional
	Addresses []string `json:"addresses,omitempty"`

	// Gateways are the IPs of the routing-VRF side of the veth pair, used as the VRF's default gateway.
	// +optional
	Gateways []string `json:"gateways,omitempty"`
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

	// HostVRF is the VRF created by the daemon for a target with a HostVRF attachment.
	// +optional
	HostVRF *P4TargetHostVRFStatus `json:"hostVRF,omitempty"`

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
