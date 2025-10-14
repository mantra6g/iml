package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RemoteAppInstance struct {
	ID      uuid.UUID `gorm:"primaryKey"`
	GroupID uuid.UUID
	Group   RemoteAppGroup `gorm:"foreignKey:group_id"`
	IP      string // in "IP/prefix" format
}

func (RemoteAppInstance) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}