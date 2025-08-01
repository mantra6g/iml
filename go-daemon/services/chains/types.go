package chains

type NetworkServiceRegistrationRequest struct {
	ChainID  string   `json:"chain_id" validate:"required,mongodb"`
	SrcAppID string   `json:"src_app_id" validate:"required,mongodb"`
	DstAppID string   `json:"dst_app_id" validate:"required,mongodb"`
	Vnfs     []string `json:"vnfs" validate:"required,dive,required,mongodb"`
}
