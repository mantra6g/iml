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
	"k8s.io/apimachinery/pkg/types"
)

const SERVICE_CHAIN_FINALIZER_LABEL = "servicechain.loom.io/finalizer"

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TODO: support label selectors for applications instead of creating an Application resource

type ApplicationReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (ref *ApplicationReference) ToNamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}
}

// ServiceChainSpec defines the desired state of ServiceChain
type ServiceChainSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// From specifies the source application.
	// +required
	From *ApplicationReference `json:"from"`

	// To specifies the destination application.
	// +required
	To *ApplicationReference `json:"to"`

	// Specifies the intermediate functions between the source and destination applications
	// +optional
	Functions []metav1.LabelSelector `json:"functions,omitempty"`
}

// ServiceChainStatus defines the observed state of ServiceChain.
type ServiceChainStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// UID of the source application.
	SourceAppUID types.UID `json:"src_app_uid"`

	// UID of the destinatination application.
	DestinationAppUID types.UID `json:"dst_app_uid"`
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
