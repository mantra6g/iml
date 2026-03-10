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

const TGT_DEP_FINALIZER_LABEL = "bmv2target.loom.io/finalizer"

const BMV2_POD_NAMESPACE = "loom-system"
const BMV2_TARGET_DEPLOYMENT_LABEL = "infra.loom.io/targetDeployment"
const BMV2_TARGET_REPLICA_INDEX_LABEL = "infra.loom.io/targetReplicaIndex"
const BMV2_DATAPLANE_CONTAINER_NAME = "data-plane"
const BMV2_DATAPLANE_CONTAINER_IMAGE = "tomasagata/p4target-dp-bmv2:latest"
const BMV2_CONTROLPLANE_CONTAINER_NAME = "control-plane"
const BMV2_CONTROLPLANE_CONTAINER_IMAGE = "tomasagata/p4target-control-plane:latest"
const BMV2_CONTROLPLANE_READY_PROBE_PORT = 5000
const BMV2_CONTROLPLANE_READY_PROBE_PATH = "/healthz"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// P4TargetDeploymentSpec defines the desired state of P4TargetDeployment
type P4TargetDeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html
}

type BMv2TargetConditionType string

const (
	BMv2TargetConditionReady BMv2TargetConditionType = "Ready"
)

// BMv2TargetCondition describes the state of a BMv2 target at a certain point.
type BMv2TargetCondition struct {
	Type               BMv2TargetConditionType `json:"type,omitempty"`
	Status             metav1.ConditionStatus  `json:"status,omitempty"`
	LastTransitionTime metav1.Time             `json:"lastUpdateTime,omitempty"`
	Reason             string                  `json:"reason,omitempty"`
	Message            string                  `json:"message,omitempty"`
}

// P4TargetDeploymentStatus defines the observed state of P4TargetDeployment.
type P4TargetDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Conditions []BMv2TargetCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"

// P4TargetDeployment is the Schema for the p4targetdeployments API
type P4TargetDeployment struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of P4TargetDeployment
	// +required
	Spec P4TargetDeploymentSpec `json:"spec"`

	// status defines the observed state of P4TargetDeployment
	// +optional
	Status P4TargetDeploymentStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// P4TargetDeploymentList contains a list of P4TargetDeployment
type P4TargetDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []P4TargetDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&P4TargetDeployment{}, &P4TargetDeploymentList{})
}
