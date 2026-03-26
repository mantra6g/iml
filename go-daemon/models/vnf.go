package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	VNFStatusActive          string = "ACTIVE"
	VNFStatusDeletionPending string = "DELETION_PENDING"
)

type NetworkFunctionType string

const (
	NetworkFunctionTypeSimple      NetworkFunctionType = "simple"
	NetworkFunctionTypeMultiplexed NetworkFunctionType = "multiplexed"
)

type VirtualNetworkFunction struct {
	ID              uuid.UUID `gorm:"primaryKey"`
	GlobalID        string
	Status          string `gorm:"index:idx_virtual_network_functions_status,default:ACTIVE"`
	Type            NetworkFunctionType `gorm:"default:'simple'"`
	Subfunctions    []Subfunction       `gorm:"foreignKey:vnf_id;constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
	SimpleVnfGroups []SimpleVnfGroup    `gorm:"foreignKey:vnf_id"`
	MultiplexedVnfGroups []MultiplexedVnfGroup    `gorm:"foreignKey:vnf_id"`
}

func (VirtualNetworkFunction) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
