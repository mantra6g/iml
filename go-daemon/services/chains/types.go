package chains

type NetworkServiceRegistrationRequest struct {
	ChainID string `json:"chain_id" validate:"required"`
}
