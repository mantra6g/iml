package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AppStatusActive          string = "ACTIVE"
	AppStatusDeletionPending string = "DELETION_PENDING"
)

type Application struct {
	ID       uuid.UUID `gorm:"primaryKey"` // Surrogate key
	GlobalID string
	Status   string `gorm:"index:idx_applications_status,default:ACTIVE"`
	Etag     int
	Groups   []AppGroup `gorm:"foreignKey:app_id"`
}

func (Application) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
