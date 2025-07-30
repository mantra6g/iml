package models

type NetworkServiceChain struct {
	ID			   uint64 `gorm:"primaryKey"`
	IMLChainID string `gorm:"uniqueIndex:iml_chain_id"`
	SrcAppID   string
	SrcApp     Application `gorm:"foreignKey:src_app_id"`
	DstAppID   string
	DstApp     Application    `gorm:"foreignKey:dst_app_id"`
	Elements   []ChainElement `gorm:"foreignKey:network_service_chain_id"`
}
