package networkfunctions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var Resource = schema.GroupVersionResource{
	Group:    "cache.desire6g.eu",
	Version:  "v1alpha1",
	Resource: "networkfunction",
}

type NetworkFunctionSpec struct {
	// Image specifies the container image for the network function.
	Image string `json:"image"`
}

type NetworkFunction struct {
	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NetworkFunction
	// +required
	Spec NetworkFunctionSpec `json:"spec"`
}
func (nf *NetworkFunction) ToUnstructured() *unstructured.Unstructured {
	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(nf)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: objMap}
}