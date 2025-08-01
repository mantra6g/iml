package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceChain struct {
	ID         uuid.UUID `gorm:"primaryKey"`
	GlobalID   string    `gorm:"uniqueIndex:chain_global_id"`
	SrcAppID   uuid.UUID
	DstAppID   uuid.UUID
	SrcApp     Application `gorm:"foreignKey:src_app_id"`
	DstApp     Application `gorm:"foreignKey:dst_app_id"`
	Elements   []ServiceChainVnfs `gorm:"foreignKey:network_service_chain_id"`
}

func (ServiceChain) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
