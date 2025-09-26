package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ServiceChainStatusActive          string = "ACTIVE"
	ServiceChainStatusDeletionPending string = "DELETION_PENDING"
)

type ServiceChain struct {
	ID       uuid.UUID `gorm:"primaryKey"`
	GlobalID string
	Status   string    `gorm:"index:status,default:ACTIVE"`
	Etag 	 int
	SrcAppID uuid.UUID
	DstAppID uuid.UUID
	SrcApp   Application `gorm:"foreignKey:src_app_id"`
	DstApp   Application `gorm:"foreignKey:dst_app_id"`
	Elements []ServiceChainVnfs `gorm:"foreignKey:network_service_chain_id"`
}

func (ServiceChain) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
