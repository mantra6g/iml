package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NetworkFunctionSpec defines the desired state of NetworkFunction
type NetworkFunctionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of NetworkFunction. Edit networkfunction_types.go to remove/update
	// +optional
	// Foo *string `json:"foo,omitempty"`

	// Image is the container image for the network function
	// +required
	Image string `json:"image"`

	// Replicas is the number of desired replicas of the network function
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`
}

// NetworkFunctionStatus defines the observed state of NetworkFunction.
type NetworkFunctionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// AvailableReplicas is the number of available replicas of the network function
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
