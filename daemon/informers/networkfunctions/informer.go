package networkfunctions

import (
	"context"
	"fmt"
	corev1alpha1 "iml-daemon/api/core/v1alpha1"

	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetUpInformer sets up the informer for NetworkFunction resources.
// This function should be called during the initialization of the controller
// manager to ensure that the informer is registered and starts watching for changes
// to NetworkFunction resources.
//
// It will automatically stop once the manager's context is cancelled.
func SetUpInformer(mgr ctrl.Manager) error {
	inf, err := mgr.GetCache().GetInformer(context.TODO(), &corev1alpha1.NetworkFunction{})
	if err != nil {
		return fmt.Errorf("failed to get network function informer: %v", err)
	}

	_, err = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			nf := obj.(*corev1alpha1.NetworkFunction)
			fmt.Println("Created:", nf.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			nf := newObj.(*corev1alpha1.NetworkFunction)
			fmt.Println("Updated:", nf.Name)
		},
		DeleteFunc: func(obj interface{}) {
			nf := obj.(*corev1alpha1.NetworkFunction)
			fmt.Println("Deleted:", nf.Name)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add network function informer's event handler: %v", err)
	}

	return nil
}
