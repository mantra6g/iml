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
	corev1alpha1 "loom/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReplicaSetConditionType string

// These are valid conditions of a replica set.
const (
	// ReplicaSetReplicaFailure is added in a replica set when one of its nfs fails to be created
	// due to insufficient quota, limit ranges, target selectors, etc. or deleted
	// due to the target driver being down or finalizers are failing.
	ReplicaSetReplicaFailure ReplicaSetConditionType = "ReplicaFailure"
)

// ReplicaSetCondition describes the state of a replica set at a certain point.
type ReplicaSetCondition struct {
	// Type of replica set condition.
	Type ReplicaSetConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=ReplicaSetConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

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

	// Selector is a label query over network function instances that should
	// match the replica count. It must match the labels of the NetworkFunctionTemplate.
	// +required
	Selector *metav1.LabelSelector `json:"selector"`

	// Template describes the NetworkFunction that will be created
	// +required
	Template corev1alpha1.NetworkFunctionTemplate `json:"template,omitempty"`

	// MinReadySeconds is the minimum number of seconds for which a newly created NetworkFunction
	// should be ready without any of its container crashing, for it to be considered available. Defaults
	// to 0 (the NetworkFunction will be considered available as soon as it is ready).
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`
}

// NetworkFunctionReplicaSetStatus defines the observed state of NetworkFunctionReplicaSet.
type NetworkFunctionReplicaSetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Replicas is the total number of non-terminated replicas that are currently running and ready.
	Replicas int32 `json:"replicas,omitempty"`

	// FullyLabeledReplicas is the number of replicas that are fully labeled and ready.
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas,omitempty"`

	// ReadyReplicas is the number of ready NetworkFunction replicas
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas is the number of available NetworkFunction replicas
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// ObservedGeneration is the most recent generation observed for this NetworkFunctionReplicaSet.
	// It corresponds to the generation of the most recently observed NetworkFunctionReplicaSet's desired state.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunctionReplicaSet's current state.
	Conditions []ReplicaSetCondition `json:"conditions,omitempty"`
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
