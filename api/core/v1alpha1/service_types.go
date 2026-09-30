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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ServiceFinalizer guards the cleanup of the Kubernetes Service generated for a loom Service.
	ServiceFinalizer = "service.loom.io/finalizer"
	// ServiceTargetNamespace is the namespace where the generated Kubernetes Services
	// and their EndpointSlices live.
	ServiceTargetNamespace = "loom-system"
	// ServiceNameLabel and ServiceNamespaceLabel point generated objects back to their loom Service.
	// Owner references can't be used since they would cross namespaces.
	ServiceNameLabel      = "service.loom.io/name"
	ServiceNamespaceLabel = "service.loom.io/namespace"
	// LoomCNINetworkName is the multus network whose IPs are published as Service endpoints.
	LoomCNINetworkName = "loom-cni"
	// EndpointSliceManagedBy is the endpointslice.kubernetes.io/managed-by value for loom EndpointSlices.
	EndpointSliceManagedBy = "endpointslice.loom.io"

	// ServiceConditionReady reports whether the Kubernetes Service backing a loom Service is in place.
	ServiceConditionReady = "Ready"
)

// ServiceType describes how a Service is exposed.
// +kubebuilder:validation:Enum=ClusterIP
type ServiceType string

const (
	// ServiceTypeClusterIP exposes the Service through a cluster-internal virtual IP.
	ServiceTypeClusterIP ServiceType = "ClusterIP"
)

// ServicePort describes a port exposed by a Service.
type ServicePort struct {
	// Name of this port. Must be a DNS_LABEL and unique within the Service
	// when more than one port is defined.
	// +optional
	Name string `json:"name,omitempty"`

	// Protocol of this port.
	// +kubebuilder:default=TCP
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`

	// Port exposed by the Service. Traffic is forwarded to the same port on the endpoints.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +required
	Port int32 `json:"port"`
}

// ServiceSpec defines the desired state of the Service
type ServiceSpec struct {
	// Type determines how the Service is exposed. Only ClusterIP is supported.
	// +kubebuilder:default=ClusterIP
	// +optional
	Type ServiceType `json:"type,omitempty"`

	// Selector matches the pods, in the same namespace as the Service,
	// whose loom-cni addresses back this Service. A pod matches when its labels
	// contain every key/value pair in the selector.
	// +kubebuilder:validation:MinProperties=1
	// +required
	Selector map[string]string `json:"selector"`

	// Ports exposed by the Service.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=port
	// +listMapKey=protocol
	// +required
	Ports []ServicePort `json:"ports"`
}

// ServiceStatus defines the observed state of Service.
type ServiceStatus struct {
	// ServiceName is the name of the Kubernetes Service generated in the loom-system namespace.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// Hostname is the fully qualified DNS name the Service is reachable at.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// ClusterIPs are the virtual IPs allocated to the Service.
	// +listType=atomic
	// +optional
	ClusterIPs []string `json:"clusterIPs,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the Service's state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Cluster-IP",type=string,JSONPath=`.status.clusterIPs[0]`
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.status.hostname`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z]([-a-z0-9]*[a-z0-9])?$')",message="name must be a DNS-1035 label"

// Service is the Schema for the services API
type Service struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Service
	// +required
	Spec ServiceSpec `json:"spec"`

	// status defines the observed state of Service
	// +optional
	Status ServiceStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ServiceList contains a list of Service
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
