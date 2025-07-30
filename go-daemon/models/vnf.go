package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VirtualNetworkFunction struct {
	ID        uuid.UUID  `gorm:"primaryKey"`
	IMLVnfID  string     `gorm:"uniqueIndex:iml_vnf_id"`
	Groups    []VnfGroup `gorm:"foreignKey:vnf_id"`
}

func (VirtualNetworkFunction) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}