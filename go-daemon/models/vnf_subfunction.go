package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Subfunction struct {
	ID						uuid.UUID `gorm:"primaryKey"`
	SubfunctionID uint32    `gorm:"uniqueIndex:idx_vnf_subfunction"`
	VnfID         uuid.UUID `gorm:"uniqueIndex:idx_vnf_subfunction"`
}

func (Subfunction) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}