package models

import "github.com/google/uuid"

type RouteStage struct {
	RouteID    uuid.UUID `gorm:"primaryKey"`
	VnfGroupID uuid.UUID `gorm:"primaryKey"`
	Position   uint8     `gorm:"index:,sort:asc"`
}
