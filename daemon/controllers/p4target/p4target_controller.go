package p4target

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/mantra6g/iml/daemon/env"
	"github.com/mantra6g/iml/daemon/pkg/dataplane"
	vrfutil "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	"github.com/mantra6g/iml/daemon/pkg/tunnel"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/api/equality"
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
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get;update;patch

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
		if err = r.Dataplane.RemoveP4TargetVRF(req.NamespacedName); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove P4Target VRF: %w", err)
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
	isLocal := nodeName != "" && nodeName == r.Config.NodeName
	isHostVRF := target.Spec.Attachment.Type == corev1alpha1.P4TargetAttachmentHostVRF
	if !isLocal || !isHostVRF {
		// This node doesn't (or no longer) host a VRF for this target.
		if err = r.Dataplane.RemoveP4TargetVRF(req.NamespacedName); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove P4Target VRF: %w", err)
		}
		if isLocal && target.Status.HostVRF != nil {
			if err = r.patchHostVRFStatus(ctx, target, nil); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if nodeName == "" || target.Spec.NfCIDR == "" {
		// Target hasn't been scheduled on a node or allocated an NF CIDR yet.
		return ctrl.Result{}, nil
	}

	if isLocal && isHostVRF {
		// The target lives in a host VRF on this node: steer its NF CIDR into the VRF and tell the plugin
		// where to find it.
		if err = r.removeRemoteRoute(req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
		info, err := r.Dataplane.EnsureP4TargetVRF(target)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set up VRF for P4Target %s: %w", req.Name, err)
		}
		return ctrl.Result{}, r.patchHostVRFStatus(ctx, target, hostVRFStatus(info))
	}

	if isLocal {
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

// patchHostVRFStatus sets status.hostVRF to desired, if it differs.
func (r *Reconciler) patchHostVRFStatus(
	ctx context.Context, target *corev1alpha1.P4Target, desired *corev1alpha1.P4TargetHostVRFStatus,
) error {
	if equality.Semantic.DeepEqual(target.Status.HostVRF, desired) {
		return nil
	}
	original := target.DeepCopy()
	target.Status.HostVRF = desired
	if err := r.Status().Patch(ctx, target, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to patch host VRF status of P4Target %s: %w", target.Name, err)
	}
	return nil
}

func hostVRFStatus(info *dataplane.P4TargetVRFInfo) *corev1alpha1.P4TargetHostVRFStatus {
	return &corev1alpha1.P4TargetHostVRFStatus{
		Name:      info.Name,
		Table:     info.Table,
		Interface: info.Interface,
		Addresses: dualStackAddressStrings(info.Addresses),
		Gateways:  dualStackAddressStrings(info.Gateways),
	}
}

func dualStackAddressStrings(addrs netutils.DualStackAddress) []string {
	var result []string
	for _, ip := range []net.IP{addrs.IPv6, addrs.IPv4} {
		if ip != nil {
			result = append(result, ip.String())
		}
	}
	return result
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
