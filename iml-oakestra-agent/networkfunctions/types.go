package networkfunctions

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var Resource = schema.GroupVersionResource{
	Group:    "core.desire6g.eu",
	Version:  "v1alpha1",
	Resource: "networkfunctions",
}

type SubFunctionSpec struct {
	// +optional
	Name string `json:"name,omitempty"`

	// ID is the unique identifier of the sub-function
	// +required
	ID uint32 `json:"id"`
}

type NetworkFunctionType string
const (
	NetworkFunctionTypeSimple NetworkFunctionType = "simple"
	NetworkFunctionTypeMultiplexed NetworkFunctionType = "multiplexed"
)

type NetworkFunctionSpec struct {
	// Replicas is the number of desired replicas of the network function
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *uint32 `json:"replicas,omitempty"`

	// Type is the type of the network function ("simple" or "multiplexed")
	// +optional
	// +kubebuilder:default=simple
	// +kubebuilder:validation:Enum=simple;multiplexed
	Type NetworkFunctionType `json:"type,omitempty"`

	// SubFunctions is a list of sub-functions provided by this network function
	// +optional
	// +kubebuilder:validation:MaxItems=16
	SubFunctions []SubFunctionSpec `json:"subFunctions,omitempty"`

	// Containers is the list of containers that make up the network function
	// +required
	// +kubebuilder:validation:MinItems=1
	Containers []corev1.Container `json:"containers,omitempty"`
}

type NetworkFunction struct {
	metav1.TypeMeta `json:",inline"`

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