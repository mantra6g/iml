package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AppInstance struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	GroupID     uuid.UUID
	ContainerID string
	IfaceName   string // e.g., "nfr-aabbcc"
	IP          string // in "IP/prefix" format
}

func (AppInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}

func (a AppInstance) GetIP() string {
	return a.IP
}