package models

type RouteSegment struct {
	RouteID    uint64 `gorm:"primaryKey"`
	VnfGroupID uint64 `gorm:"primaryKey"`

	Position   uint8  // Position in the route

	Route      Route     `gorm:"foreignKey:route_id"`
	VnfGroup   VnfGroup  `gorm:"foreignKey:vnf_group_id"`
}
