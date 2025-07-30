package api

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

// =========== Applications ===========

type AppInstanceConfigRequest struct {
	ApplicationID string `json:"application_id" validate:"required"`
	ContainerID   string `json:"container_id" validate:"required"`
}

type AppInstanceConfigResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type AppInstanceTeardownRequest struct {
	ContainerID   string `json:"container_id" validate:"required"`
}

type AppInstanceTeardownResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

// ========== VNFs ===========

type VnfInstanceConfigRequest struct {
	VnfID       string `json:"vnf_id" validate:"required"`
	ContainerID string `json:"container_id" validate:"required"`
}

type VnfInstanceConfigResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type VnfInstanceTeardownRequest struct {
	ContainerID   string `json:"container_id" validate:"required"`
}

type VnfInstanceTeardownResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

type NetworkServiceRegistrationRequest struct {
	ChainID string `json:"chain_id" validate:"required"`
}