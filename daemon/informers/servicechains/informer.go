package servicechains

import (
	"context"
	"fmt"
	corev1alpha1 "iml-daemon/api/core/v1alpha1"

	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetUpInformer sets up the informer for ServiceChain resources.
// This function should be called during the initialization of the controller
// manager to ensure that the informer is registered and starts watching for changes
// to ServiceChain resources.
//
// It will automatically stop once the manager's context is cancelled.
func SetUpInformer(mgr ctrl.Manager) error {
	inf, err := mgr.GetCache().GetInformer(context.TODO(), &corev1alpha1.ServiceChain{})
	if err != nil {
		return fmt.Errorf("failed to get service chain informer: %v", err)
	}

	_, err = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			sc := obj.(*corev1alpha1.ServiceChain)
			fmt.Println("Created:", sc.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			sc := newObj.(*corev1alpha1.ServiceChain)
			fmt.Println("Updated:", sc.Name)
		},
		DeleteFunc: func(obj interface{}) {
			sc := obj.(*corev1alpha1.ServiceChain)
			fmt.Println("Deleted:", sc.Name)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add service chain informer's event handler: %v", err)
	}

	return nil
}
