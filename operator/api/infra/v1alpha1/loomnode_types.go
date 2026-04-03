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

// LoomNodeSpec defines the desired state of LoomNode
type LoomNodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// PodCIDRs specifies the CIDR blocks used for pod IPs on this node.
	// This is used by the CNI plugin to determine which IPs to assign to pods scheduled on this node.
	// When left empty, the controller will automatically allocate a CIDR block for this node
	// from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.
	// +optional
	PodCIDRs []string `json:"podCIDRs,omitempty"`

	// SidCIDRs specifies the CIDR blocks used for segment identifiers (SID) on this node.
	// This is used by the CNI plugin to determine which SIDs to assign to pods scheduled on this node.
	// When left empty, the controller will automatically allocate a CIDR block for this node
	// from the cluster's CIDR range set with the --cluster-cidr argument when starting the controller.
	// +optional
	SidCIDRs []string `json:"sidCIDRs,omitempty"`

	// P4TargetCIDRs are similar to PodCIDRs but used for P4Targets in this node.
	// +optional
	P4TargetCIDRs []string `json:"p4TargetCIDRs,omitempty"`

	// TunnelCIDRs are similar to PodCIDRs but used for tunnels in this node.
	// +optional
	TunnelCIDRs []string `json:"tunnelCIDRs,omitempty"`
}

// LoomNodeStatus defines the observed state of LoomNode.
type LoomNodeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// TransportIPs specifies the IPs that the networking plugin should use to create the tunnels between nodes.
	TransportIPs []string `json:"transportIPs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// LoomNode is the Schema for the loomnodes API
type LoomNode struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of LoomNode
	// +required
	Spec LoomNodeSpec `json:"spec"`

	// status defines the observed state of LoomNode
	// +optional
	Status LoomNodeStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// LoomNodeList contains a list of LoomNode
type LoomNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoomNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoomNode{}, &LoomNodeList{})
}
