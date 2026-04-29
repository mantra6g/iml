package node

import (
	"context"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/pkg/tunnel"

	cmputils "iml-daemon/pkg/utils/cmp"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Reconciler reconciles a Node object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	TunnelManager tunnel.Manager
	Config        *env.GlobalConfig
}

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	node := &corev1.Node{}
	err := r.Get(ctx, req.NamespacedName, node)
	if apierrors.IsNotFound(err) {
		logger.V(1).Info("Node resource not found. Deleting tunnels", "node", req.NamespacedName)
		err = r.TunnelManager.DeleteNodeTunnels(req.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete node tunnels: %w", err)
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get node %s: %w", req.Name, err)
	}

	err = r.TunnelManager.UpdateNodeTunnels(node)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update node tunnels: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{},
			// Only receive change events when the node's addresses change, as that's the only thing we care about for now.
			// Also skip events from this node. Don't care about those.
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool { return e.Object.GetName() != r.Config.NodeName },
				UpdateFunc: func(e event.UpdateEvent) bool {
					newNode := e.ObjectNew.(*corev1.Node)
					if newNode.Name == r.Config.NodeName {
						return false
					}
					oldNode := e.ObjectOld.(*corev1.Node)
					return !cmputils.ElementsMatchInAnyOrder(newNode.Status.Addresses, oldNode.Status.Addresses)
				},
				DeleteFunc: func(e event.DeleteEvent) bool { return e.Object.GetName() != r.Config.NodeName },
			})).
		Named("node-daemon").
		Complete(r)
}
