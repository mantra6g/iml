package models

import (
	"net"

	"github.com/google/uuid"
)

type SidAssignment struct {
	VnfMultiplexedGroupID uuid.UUID `gorm:"primaryKey"`
	VNFSubfunctionID      uint32   `gorm:"primaryKey"`
	SID                   string  // IPNet
}
func (sa *SidAssignment) GetSID() net.IPNet {
	ip, ipNet, err := net.ParseCIDR(sa.SID)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: ipNet.Mask,
	}
}