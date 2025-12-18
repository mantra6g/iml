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

type TargetArchitecture string
const (
	BMv2      TargetArchitecture = "bmv2"
	// Tofino 	  TargetArchitecture = "tofino"
	// XDP 		  TargetArchitecture = "xdp"
	// DPDK 		  TargetArchitecture = "dpdk"
)

// NetworkFunctionSpec defines the desired state of NetworkFunction
type NetworkFunctionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of NetworkFunction. Edit networkfunction_types.go to remove/update
	// +optional
	// Foo *string `json:"foo,omitempty"`

	// Replicas is the number of desired replicas of the network function
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Supported target architectures for the network function
	// +required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:Items=enum=bmv2
	SupportedTargets []TargetArchitecture `json:"supportedTargets,omitempty"`

	// P4File is the actual P4 program file for the network function.
	// It can be the actual p4program encoded in base64 or 
	// a s3://, http:// or https:// URL pointing to the P4 file location.
	// +required
	P4File string `json:"p4File,omitempty"`
}

// NetworkFunctionStatus defines the observed state of NetworkFunction.
type NetworkFunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates the current phase of the NetworkFunction
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunction's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// CurrentReplicas is the current number of replicas of the network function
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// ReadyReplicas is the number of ready replicas of the network function
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkFunction is the Schema for the networkfunctions API
type NetworkFunction struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NetworkFunction
	// +required
	Spec NetworkFunctionSpec `json:"spec"`

	// status defines the observed state of NetworkFunction
	// +optional
	Status NetworkFunctionStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkFunctionList contains a list of NetworkFunction
type NetworkFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkFunction{}, &NetworkFunctionList{})
}
