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
	// Entries to be added to the table
	// +optional
	Entries []TableEntry `json:"entries"`

	// DefaultAction is the action to execute whenever there is no match
	// +optional
	DefaultAction ActionConfig `json:"defaultAction"`
}

type TableEntry struct {
	// MatchFields is a list of fields to match against for this entry.
	// +required
	MatchFields []MatchField `json:"matchFields"`
	// Action is the action to take if the packet matches the MatchFields.
	// +required
	Action ActionConfig `json:"action"`
}

// MatchField determines which fields and values to match.
// +kubebuilder:validation:AtMostOneOf:=exact;ternary;lpm;range;optional
type MatchField struct {
	// Name of the field to match
	// +required
	Name string `json:"name"`

	// Type of the value
	// +required
	Type MatchFieldType `json:"type"`

	// Exact match on the field value.
	// +optional
	Exact *ParametrizedValue `json:"exact"`

	// Ternary specifies a mask to
	// +optional
	Ternary *TernaryValue `json:"ternary"`

	// LPM specifies a prefix length match on the field value.
	// +optional
	LPM *LPMValue `json:"lpm"`

	// Range
	// +optional
	Range *RangeValue `json:"range"`

	// Optional match on the field value. If the value is present, it will be treated as an exact match,
	// if the value is not present, it will be treated as a wildcard.
	// +optional
	Optional *ParametrizedValue `json:"optional"`
}

type MatchFieldType string

const (
	ExactMatch    MatchFieldType = "Exact"
	TernaryMatch  MatchFieldType = "Ternary"
	LPMMatch      MatchFieldType = "LPM"
	RangeMatch    MatchFieldType = "Range"
	OptionalMatch MatchFieldType = "Optional"
)

type TernaryValue struct {
	Value ParametrizedValue `json:"value"`
	Mask  string            `json:"mask"`
}

type LPMValue struct {
	Value ParametrizedValue `json:"value"`
	// PrefixLen in bits
	PrefixLen string `json:"prefixLen"`
}

type RangeValue struct {
	Low  ParametrizedValue `json:"low"`
	High ParametrizedValue `json:"high"`
}

// ParametrizedValue allows defining a value and the type at the same time.
// +kubebuilder:validation:ExactlyOneOf:=rawHex;int;ipv4Address;ipv6Address;macAddress
type ParametrizedValue struct {
	// RawHex allows specifying hex values directly
	// +optional
	RawHex *string `json:"rawHex"`
	// Int specifies integer values of arbitrary size as a string
	// +optional
	Int *string `json:"int"`
	// IPv4Address specifies an IPv4 address
	// +optional
	IPv4Address *string `json:"ipv4Address"`
	// IPv6Address specifies an IPv6 address
	// +optional
	IPv6Address *string `json:"ipv6Address"`
	// MACAddress specifies a MAC address
	// +optional
	MACAddress *string `json:"macAddress"`
}

type ActionConfig struct {
	// Name of the action to execute
	// +required
	Name string `json:"name"`
	// Parameters is a list of parameters for the action.
	// +optional
	Parameters []NamedParameter `json:"parameters"`
}

type NamedParameter struct {
	Value ParametrizedValue `json:",inline"`
	// Name of the parameter. This is used to identify the parameter in the action definition.
	// +required
	Name string `json:"name"`
}

// NetworkFunctionConfigSpec defines the desired state of NetworkFunctionConfig
type NetworkFunctionConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Tables defines the configurations for each of the tables in the network function
	// +optional
	Tables map[string]TableConfig `json:"tables"`
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
