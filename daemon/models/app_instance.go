package models

import (
	"net"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AppInstance struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	GroupID     uuid.UUID
	ContainerID string
	IfaceName   string // e.g., "nfr-aabbcc"
	IP          string // IPNets
}

func (ai AppInstance) GetIP() net.IPNet {
	ip, ipNet, err := net.ParseCIDR(ai.IP)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: ipNet.Mask,
	}
}

func (AppInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
