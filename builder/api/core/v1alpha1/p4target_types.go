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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const TARGET_LABEL = "core.desire6g.eu/target"
const P4TARGET_FINALIZER_LABEL = "core.desire6g.eu/p4Target-finalizer"

const (
	TARGET_BMV2   = "bmv2"
	TARGET_TOFINO = "tofino"
	// TARGET_XDP  = "xdp"
	// TARGET_DPDK = "dpdk"
)

const (
	P4_TARGET_PHASE_PENDING  = "Pending"
	P4_TARGET_PHASE_READY    = "Ready"
	P4_TARGET_PHASE_OCCUPIED = "Occupied"
	P4_TARGET_PHASE_UNKNOWN  = "Unknown"
)

// P4TargetSpec defines the desired state of P4Target
type P4TargetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// TargetClass is the target class of the P4 target
	// +required
	// +kubebuilder:validation:Enum=bmv2
	TargetClass string `json:"targetClass,omitempty"`
}

// P4TargetStatus defines the observed state of P4Target.
type P4TargetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase is the current phase of the P4 target
	Phase string `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates if the underlying P4 target is ready to accept network functions
	Ready bool `json:"ready,omitempty"`

	// LastHeartbeatTime is the last time the P4 target sent a heartbeat
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"

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
