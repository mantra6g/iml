package models

import "gorm.io/gorm"

type Route struct {
	ID       uint64 `gorm:"primaryKey"`
	ChainID  string
	SrcID    uint64
	DstID    uint64
	Segments []VnfGroup `gorm:"many2many:route_segments"`
}

func (r *Route) AfterSave(tx *gorm.DB) (err error) {
	// TODO: Create the corresponding dataplane route
	return nil
}