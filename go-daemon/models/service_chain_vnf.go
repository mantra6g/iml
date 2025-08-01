package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceChainVnfs struct {
	ChainID  uuid.UUID `gorm:"primaryKey"`
	Position uint8     `gorm:"primaryKey;autoIncrement:false;index:,sort:asc"`
	VnfID    uuid.UUID
	Vnf      VirtualNetworkFunction `gorm:"foreignKey:vnf_id"`
}

func (ServiceChainVnfs) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("chain_id", randomID)
	return
}
