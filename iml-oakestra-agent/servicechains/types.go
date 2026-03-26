package servicechains

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var Resource = schema.GroupVersionResource{
	Group:    "core.loom.io",
	Version:  "v1alpha1",
	Resource: "servicechains",
}

type NetworkFunctionReference struct {
	// Name of the NetworkFunction
	// +required
	Name string `json:"name"`

	// Namespace of the NetworkFunction
	// +required
	Namespace string `json:"namespace"`

	// SubFunctionID is the ID of the sub-function within the NetworkFunction
	// +optional
	SubFunctionID *uint32 `json:"subFunctionID,omitempty"`
}

type ApplicationReference struct {
	// Namespace of the resource
	Namespace string `json:"namespace,omitempty"`

	// Name of the resource
	Name string `json:"name"`
}

type ServiceChainSpec struct {
	// Specifies the source application
	From *ApplicationReference `json:"from"`

	// Specifies the destination application
	To *ApplicationReference `json:"to"`

	// Specifies the intermediate functions between the source and destination applications
	Functions []NetworkFunctionReference `json:"functions,omitempty"`
}

type ServiceChain struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ServiceChain
	Spec ServiceChainSpec `json:"spec"`
}

func (sc *ServiceChain) ToUnstructured() *unstructured.Unstructured {
	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sc)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: objMap}
}
