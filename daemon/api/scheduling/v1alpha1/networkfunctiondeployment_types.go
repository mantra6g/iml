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
	corev1alpha1 "iml-daemon/api/core/v1alpha1"

	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	DefaultDeploymentStrategyType      = DeploymentStrategyTypeRollingUpdate
	DefaultRollingUpdateMaxUnavailable = "25%"
	DefaultRollingUpdateMaxSurge       = "25%"
)
const NFDeploymentFinalizer = "networkfunctiondeployment.loom.io/finalizer"
const NFSpecHashLabel = "networkfunction.loom.io/specHash"

type Foo = v1.DeploymentStrategy

type DeploymentStrategyType string

const (
	DeploymentStrategyTypeRollingUpdate DeploymentStrategyType = "RollingUpdate"
	DeploymentStrategyTypeRecreate      DeploymentStrategyType = "Recreate"
)

type RollingUpdateDeployment struct {
	// MaxUnavailable is the maximum number of pods that can be unavailable during the update process.
	// It can be an absolute number (e.g., 1) or a percentage (e.g., "25%").
	// Defaults to 25% if not specified.
	// +optional
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:validation:Pattern="^((100|[0-9]{1,2})%|[0-9]+)$"
	// +kubebuilder:default="25%"
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// MaxSurge is the maximum number of pods that can be created above the desired
	// number of replicas during the update process.
	// It can be an absolute number (e.g., 1) or a percentage (e.g., "25%").
	// Defaults to 25% if not specified.
	// +optional
	// +kubebuilder:validation:XIntOrString
	// +kubebuilder:validation:Pattern="^((100|[0-9]{1,2})%|[0-9]+)$"
	// +kubebuilder:default="25%"
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

type DeploymentStrategy struct {
	// Type of deployment. Can be "RollingUpdate" or "Recreate".
	// +kubebuilder:validation:Enum=RollingUpdate;Recreate
	// +kubebuilder:default="RollingUpdate"
	Type DeploymentStrategyType `json:"type"`

	// RollingUpdate strategy parameters. Should only be present if Type is RollingUpdate.
	// +optional
	RollingUpdate *RollingUpdateDeployment `json:"rollingUpdate,omitempty"`
}

// NetworkFunctionDeploymentSpec defines the desired state of NetworkFunctionDeployment
type NetworkFunctionDeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Replicas is the number of desired replicas of the network function
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Strategy defines the deployment strategy for the network function
	// +optional
	Strategy *DeploymentStrategy `json:"strategy,omitempty"`

	// Selector is a label query over network function instances that should
	// match the replica count. It must match the labels of the NetworkFunctionTemplate.
	// +required
	Selector *metav1.LabelSelector `json:"selector"`

	// Template describes the NetworkFunctionDeployment that will be created
	// +required
	Template corev1alpha1.NetworkFunctionTemplate `json:"template,omitempty"`

	// MinReadySeconds is the minimum number of seconds for which a newly created NetworkFunction
	// should be ready without any of its container crashing, for it to be considered available. Defaults
	// to 0 (the NetworkFunction will be considered available as soon as it is ready).
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`
}

type NFDeploymentConditionType string

const (
	// NFDeploymentAvailable condition type is intended to indicate whether the network function is currently
	// available and ready to serve traffic.
	NFDeploymentAvailable NFDeploymentConditionType = "Available"
	// NFProgressing condition type is intended to indicate whether the
	// network function is in the process of being deployed or updated. It can be used to
	// provide more granular information about the deployment status of the network function,
	// such as whether it is currently being rolled out, if there are any issues during the rollout,
	// or if it is waiting for certain conditions to be met before proceeding with the deployment.
	// Currently UNIMPLEMENTED
	NFDeploymentProgressing NFDeploymentConditionType = "Progressing"
)

type NFDeploymentCondition struct {
	Type               NFDeploymentConditionType `json:"type"`
	Status             metav1.ConditionStatus    `json:"status"`
	LastUpdateTime     metav1.Time               `json:"lastUpdateTime"`
	LastTransitionTime metav1.Time               `json:"lastTransitionTime"`
	Reason             string                    `json:"reason,omitempty"`
	Message            string                    `json:"message,omitempty"`
}

// NetworkFunctionDeploymentStatus defines the observed state of NetworkFunctionDeployment.
type NetworkFunctionDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the most recent generation observed for this NetworkFunctionDeployment. It corresponds to the
	// generation of the NetworkFunctionDeploymentSpec that was last processed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the total number of replicas observed by the controller.
	Replicas int32 `json:"replicas,omitempty"`

	// UpdatedReplicas is the total number of replicas that have been updated to match the desired state.
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// ReadyReplicas is the number of ready replicas of the network function
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas is the number of replicas that are ready and stable for at least minReadySeconds.
	// A replica is considered available when its ready condition is true, and it has been ready for
	// at least minReadySeconds. Defaults to 0 (the replica will be considered available as soon as it is ready)
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// UnavailableReplicas is the number of unavailable replicas of the network function
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// CollisionCount is the count of hash collisions for the NetworkFunctionDeployment. The number is incremented by the
	// controller when it detects a hash collision between NetworkFunctionReplicaSets with different spec templates.
	// The controller uses this field to generate a unique name for the NetworkFunction
	CollisionCount *int32 `json:"collisionCount,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunctionDeployment's current state.
	Conditions []NFDeploymentCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NetworkFunctionDeployment is the Schema for the networkfunctions API
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
