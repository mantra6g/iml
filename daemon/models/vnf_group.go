package models

import (
	"net"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VnfGroup interface {
	GetID() uuid.UUID
	GetVnfID() uuid.UUID
	GetSubnet() net.IPNet
	GetGatewayIP() net.IP
	GetBridge() string

	Save(dbClient *gorm.DB) error
	Delete(dbClient *gorm.DB) error
}

type BaseVnfGroup struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	VnfID     uuid.UUID
	Subnet    string // IPNets
	GatewayIP string // IP
	Bridge    string
}

func (BaseVnfGroup) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
func (bg BaseVnfGroup) GetID() uuid.UUID {
	return bg.ID
}
func (bg BaseVnfGroup) GetVnfID() uuid.UUID {
	return bg.VnfID
}
func (bg BaseVnfGroup) GetSubnet() net.IPNet {
	ip, subnet, err := net.ParseCIDR(bg.Subnet)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: subnet.Mask,
	}
}
func (bg BaseVnfGroup) GetGatewayIP() net.IP {
	return net.ParseIP(bg.GatewayIP)
}
func (bg BaseVnfGroup) GetBridge() string {
	return bg.Bridge
}

type SimpleVnfGroup struct {
	BaseVnfGroup `gorm:"embedded"`
	// ID        uuid.UUID `gorm:"primaryKey"`
	// VnfID     uuid.UUID
	SID       string              // IPNets
	Instances []SimpleVnfInstance `gorm:"foreignKey:group_id"`
	Stages    []RouteStage        `gorm:"polymorphic:VnfGroup;"`
	// Subnet    net.IPNets
	// Gateways net.IP
	// Bridge    string
}

func (sg *SimpleVnfGroup) GetSID() net.IPNet {
	ip, ipNet, err := net.ParseCIDR(sg.SID)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: ipNet.Mask,
	}
}
func (sg *SimpleVnfGroup) Save(dbClient *gorm.DB) error {
	return dbClient.Save(sg).Error
}
func (sg *SimpleVnfGroup) Delete(dbClient *gorm.DB) error {
	return dbClient.Delete(&SimpleVnfGroup{}, "id = ?", sg.ID).Error
}

type MultiplexedVnfGroup struct {
	BaseVnfGroup `gorm:"embedded"`
	// ID         uuid.UUID `gorm:"primaryKey"`
	// VnfID      uuid.UUID
	SidAssignments []SidAssignment          `gorm:"foreignKey:VnfMultiplexedGroupID"`
	Instances      []MultiplexedVnfInstance `gorm:"foreignKey:group_id"`
	Stages         []RouteStage             `gorm:"polymorphic:VnfGroup;"`
	// Subnet     net.IPNets
	// Gateways  net.IP
	// Bridge     string
}

func (mg *MultiplexedVnfGroup) GetSubfunctionSIDs() map[uint32]net.IPNet {
	sidMap := make(map[uint32]net.IPNet)
	for _, assignment := range mg.SidAssignments {
		sidMap[assignment.VNFSubfunctionID] = assignment.GetSID()
	}
	return sidMap
}
func (mg *MultiplexedVnfGroup) Save(dbClient *gorm.DB) error {
	return dbClient.Save(mg).Error
}
func (mg *MultiplexedVnfGroup) Delete(dbClient *gorm.DB) error {
	return dbClient.Delete(&MultiplexedVnfGroup{}, "id = ?", mg.ID).Error
}
