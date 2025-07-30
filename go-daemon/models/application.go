package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Application struct {
	ID          uuid.UUID     `gorm:"primaryKey"` // Surrogate key
	IMLAppID    string        `gorm:"uniqueIndex:iml_app_id"`
	Instances   []AppInstance `gorm:"foreignKey:function_id"`
}

func (Application) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}