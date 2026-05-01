package loomnode

import (
	"context"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/pkg/dataplane"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Reconciler reconciles a LoomNode object
type Reconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Dataplane dataplane.Dataplane
	Config    *env.GlobalConfig
}

// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	loomNode := &infrav1alpha1.LoomNode{}
	err := r.Get(ctx, req.NamespacedName, loomNode)
	if apierrors.IsNotFound(err) {
		logger.Info("LoomNode resource not found. Deleting node routes", "node", req.NamespacedName)
		err = r.Dataplane.RemoveNodeRoutes(req.NamespacedName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove node routes: %w", err)
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch loom node: %w", err)
	}

	err = r.Dataplane.UpdateNodeRoutes(loomNode)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update loom node routes: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.LoomNode{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool { return e.Object.GetName() != r.Config.NodeName },
				UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					newNode := e.ObjectNew.(*infrav1alpha1.LoomNode)
					if newNode.Name == r.Config.NodeName {
						return false
					}
					oldNode := e.ObjectOld.(*infrav1alpha1.LoomNode)
					return newNode.Generation != oldNode.Generation
				},
				DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool { return e.Object.GetName() != r.Config.NodeName },
			})).
		Named("loomnode-daemon").
		Complete(r)
}
