package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Route struct {
	ID            uuid.UUID `gorm:"primaryKey"`
	ChainID       uuid.UUID
	SrcAppGroupID uuid.UUID
	DstAppGroupID uuid.UUID
	Stages        []RouteStage `gorm:"constraint:OnDelete:CASCADE,OnUpdate:CASCADE"`
}

func (r Route) GetStagedGroups(db *gorm.DB) ([]VnfGroup, error) {
	var stages []RouteStage
	if err := db.Where("route_id", r.ID).Find(&stages).Error; err != nil {
		return nil, err
	}

	out := make([]VnfGroup, 0, len(stages))
	for _, r := range stages {
		switch r.VnfGroupType {
		case "simple_vnf_groups":
			var b *SimpleVnfGroup
			if err := db.First(b, r.VnfGroupID).Error; err != nil {
				return nil, err
			}
			out = append(out, b)
		case "multiplexed_vnf_groups":
			var b *MultiplexedVnfGroup
			if err := db.First(b, r.VnfGroupID).Error; err != nil {
				return nil, err
			}
			out = append(out, b)
		}
	}

	return out, nil
}

func (Route) BeforeCreate(tx *gorm.DB) (err error) {
	randomID, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	tx.Statement.SetColumn("id", randomID)
	return
}
