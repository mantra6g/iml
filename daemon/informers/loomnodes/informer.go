package loomnodes

import (
	"context"
	"fmt"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"

	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetUpInformer sets up the informer for LoomNode resources.
// This function should be called during the initialization of the controller
// manager to ensure that the informer is registered and starts watching for changes
// to LoomNode resources.
//
// It will automatically stop once the manager's context is canceled.
func SetUpInformer(mgr ctrl.Manager) error {
	inf, err := mgr.GetCache().GetInformer(context.TODO(), &infrav1alpha1.LoomNode{})
	if err != nil {
		return fmt.Errorf("failed to get loomnode informer: %v", err)
	}

	_, err = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*infrav1alpha1.LoomNode)
			fmt.Println("Created:", node.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			loomNode := newObj.(*infrav1alpha1.LoomNode)
			fmt.Println("Updated:", loomNode.Name)
		},
		DeleteFunc: func(obj interface{}) {
			loomNodeTombstone := obj.(*infrav1alpha1.LoomNode)
			fmt.Println("Deleted:", loomNodeTombstone.Name)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add loomnode informer's event handler: %v", err)
	}

	return nil
}
