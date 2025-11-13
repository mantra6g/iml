package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VnfGroup struct {
	ID         uuid.UUID `gorm:"primaryKey"`
	VnfID      uuid.UUID
	SID        string // in "IP/prefix" format
	Instances  []VnfInstance `gorm:"foreignKey:group_id"`
	Subnet     string
	GatewayIP  string
	Bridge     string
}

func (VnfGroup) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}

func (VnfGroup) AfterCreate(tx *gorm.DB) (err error) {
	// Automatically create segments for the VNF group
	// This is a placeholder for any logic that needs to run after creation
	return nil
}