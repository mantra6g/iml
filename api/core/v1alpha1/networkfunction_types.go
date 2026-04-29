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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const NetworkFunctionFinalizer = "networkfunction.loom.io/finalizer"

const TARGET_ASSIGNMENT_LABEL = "scheduling.loom.io/assignedTarget"
const CONTROL_PLANE_POD_LABEL = "scheduling.loom.io/controlPlane"

const CONTROL_PLANE_POD_NF_NAME_ENV_VAR_KEY = "NF_NAME"
const CONTROL_PLANE_POD_NF_NAMESPACE_ENV_VAR_KEY = "NF_NAMESPACE"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ControlPlaneSpec struct {
	// Image is the container image for the control plane pod of the network function.
	// +required
	Image string `json:"image,omitempty"`

	// ImagePullPolicy defines the image pull policy for the control plane pod.
	// +optional
	// +kubebuilder:default=IfNotPresent
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Resources defines the resource requests and limits for the control plane pod.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// NodeName specifies the name of the node where the control plane pod should be scheduled.
	// If specified, the scheduler will attempt to schedule the pod on the specified node.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// NodeSelector defines the node selector for the control plane pod.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations defines the tolerations for the control plane pod.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// Affinity defines the affinity rules for the control plane pod.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// ExtraEnv defines extra environment variables for the control plane pod.
	// +optional
	ExtraEnv []v1.EnvVar `json:"extraEnv,omitempty"`

	// Args defines the command-line arguments for the control plane pod.
	// +optional
	Args []string `json:"args,omitempty"`
}

type NetworkFunctionTemplate struct {
	// Standard object's metadata.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the pod.
	// +optional
	Spec NetworkFunctionSpec `json:"spec,omitempty"`
}

type NetworkFunctionConfigReference struct {
	Name string `json:"name"`
}

// NetworkFunctionSpec defines the desired state of NetworkFunction
type NetworkFunctionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// TargetName is an optional field that can be used to specify the name of the P4Target
	// where this NetworkFunction instance should be scheduled.
	// If not specified, the scheduler will automatically select a suitable P4Target based on the TargetSelector.
	// +optional
	TargetName string `json:"targetName,omitempty"`

	// ControlPlane defines the template for the control plane pod
	// of the network function.
	// +optional
	ControlPlane *ControlPlaneSpec `json:"controlPlane,omitempty"`

	// TargetSelector is used to select P4 targets based on their supported architectures
	// +optional
	TargetSelector map[string]string `json:"targetSelector,omitempty"`

	// ConfigRef allows specifying a NetworkFunctionConfig resource to modify table entries
	// +optional
	ConfigRef *NetworkFunctionConfigReference `json:"configRef,omitempty"`

	// P4File is the actual P4 program file for the network function.
	// It can be the actual p4program encoded in base64 or
	// a s3://, http:// or https:// URL pointing to the P4 file location.
	// +required
	P4File string `json:"p4File"`
}

type NetworkFunctionPhase string

const (
	NetworkFunctionPending NetworkFunctionPhase = "Pending"
	NetworkFunctionRunning NetworkFunctionPhase = "Running"
	NetworkFunctionFailed  NetworkFunctionPhase = "Failed"
)

type NetworkFunctionReason string

var f = v1.PodReasonInfeasible

const (
	NetworkFunctionReasonEvicted       NetworkFunctionReason = "Evicted"
	NetworkFunctionReasonUnschedulable NetworkFunctionReason = "Unschedulable"
)

// NetworkFunctionConditionType is a valid value for NetworkFunctionCondition.Type
type NetworkFunctionConditionType string

// These are built-in conditions of a nf. An application may use a custom condition not listed here.
const (
	// NetworkFunctionInitialized means that the p4 programs have been installed and are running.
	NetworkFunctionInitialized NetworkFunctionConditionType = "Initialized"
	// NetworkFunctionReady means the nf is able to service requests and should be added to the
	// load balancing pools of all matching services.
	NetworkFunctionReady NetworkFunctionConditionType = "Ready"
	// NetworkFunctionScheduled represents status of the scheduling process for this nf.
	NetworkFunctionScheduled NetworkFunctionConditionType = "Scheduled"
	// DisruptionTarget indicates the nf is about to be terminated due to a
	// disruption (such as preemption, eviction API or garbage-collection).
	DisruptionTarget NetworkFunctionConditionType = "DisruptionTarget"
	// NetworkFunctionReadyToStart programmable target is successfully configured and
	// the nf is ready to run.
	NetworkFunctionReadyToStart NetworkFunctionConditionType = "ReadyToStart"
)

type NetworkFunctionCondition struct {
	Type               NetworkFunctionConditionType `json:"type"`
	Status             metav1.ConditionStatus       `json:"status"`
	LastProbeTime      metav1.Time                  `json:"lastProbeTime,omitempty"`
	LastTransitionTime metav1.Time                  `json:"lastTransitionTime,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
	Message            string                       `json:"message,omitempty"`
}

// NetworkFunctionStatus defines the observed state of NetworkFunction.
type NetworkFunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// AssignedIP is the IP address assigned to the NetworkFunction, used for routing traffic to it.
	AssignedIP string `json:"assignedIP,omitempty"`

	// ObservedGeneration is the most recent generation observed for this NetworkFunction
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase indicates the current phase of the NetworkFunction
	Phase NetworkFunctionPhase `json:"phase,omitempty"`

	// Reason provides additional information about why the NetworkFunction is in its current phase.
	Reason NetworkFunctionReason `json:"reason,omitempty"`

	// Conditions represent the latest available observations of the
	// NetworkFunction's current state.
	Conditions []NetworkFunctionCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=nf;nfs

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
