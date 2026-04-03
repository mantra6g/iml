package p4targets

import (
	"context"
	"fmt"
	corev1alpha1 "iml-daemon/api/core/v1alpha1"

	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetUpInformer sets up the informer for P4Targets resources.
// This function should be called during the initialization of the controller
// manager to ensure that the informer is registered and starts watching for changes
// to P4Targets resources.
//
// It will automatically stop once the manager's context is canceled.
func SetUpInformer(mgr ctrl.Manager) error {
	inf, err := mgr.GetCache().GetInformer(context.TODO(), &corev1alpha1.P4Target{})
	if err != nil {
		return fmt.Errorf("failed to get p4target informer: %v", err)
	}

	_, err = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			target := obj.(*corev1alpha1.P4Target)
			fmt.Println("Created:", target.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			target := newObj.(*corev1alpha1.P4Target)
			fmt.Println("Updated:", target.Name)
		},
		DeleteFunc: func(obj interface{}) {
			target := obj.(*corev1alpha1.P4Target)
			fmt.Println("Deleted:", target.Name)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add p4target informer's event handler: %v", err)
	}

	return nil
}
