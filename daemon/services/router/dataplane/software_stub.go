// helper_stub.go
//go:build !linux && !windows

package dataplane

import (
	"fmt"
	"net"

	"github.com/google/uuid"
)

type Software struct {
}

func (s *Software) Close() error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) AddVNFSubnet(vnfGroupID uuid.UUID, sidAmount int) (VNFSubnet, error) {
	return nil, fmt.Errorf("unsupported architecture")
}
func (s *Software) AddApplicationSubnet(appGroupID uuid.UUID) (AppSubnet, error) {
	return nil, fmt.Errorf("unsupported architecture")
}

func (s *Software) RemoveApplicationSubnet(appGroupID uuid.UUID) {}
func (s *Software) RemoveVNFSubnet(vnfGroupID uuid.UUID)         {}

func (s *Software) AddApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) (*net.IPNet, string, error) {
	return nil, "", fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) AddVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) (*net.IPNet, string, error) {
	return nil, "", fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) error {
	return fmt.Errorf("unsupported architecture")
}

func (s *Software) AddRoute(srcAppGroup uuid.UUID, dstNet net.IPNet, sids []net.IP) error {
	return fmt.Errorf("unsupported architecture")
}
func (s *Software) RemoveRoute(srcAppGroup uuid.UUID, dstNet string) error {
	return fmt.Errorf("unsupported architecture")
}

func NewSoftware(sidRange *net.IPNet, appRange *net.IPNet, vnfRange *net.IPNet, tunRange *net.IPNet) (Manager, error) {
	return nil, fmt.Errorf("unsupported architecture")
}
