package dataplane

import (
	"net"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	"iml-daemon/pkg/netutils"

	"k8s.io/apimachinery/pkg/types"
)

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
	RemoveAppRoutes(app *corev1alpha1.Application) error

	UpdateP4TargetRoutes(target *corev1alpha1.P4Target) error
	RemoveP4TargetRoutes(target *corev1alpha1.P4Target) error

	UpdateNodeRoutes(node *infrav1alpha1.LoomNode) error
	RemoveNodeRoutes(node *infrav1alpha1.LoomNode) error

	AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error
	RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error
}
