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

// NetworkFunctionDeploymentSpec defines the desired state of NetworkFunctionDeployment
type NetworkFunctionDeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Replicas is the number of desired replicas of the NetworkFunction
	// +optional
	// +kubebuilder:default:=1
	Replicas *int32 `json:"replicas,omitempty"`

}

// NetworkFunctionDeploymentStatus defines the observed state of NetworkFunctionDeployment.
type NetworkFunctionDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates the current phase of the NetworkFunctionDeployment
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunctionDeployment's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastDeploymentTime is the last time the NetworkFunctionDeployment was deployed
	LastDeploymentTime metav1.Time `json:"lastDeploymentTime,omitempty"`

	// CurrentReplicas is the current number of replicas of the NetworkFunction
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// ReadyReplicas is the number of ready replicas of the NetworkFunction
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkFunctionDeployment is the Schema for the networkfunctiondeployments API
type NetworkFunctionDeployment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NetworkFunctionDeployment
	// +required
	Spec NetworkFunctionDeploymentSpec `json:"spec"`

	// status defines the observed state of NetworkFunctionDeployment
	// +optional
	Status NetworkFunctionDeploymentStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkFunctionDeploymentList contains a list of NetworkFunctionDeployment
type NetworkFunctionDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkFunctionDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkFunctionDeployment{}, &NetworkFunctionDeploymentList{})
}
