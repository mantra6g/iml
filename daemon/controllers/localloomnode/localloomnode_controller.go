package localloomnode

import (
	"context"
	"fmt"

	"github.com/mantra6g/iml/daemon/env"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	cmputils "github.com/mantra6g/iml/daemon/pkg/utils/cmp"

	v1 "k8s.io/api/core/v1"
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
	Scheme *runtime.Scheme
	Config *env.GlobalConfig
}

// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// This controller reacts to changes on its own node's v1.Node object and keeps this node's
// LoomNode.Spec.Addresses in sync with the node's InternalIP/ExternalIP addresses. If the
// LoomNode resource is deleted, we don't recreate it here — env.SetUpNode owns bootstrapping it.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	node := &v1.Node{}
	err := r.Get(ctx, req.NamespacedName, node)
	if apierrors.IsNotFound(err) {
		logger.V(1).Info("Node resource not found, nothing to sync", "node", req.NamespacedName)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get node %s: %w", req.Name, err)
	}

	addresses := make([]infrav1alpha1.Address, 0, len(node.Status.Addresses))
	for _, addr := range node.Status.Addresses {
		if addr.Type != v1.NodeInternalIP && addr.Type != v1.NodeExternalIP {
			continue
		}
		addresses = append(addresses, infrav1alpha1.Address{
			IP:   addr.Address,
			Type: infrav1alpha1.AddressType(addr.Type),
		})
	}

	loomNode := &infrav1alpha1.LoomNode{}
	err = r.Get(ctx, client.ObjectKey{Name: r.Config.NodeName}, loomNode)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get loom node %s: %w", r.Config.NodeName, err)
	}

	if cmputils.ElementsMatchInAnyOrder(loomNode.Spec.Addresses, addresses) {
		return ctrl.Result{}, nil
	}

	original := loomNode.DeepCopy()
	loomNode.Spec.Addresses = addresses
	if err = r.Patch(ctx, loomNode, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update loom node addresses: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Node{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool { return e.Object.GetName() == r.Config.NodeName },
				UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
					if e.ObjectOld == nil || e.ObjectNew == nil {
						return false
					}
					newNode := e.ObjectNew.(*v1.Node)
					if newNode.Name != r.Config.NodeName {
						return false
					}
					oldNode := e.ObjectOld.(*v1.Node)
					return !cmputils.ElementsMatchInAnyOrder(newNode.Status.Addresses, oldNode.Status.Addresses)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool { return e.Object.GetName() == r.Config.NodeName },
			})).
		Named("local-loomnode-daemon").
		Complete(r)
}
