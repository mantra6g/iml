package tunnel

import (
	corev1 "k8s.io/api/core/v1"
)

type Manager interface {
	UpdateNodeTunnels(node *corev1.Node) error
	DeleteNodeTunnels(nodeName string) error
	GetTunnelInterface(nodeName string) (string, error)
	Close() error
}
