package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RemoteAppGroup struct {
	ID              uuid.UUID `gorm:"primaryKey"` // Surrogate key
	NodeID          uuid.UUID
	AppID           uuid.UUID
	ExternalGroupID string
	Instances       []RemoteAppInstance `gorm:"foreignKey:group_id;constraint:OnDelete:CASCADE"`
}

func (RemoteAppGroup) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}