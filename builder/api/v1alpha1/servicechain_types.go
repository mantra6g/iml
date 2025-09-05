package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceChainSpec defines the desired state of ServiceChain
type ServiceChainSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Specifies the source application
	// +required
	From *ObjectReference `json:"from"`

	// Specifies the destination application
	// +required
	To *ObjectReference `json:"to"`

	// Specifies the intermediate functions between the source and destination applications
	// +optional
	Functions []ObjectReference `json:"functions,omitempty"`
}

// ServiceChainStatus defines the observed state of ServiceChain.
type ServiceChainStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// UID of the source application.
	SourceAppUID types.UID `json:"src_app_uid"`

	// UID of the destinatination application.
	DestinationAppUID types.UID `json:"dst_app_uid"`

	// UID of the intermediate functions.
	Functions []types.UID `json:"nfs_uids"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceChain is the Schema for the servicechains API
type ServiceChain struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ServiceChain
	// +required
	Spec ServiceChainSpec `json:"spec"`

	// status defines the observed state of ServiceChain
	// +optional
	Status ServiceChainStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ServiceChainList contains a list of ServiceChain
type ServiceChainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceChain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceChain{}, &ServiceChainList{})
}
