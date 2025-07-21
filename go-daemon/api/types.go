package api

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

// =========== Applications ===========

type AppConfigRequest struct {
	ApplicationID string `json:"application_id"`
	ContainerID   string `json:"container_id"`
}

type AppConfigResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type AppTeardownRequest struct {
	ApplicationID string `json:"application_id"`
}

type AppTeardownResponse struct {
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

type VnfConfigRequest struct {
	ApplicationID string `json:"application_id"`
}

type VnfConfigResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type VnfTeardownRequest struct {
	ApplicationID string `json:"application_id"`
}

type VnfTeardownResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}
