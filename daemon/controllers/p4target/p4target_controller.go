package p4target

import (
	"context"
	"fmt"
	"sync"

	"github.com/mantra6g/iml/daemon/env"
	"github.com/mantra6g/iml/daemon/pkg/dataplane"
	vrfutil "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	"github.com/mantra6g/iml/daemon/pkg/tunnel"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// remoteRoute records an NF CIDR routed through the tunnel towards the node hosting a P4Target.
type remoteRoute struct {
	nodeName string
	nfCIDR   string
}

// Reconciler reconciles a P4Target object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Dataplane     dataplane.Dataplane
	TunnelManager tunnel.Manager
	Config        *env.GlobalConfig

	mu           sync.Mutex
	remoteRoutes map[types.NamespacedName]remoteRoute
}

// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	r.mu.Lock()
	defer r.mu.Unlock()

	target := &corev1alpha1.P4Target{}
	err := r.Get(ctx, req.NamespacedName, target)
	if apierrors.IsNotFound(err) {
		logger.Info("P4Target resource not found. Deleting target routes", "target", req.NamespacedName)
		if err = r.removeRemoteRoute(req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
		err = r.Dataplane.RemoveP4TargetRoutes(req.NamespacedName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove P4Target routes: %w", err)
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get p4target %s: %w", req.Name, err)
	}

	nodeName := target.Status.NodeName
	if nodeName == "" || target.Spec.NfCIDR == "" {
		// Target hasn't been scheduled on a node or allocated an NF CIDR yet.
		return ctrl.Result{}, nil
	}

	if nodeName == r.Config.NodeName {
		// The target is attached to this node's routing bridge, so it can be routed to directly.
		if err = r.removeRemoteRoute(req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
		err = r.Dataplane.UpdateP4TargetRoutes(target)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update P4Target routes: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// The target lives on a peer node: send its NF CIDR through the tunnel towards that node,
	// whose routing VRF forwards it to the target.
	desired := remoteRoute{nodeName: nodeName, nfCIDR: target.Spec.NfCIDR}
	if current, exists := r.remoteRoutes[req.NamespacedName]; exists && current != desired {
		if err = r.removeRemoteRoute(req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
	}
	nfCIDR, err := vrfutil.ParseDualStackNetworkFromStrings([]string{target.Spec.NfCIDR})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse nf CIDR for P4Target %s: %w", req.Name, err)
	}
	if err = r.TunnelManager.AddEgressRoute(nodeName, nfCIDR); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to route P4Target %s towards node %s: %w", req.Name, nodeName, err)
	}
	if r.remoteRoutes == nil {
		r.remoteRoutes = make(map[types.NamespacedName]remoteRoute)
	}
	r.remoteRoutes[req.NamespacedName] = desired

	return ctrl.Result{}, nil
}

// removeRemoteRoute removes the tunnel route previously installed for the target, if any.
func (r *Reconciler) removeRemoteRoute(key types.NamespacedName) error {
	current, exists := r.remoteRoutes[key]
	if !exists {
		return nil
	}
	nfCIDR, err := vrfutil.ParseDualStackNetworkFromStrings([]string{current.nfCIDR})
	if err != nil {
		return fmt.Errorf("failed to parse nf CIDR %s: %w", current.nfCIDR, err)
	}
	if err = r.TunnelManager.RemoveEgressRoute(current.nodeName, nfCIDR); err != nil {
		return fmt.Errorf("failed to remove route for P4Target %s towards node %s: %w",
			key.Name, current.nodeName, err)
	}
	delete(r.remoteRoutes, key)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.P4Target{}).
		Named("p4target-daemon").
		Complete(r)
}
