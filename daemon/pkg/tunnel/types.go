package tunnel

import (
	"context"

	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	corev1 "k8s.io/api/core/v1"
)

type Manager interface {
	UpdateNodeTunnels(node *corev1.Node) error
	DeleteNodeTunnels(nodeName string) error
	GetTunnelInterface(nodeName string) (string, error)
	// AddEgressRoute ensures that traffic destined to dst is encapsulated and sent towards the
	// tunnel endpoint of the node identified by nodeName. It is idempotent and may be called
	// again to update the destination network(s) routed towards that node's tunnel.
	AddEgressRoute(nodeName string, dst netutils.DualStackNetwork) error
	// RemoveEgressRoute undoes a previous AddEgressRoute call for the given node and destination.
	RemoveEgressRoute(nodeName string, dst netutils.DualStackNetwork) error
	Shutdown(ctx context.Context) error
}
