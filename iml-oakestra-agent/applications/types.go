package applications

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var Resource = schema.GroupVersionResource{
	Group:    "cache.desire6g.eu",
	Version:  "v1alpha1",
	Resource: "application",
}

type ApplicationSpec struct {
	// OverrideID allows specifying a custom identifier for the application.
	// Necessary when inheriting IDs from another system.
	OverrideID string `json:"override_id"`
}

type Application struct {
	// metadata is a standard object metadata
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Application
	Spec ApplicationSpec `json:"spec"`
}
func (a *Application) ToUnstructured() *unstructured.Unstructured {
	objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(a)
	if err != nil {
		return nil
	}
	return &unstructured.Unstructured{Object: objMap}
}