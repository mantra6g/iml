package vrf

import (
	"fmt"
	"net"

	"iml-daemon/pkg/dataplane"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
)

type Software struct {
}

func (s *Software) Close() error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) ConfigureAppInstance(app *corev1alpha1.Application, containerID string,
) (*dataplane.AppConfig, error) {
	return nil, fmt.Errorf("Windows is not supported yet")
}
func (s *Software) DeleteAppInstance(containerID string) error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) ConfigureP4TargetInstance(target *corev1alpha1.P4Target, containerID string,
) (*dataplane.P4TargetConfig, error) {
	return nil, fmt.Errorf("Windows is not supported yet")
}
func (s *Software) DeleteP4TargetInstance(containerID string) error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) UpdateApp(app *corev1alpha1.Application) error {
	return fmt.Errorf("Windows is not supported yet")
}
func (s *Software) RemoveApp(app *corev1alpha1.Application) error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) UpdateP4Target(target *corev1alpha1.P4Target) error {
	return fmt.Errorf("Windows is not supported yet")
}
func (s *Software) RemoveP4Target(target *corev1alpha1.P4Target) error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) UpdateNode(node *infrav1alpha1.LoomNode) error {
	return fmt.Errorf("Windows is not supported yet")
}
func (s *Software) RemoveNode(node *infrav1alpha1.LoomNode) error {
	return fmt.Errorf("Windows is not supported yet")
}

func (s *Software) AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error {
	return fmt.Errorf("Windows is not supported yet")
}
func (s *Software) RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error {
	return fmt.Errorf("Windows is not supported yet")
}
