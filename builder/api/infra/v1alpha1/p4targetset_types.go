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

type TargetClass string
const (
	BMv2      TargetClass = "bmv2"
	// Tofino 	  TargetClass = "tofino"
	// XDP 		  TargetClass = "xdp"
	// DPDK 		  TargetClass = "dpdk"
)

// P4TargetSetSpec defines the desired state of P4TargetSet
type P4TargetSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Architecture is the target architecture of the P4 target set
	// +required
	// +kubebuilder:validation:Enum=bmv2
	TargetClass TargetClass `json:"targetClass,omitempty"`

	// Replicas is the number of desired replicas of the P4 target set
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`

	// CPUs is the number of CPUs allocated to each P4 target instance
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	CPUs *int32 `json:"cpus,omitempty"`

	// Memory is the amount of memory (in MiB) allocated to each P4 target instance
	// +optional
	// +kubebuilder:default=512
	// +kubebuilder:validation:Minimum=128
	Memory *int32 `json:"memory,omitempty"`
}

// P4TargetSetStatus defines the observed state of P4TargetSet.
type P4TargetSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase is the current phase of the P4 target set
	Phase string `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentReplicas is the number of created replicas of the P4 target set
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// ReadyReplicas is the number of ready replicas of the P4 target set
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// FailedReplicas is the number of failed replicas of the P4 target set
	FailedReplicas int32 `json:"failedReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// P4TargetSet is the Schema for the p4targetsets API
type P4TargetSet struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of P4TargetSet
	// +required
	Spec P4TargetSetSpec `json:"spec"`

	// status defines the observed state of P4TargetSet
	// +optional
	Status P4TargetSetStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// P4TargetSetList contains a list of P4TargetSet
type P4TargetSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []P4TargetSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&P4TargetSet{}, &P4TargetSetList{})
}
