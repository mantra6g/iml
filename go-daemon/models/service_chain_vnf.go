package models

import (
	"github.com/google/uuid"
)

type ServiceChainVnfs struct {
	ChainID  uuid.UUID `gorm:"primaryKey"`
	Position uint8     `gorm:"primaryKey;autoIncrement:false;index:,sort:asc"`
	VnfID    uuid.UUID
	SubfunctionID *uint32 `gorm:"default:null"`
}
