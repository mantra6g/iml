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

const REPLICA_SET_FINALIZER_LABEL = "scheduling.desire6g.eu/networkFunctionReplicaSet-finalizer"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetworkFunctionReplicaSetSpec defines the desired state of NetworkFunctionReplicaSet
type NetworkFunctionReplicaSetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Replicas is the number of desired replicas of the NetworkFunction
	// +optional
	// +kubebuilder:default:=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Template describes the NetworkFunction that will be created
	// +required
	Template NetworkFunctionBindingTemplate `json:"template,omitempty"`
}

// NetworkFunctionReplicaSetStatus defines the observed state of NetworkFunctionReplicaSet.
type NetworkFunctionReplicaSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase indicates the current phase of the NetworkFunctionReplicaSet
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunctionReplicaSet's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastDeploymentTime is the last time the NetworkFunctionReplicaSet was deployed
	LastDeploymentTime metav1.Time `json:"lastDeploymentTime,omitempty"`

	// CurrentReplicas is the current number of replicas of the NetworkFunctionBinding
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// ReadyReplicas is the number of ready replicas of the NetworkFunctionBinding
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkFunctionReplicaSet is the Schema for the networkfunctionreplicasets API
type NetworkFunctionReplicaSet struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NetworkFunctionReplicaSet
	// +required
	Spec NetworkFunctionReplicaSetSpec `json:"spec"`

	// status defines the observed state of NetworkFunctionReplicaSet
	// +optional
	Status NetworkFunctionReplicaSetStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// NetworkFunctionReplicaSetList contains a list of NetworkFunctionReplicaSet
type NetworkFunctionReplicaSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkFunctionReplicaSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkFunctionReplicaSet{}, &NetworkFunctionReplicaSetList{})
}
