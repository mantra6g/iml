package models

import (
	"net"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VnfInstance defines the interface for VNF instances.
type VnfInstance interface {
	GetID() uuid.UUID
	GetGroupID() uuid.UUID
	GetIP() net.IPNet
	GetContainerID() string
	GetIfaceName() string

	Save(dbClient *gorm.DB) error
	Delete(dbClient *gorm.DB) error
}

// BaseVnfInstance provides common fields for VNF instances.
type BaseVnfInstance struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	GroupID     uuid.UUID
	IP          string // IPNets
	ContainerID string
	IfaceName   string
}

func (bi BaseVnfInstance) GetID() uuid.UUID {
	return bi.ID
}
func (bi BaseVnfInstance) GetGroupID() uuid.UUID {
	return bi.GroupID
}
func (bi BaseVnfInstance) GetIP() net.IPNet {
	ip, ipNet, err := net.ParseCIDR(bi.IP)
	if err != nil {
		return net.IPNet{}
	}
	return net.IPNet{
		IP:   ip,
		Mask: ipNet.Mask,
	}
}
func (bi BaseVnfInstance) GetContainerID() string {
	return bi.ContainerID
}
func (bi BaseVnfInstance) GetIfaceName() string {
	return bi.IfaceName
}
func (bi BaseVnfInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}

// SimpleVnfInstance represents an instance of a simple VNF within a SimpleVnfGroup.
type SimpleVnfInstance struct {
	BaseVnfInstance `gorm:"embedded"`
	// ID          uuid.UUID `gorm:"primaryKey"`
	// GroupID     uuid.UUID
	// IP          net.IPNets // in "ip"/"prefix" format
	// ContainerID string
	// IfaceName   string // e.g., "nfr-aabbcc"
}

func (si *SimpleVnfInstance) Save(dbClient *gorm.DB) error {
	return dbClient.Save(si).Error
}
func (si *SimpleVnfInstance) Delete(dbClient *gorm.DB) error {
	return dbClient.Delete(&SimpleVnfInstance{}, "id = ?", si.ID).Error
}

// MultiplexedVnfInstance represents an instance of a multiplexed VNF within a MultiplexedVnfGroup.
type MultiplexedVnfInstance struct {
	BaseVnfInstance `gorm:"embedded"`
	// ID                 uuid.UUID `gorm:"primaryKey"`
	// MultiplexedGroupID uuid.UUID
	// ContainerID        string
	// IfaceName          string    // e.g., "nfr-aabbcc"
	// IP                 net.IPNets // in "IP/prefix" format
}

func (mi *MultiplexedVnfInstance) Save(dbClient *gorm.DB) error {
	return dbClient.Save(mi).Error
}
func (mi *MultiplexedVnfInstance) Delete(dbClient *gorm.DB) error {
	return dbClient.Delete(&MultiplexedVnfInstance{}, "id = ?", mi.ID).Error
}
