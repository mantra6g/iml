package tunnel

import (
	"context"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"
)

type Manager interface {
	UpdateNodeTunnels(loomNode *infrav1alpha1.LoomNode) error
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
