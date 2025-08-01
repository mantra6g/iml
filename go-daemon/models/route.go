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

func (r *Route) AfterSave(tx *gorm.DB) (err error) {
	// TODO: Create the corresponding dataplane route
	return nil
}