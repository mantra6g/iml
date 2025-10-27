package models

import (
	"github.com/google/uuid"
)

type Route struct {
	ID              uuid.UUID `gorm:"primaryKey"`
	ChainID         uuid.UUID
	SrcAppGroupID   uuid.UUID
	DstAppGroupID   uuid.UUID
	Stages          []VnfGroup `gorm:"many2many:route_stages"`
}