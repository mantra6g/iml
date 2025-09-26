package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	VNFStatusActive          string = "ACTIVE"
	VNFStatusDeletionPending string = "DELETION_PENDING"
)

type VirtualNetworkFunction struct {
	ID       uuid.UUID  `gorm:"primaryKey"`
	GlobalID string
	Status   string     `gorm:"index:status,default:ACTIVE"`
	Etag	 int
	Groups   []VnfGroup `gorm:"foreignKey:vnf_id"`
}

func (VirtualNetworkFunction) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}