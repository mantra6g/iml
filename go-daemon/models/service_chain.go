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
	Status   string `gorm:"index:idx_service_chains_status,default:ACTIVE"`
	SrcAppID uuid.UUID
	DstAppID uuid.UUID
	Elements []ServiceChainVnfs `gorm:"foreignKey:chain_id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (ServiceChain) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
