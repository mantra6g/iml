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

type EndpointDetails struct {
	// ManagementAddress is the management IP address of the P4 target
	// +required
	ManagementAddress string `json:"mgmtAddress,omitempty"`

	// P4RuntimePort is the P4Runtime gRPC port of the P4 target
	// +required
	P4RuntimePort int32 `json:"p4runtimePort,omitempty"`

	// GNMIPort is the gNMI gRPC port of the P4 target
	// +required
  GNMIport int32 `json:"gnmiPort,omitempty"`
}

type ResourceRequirements struct {
	// CPU is the number of CPUs allocated to the P4 target
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	CPU *int32 `json:"cpu,omitempty"`

	// Memory is the amount of memory (in MiB) allocated to the P4 target
	// +optional
	// +kubebuilder:default=512
	// +kubebuilder:validation:Minimum=128
	Memory *int32 `json:"memory,omitempty"`
}

type ResourceList struct {
	CPUs int32 `json:"cpus,omitempty"`
	Memory int32 `json:"memory,omitempty"`
}

// P4TargetSpec defines the desired state of P4Target
type P4TargetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// TargetClass is the target class of the P4 target
	// +required
	// +kubebuilder:validation:Enum=bmv2
	TargetClass TargetClass `json:"targetClass,omitempty"`

	// Endpoint defines the connection details of the P4 target
	// +required
	Endpoint EndpointDetails `json:"endpoint,omitempty"`

	// Resources defines the compute resources allocated to the P4 target
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty"`
}

// P4TargetStatus defines the observed state of P4Target.
type P4TargetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase is the current phase of the P4 target
	Phase string `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Capacity is the total amount of resources on the P4 target
	Capacity ResourceList `json:"capacity,omitempty"`

	// Allocatable is the amount of resources allocatable on the P4 target
	Allocatable ResourceList `json:"allocatable,omitempty"`

	// Used is the amount of resources currently used on the P4 target
	Used ResourceList `json:"used,omitempty"`

	// LastHeartbeatTime is the last time the P4 target sent a heartbeat
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty"`
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
