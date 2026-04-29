package dataplane

import (
	"net"

	netutils "iml-daemon/pkg/utils/net"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"

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
	IfaceName       string
}

type Dataplane interface {
	Close() error

	ConfigureAppInstance(app *corev1alpha1.Application, containerID string) (*AppConfig, error)
	DeleteAppInstance(containerID string) error

	ConfigureP4TargetInstance(target *corev1alpha1.P4Target, containerID string) (*P4TargetConfig, error)
	DeleteP4TargetInstance(containerID string) error

	UpdateAppRoutes(app *corev1alpha1.Application) error
	RemoveAppRoutes(app client.ObjectKey) error

	UpdateP4TargetRoutes(target *corev1alpha1.P4Target) error
	RemoveP4TargetRoutes(target client.ObjectKey) error

	UpdateNodeRoutes(node *infrav1alpha1.LoomNode) error
	RemoveNodeRoutes(node client.ObjectKey) error

	AddServiceChainRoutes(service *corev1alpha1.ServiceChain, routes []SRv6Route) error
	ListServiceChainRoutes(service *corev1alpha1.ServiceChain) ([]SRv6Route, error)
	DeleteAllServiceChainRoutes(service client.ObjectKey) error
	//AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error
	//RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error
}
