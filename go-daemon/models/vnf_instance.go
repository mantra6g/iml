package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VnfInstance struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	GroupID     uuid.UUID
	IP          string // in "ip"/"prefix" format
	ContainerID string
	IfaceName   string // e.g., "nfr-aabbcc"
}

func (VnfInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}