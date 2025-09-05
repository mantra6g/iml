package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectReference struct {
	// Namespace of the resource.
	// Optional. Defaults to "default" if not set.
	// +kubebuilder:default:=default
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,1,opt,name=namespace"`

	// Name of the resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +required
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`
}

func (nr *ObjectReference) ToObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: nr.Namespace,
		Name:      nr.Name,
	}
}
