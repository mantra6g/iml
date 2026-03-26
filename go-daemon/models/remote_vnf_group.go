package models

import "github.com/google/uuid"

type RemoteVnfGroup struct {
	ID              uuid.UUID `gorm:"primaryKey"` // Surrogate key
	ExternalGroupID string
	VnfID           uuid.UUID
	WorkerID        uuid.UUID
	SID             string // IPNet
}