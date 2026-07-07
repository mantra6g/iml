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

const (
	LabelNetworkServiceName string = "loom.io/network-service-name"
	LabelManagedBy          string = "loom.io/managed-by"
)

type UnavailabilityPolicyType string

const (
	PolicyDrop   UnavailabilityPolicyType = "Drop"
	PolicyBypass UnavailabilityPolicyType = "Bypass"
)

type LoadBalancingPolicy struct {
	Type LoadBalancingType `json:"type"`
}

type LoadBalancingType string

const (
	RoundRobinLoadBalancing LoadBalancingType = "RoundRobin"
	ECMPLoadBalancing       LoadBalancingType = "HashBased"
)

// NetworkServiceSpec defines the desired state of NetworkService
type NetworkServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// selector defines the network functions that will be part of this network service.
	// +required
	Selector map[string]string `json:"selector"`

	// lbPolicy states how to load balance the traffic across the matching network functions.
	// +optional
	LoadBalancingPolicy *LoadBalancingPolicy `json:"lbPolicy,omitempty"`

	// segmentID is the SRv6 identifier for this service.
	// Automatically assigned by the controller if not specified.
	// Immutable after being set.
	// +optional
	SegmentID string `json:"segmentID,omitempty"`

	// unavailabilityPolicy determines how the data-plane should behave when no matching network functions are found.
	// Possible values include:
	// - "Drop": drops incoming packets
	// - "Bypass": the network service is skipped
	//
	// Defaults to "Drop".
	// +optional
	// +kubebuilder:default:="Drop"
	UnavailabilityPolicy UnavailabilityPolicyType `json:"unavailabilityPolicy,omitempty"`
}

// NetworkServiceStatus defines the observed state of NetworkService.
type NetworkServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the NetworkService resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkService is the Schema for the networkservices API
type NetworkService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NetworkService
	// +required
	Spec NetworkServiceSpec `json:"spec"`

	// status defines the observed state of NetworkService
	// +optional
	Status NetworkServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkServiceList contains a list of NetworkService
type NetworkServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NetworkService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkService{}, &NetworkServiceList{})
}
