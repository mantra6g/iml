package tunnel

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Manager interface {
	UpdateNodeTunnels(node *corev1.Node) error
	DeleteNodeTunnels(nodeID types.UID) error
	GetTunnelInterface(nodeID types.UID) (string, error)
	Close() error
}
