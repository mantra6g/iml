package chains

type NetworkServiceRegistrationRequest struct {
	ChainID  string
	SrcAppID string
	DstAppID string
	Vnfs     []string
}
