package dataplane

import (
	"net"

	"github.com/google/uuid"
)

type VNFSubnet interface {
	Network() *net.IPNet
	GatewayIP() net.IP
	SIDs() []net.IPNet
	Bridge() string
}

type AppSubnet interface {
	Network() *net.IPNet
	GatewayIP() net.IP
	Bridge() string
}

type Manager interface {
	Close() error

	AddVNFSubnet(vnfGroupID uuid.UUID, sidAmount int) (VNFSubnet, error)
	AddApplicationSubnet(appGroupID uuid.UUID) (AppSubnet, error)

	RemoveApplicationSubnet(appGroupID uuid.UUID)
	RemoveVNFSubnet(vnfGroupID uuid.UUID)

	AddApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) (*net.IPNet, string, error)
	RemoveApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) error

	AddVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) (*net.IPNet, string, error)
	RemoveVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) error

	AddRoute(srcAppGroup uuid.UUID, dstNet net.IPNet, sids []net.IP) error
	RemoveRoute(srcAppGroup uuid.UUID, dstNet string) error
}
