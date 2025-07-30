package models

type ChainElement struct {
	ChainID   string `gorm:"primaryKey"`
	Position  uint8  `gorm:"primaryKey;autoIncrement:false;index:,sort:asc"`
	VnfID     string
	Vnf       VirtualNetworkFunction `gorm:"foreignKey:vnf_id"`
}