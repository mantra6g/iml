package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Application struct {
	ID        uuid.UUID   `gorm:"primaryKey"` // Surrogate key
	GlobalID  string      `gorm:"uniqueIndex:app_global_id"`
	Groups    []AppGroup  `gorm:"foreignKey:app_id"`
}

func (Application) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}