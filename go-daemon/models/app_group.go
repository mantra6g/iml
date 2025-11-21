package models

import (
	"net"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AppGroup represents a group of application instances.
type AppGroup struct {
	ID         uuid.UUID `gorm:"primaryKey"` // Surrogate key
	AppID      uuid.UUID
	Instances  []AppInstance `gorm:"foreignKey:group_id"`
	Subnet     string // IPNet
	GatewayIP  string // IP
	Bridge     string
}
func (ag AppGroup) GetSubnet() net.IPNet {
	ip, subnet, err := net.ParseCIDR(ag.Subnet)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: subnet.Mask,
	}
}
func (ag AppGroup) GetGatewayIP() net.IP {
	return net.ParseIP(ag.GatewayIP)
}

func (AppGroup) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}