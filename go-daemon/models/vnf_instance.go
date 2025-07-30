package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VnfInstance struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	GroupID     uuid.UUID
	Group       VnfGroup  `gorm:"foreignKey:group_id"`
	ContainerID string
	IntfName    string // e.g., "nfr-aabbcc"
}

func (VnfInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}

func (v VnfInstance) GetIP() string {
	return v.Group.SID
}