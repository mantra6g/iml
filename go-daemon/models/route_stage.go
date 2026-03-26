package models

import "github.com/google/uuid"

type RouteStage struct {
	RouteID      uuid.UUID `gorm:"primaryKey"`
	Position     uint8     `gorm:"primaryKey;index:,sort:asc"`
	VnfGroupID   uuid.UUID 
	VnfGroupType string  
}
