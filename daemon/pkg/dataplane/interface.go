package dataplane

import (
	"context"
	"errors"
	"net"

	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SRv6Route struct {
	SourceApp      client.ObjectKey
	DestinationApp client.ObjectKey
	DestNet        netutils.DualStackNetwork
	FunctionIPs    []net.IP
}

type AppConfig struct {
	IPs          netutils.DualStackNetwork
	ClusterCIDRs netutils.DualStackNetwork
	ServiceCIDRs netutils.DualStackNetwork
	Gateways     netutils.DualStackAddress
	Bridge       string
	IfaceName    string
	MTU          uint32
}

type P4TargetConfig struct {
	IPv6Net         net.IPNet
	ClusterIPv6CIDR net.IPNet
	IPv6Gateway     net.IP
	Bridge          string
	MTU             int
	IfaceConfig     *InterfaceConfig
}

// InterfaceConfig names the host-side interfaces the CNI plugin creates for a containerized P4Target.
type InterfaceConfig struct {
	// CrossNamespaceInterfaces are the host-side peers of the target's nfX interfaces. Empty for a
	// single-interface target, which is connected through BridgeVethPeerName instead.
	CrossNamespaceInterfaces []string
	// BridgeVethPeerName is the host-side peer of a single-interface target's iml0 interface.
	BridgeVethPeerName string
}

// ErrP4TargetLinksMissing reports that the interfaces a P4Target is wired through don't exist yet:
// the CNI plugin hasn't created them, or the device hasn't exposed them.
var ErrP4TargetLinksMissing = errors.New("P4Target host interfaces are missing")

// ReusesHostInterfaces reports whether the target is reached through interfaces the device exposes
// on the host by itself (e.g. a BlueField-3's nfX), so the CNI plugin is never involved.
func ReusesHostInterfaces(target *corev1alpha1.P4Target) bool {
	return target.Spec.Network != nil &&
		target.Spec.Network.HostInterfaces != nil &&
		target.Spec.Network.HostInterfaces.Strategy == corev1alpha1.HostInterfaceStrategyReuse
}

type Dataplane interface {
	Shutdown(ctx context.Context) error

	ConfigureAppInstance(app *corev1alpha1.Application, containerID string) (*AppConfig, error)
	DeleteAppInstance(containerID string) error

	ConfigureP4TargetInstance(target *corev1alpha1.P4Target) (*P4TargetConfig, error)
	AssignP4TargetIPs(ctx context.Context, target *corev1alpha1.P4Target) (netutils.DualStackNetwork, error)
	// EnsureP4TargetHostWiring connects the target's host-side interfaces to the routing bridge and
	// attaches its eBPF programs. It is idempotent and returns ErrP4TargetLinksMissing if the
	// interfaces don't exist yet.
	EnsureP4TargetHostWiring(target *corev1alpha1.P4Target) error
	// TeardownP4TargetHostWiring undoes EnsureP4TargetHostWiring, keeping the target's IPs.
	TeardownP4TargetHostWiring(target client.ObjectKey) error
	// DeleteP4TargetInstance tears down the target's wiring and forgets everything about it.
	DeleteP4TargetInstance(target client.ObjectKey) error

	UpdateAppRoutes(app *corev1alpha1.Application) error
	RemoveAppRoutes(app client.ObjectKey) error

	UpdateP4TargetRoutes(target *corev1alpha1.P4Target) error
	RemoveP4TargetRoutes(target client.ObjectKey) error

	AddServiceChainRoutes(service *corev1alpha1.ServiceChain, routes []SRv6Route) error
	ListServiceChainRoutes(service *corev1alpha1.ServiceChain) ([]SRv6Route, error)
	DeleteAllServiceChainRoutes(service client.ObjectKey) error
	//AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error
	//RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error
}
