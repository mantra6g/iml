package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Route struct {
	ID              uuid.UUID `gorm:"primaryKey"`
	ChainID         uuid.UUID
	SrcAppGroupID   uuid.UUID
	DstAppGroupID   uuid.UUID
	Stages          []VnfGroup `gorm:"many2many:route_stages"`
}

func (Route) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}