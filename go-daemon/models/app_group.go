package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AppGroup represents a group of application instances.
type AppGroup struct {
	ID         uuid.UUID `gorm:"primaryKey"` // Surrogate key
	AppID      uuid.UUID
	Instances  []AppInstance `gorm:"foreignKey:group_id"`
	Subnet     string
	GatewayIP  string
	Bridge     string
}

func (AppGroup) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}