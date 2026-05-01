package tunnel

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

type Manager interface {
	UpdateNodeTunnels(node *corev1.Node) error
	DeleteNodeTunnels(nodeName string) error
	GetTunnelInterface(nodeName string) (string, error)
	Shutdown(ctx context.Context) error
}
