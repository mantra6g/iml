// helper_stub.go
//go:build !linux && !windows

package vrf

import (
	"fmt"
	"net"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	"iml-daemon/pkg/dataplane"

	"k8s.io/apimachinery/pkg/types"
)

type Software struct {
}

func (s *Software) Close() error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) ConfigureAppInstance(app *corev1alpha1.Application, containerID string,
) (*dataplane.AppConfig, error) {
	return nil, fmt.Errorf("unsupported architecture")
}
func (s *Software) DeleteAppInstance(containerID string) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) ConfigureP4TargetInstance(target *corev1alpha1.P4Target, containerID string,
) (*dataplane.P4TargetConfig, error) {
	return nil, fmt.Errorf("unsupported architecture")
}
func (s *Software) DeleteP4TargetInstance(containerID string) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) UpdateApp(app *corev1alpha1.Application) error {
	return fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveApp(app *corev1alpha1.Application) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) UpdateP4Target(target *corev1alpha1.P4Target) error {
	return fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveP4Target(target *corev1alpha1.P4Target) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) UpdateNode(node *infrav1alpha1.LoomNode) error {
	return fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveNode(node *infrav1alpha1.LoomNode) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error {
	return fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error {
	return fmt.Errorf("unsupported architecture")
}
