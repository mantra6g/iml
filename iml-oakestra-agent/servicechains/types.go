package servicechains

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var Resource = schema.GroupVersionResource{
	Group:    "cache.desire6g.eu",
	Version:  "v1alpha1",
	Resource: "servicechain",
}

type ObjectReference struct {
	// Namespace of the resource
	Namespace string `json:"namespace,omitempty"`

	// Name of the resource
	Name string `json:"name"`
}

type ServiceChainSpec struct {
	// Specifies the source application
	From *ObjectReference `json:"from"`

	// Specifies the destination application
	To *ObjectReference `json:"to"`

	// Specifies the intermediate functions between the source and destination applications
	Functions []ObjectReference `json:"functions,omitempty"`
}

type ServiceChain struct {
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
