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

type TableConfig struct {
	// Name of the table
	// +required
	Name string `json:"name"`
	// Entries to be added to the table
	// +optional
	Entries []TableEntry `json:"entries"`
}

type TableEntry struct {
	// MatchFields is a list of fields to match against for this entry.
	// +required
	MatchFields []TypedValue `json:"matchFields"`
	// Action is the action to take if the packet matches the MatchFields.
	// +required
	Action ActionConfig `json:"action"`
}

type TypedValue struct {
	// Name of the field to match
	// +required
	Name string `json:"name"`
	// Value to of the field to match against
	// +required
	Value string `json:"value"`
	// Type of the value
	// +required
	Type string `json:"type"`
}

type ActionConfig struct {
	// Name of the action to execute
	// +required
	Name string `json:"name"`
	// Parameters is a list of parameters for the action.
	// +optional
	Parameters []TypedValue `json:"parameters"`
}

// NetworkFunctionConfigSpec defines the desired state of NetworkFunctionConfig
type NetworkFunctionConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Tables defines the configurations for each of the tables in the network function
	// +optional
	Tables []TableConfig `json:"tables"`
}

// NetworkFunctionConfigStatus defines the observed state of NetworkFunctionConfig.
type NetworkFunctionConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkFunctionConfig is the Schema for the networkfunctionconfigs API
type NetworkFunctionConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NetworkFunctionConfig
	// +required
	Spec NetworkFunctionConfigSpec `json:"spec"`

	// status defines the observed state of NetworkFunctionConfig
	// +optional
	Status NetworkFunctionConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkFunctionConfigList contains a list of NetworkFunctionConfig
type NetworkFunctionConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NetworkFunctionConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkFunctionConfig{}, &NetworkFunctionConfigList{})
}
