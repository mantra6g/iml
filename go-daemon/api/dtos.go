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
	IPNet         string `json:"ip_net"`
	IfaceName     string `json:"iface_name"`
	ClusterCIDR   string `json:"cluster_cidr"`
	GatewayIP     string `json:"gateway_ip"`
	BridgeName    string `json:"bridge_name"`
}

type AppInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}

// ========== VNFs ===========

type VnfInstanceConfigRequest struct {
	VnfID       string `json:"vnf_id" validate:"required"`
	ContainerID string `json:"container_id" validate:"required"`
}

type VnfInstanceConfigResponse struct {
	IPNet       string `json:"ip_net"`
	SID         string `json:"sid"`
	IfaceName   string `json:"iface_name"`
	ClusterCIDR string `json:"cluster_cidr"`
	GatewayIP   string `json:"gateway_ip"`
	BridgeName  string `json:"bridge_name"`
}

type VnfInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

type NetworkServiceRegistrationRequest struct {
	ChainID  string   `json:"chain_id" validate:"required"`
	SrcAppID string   `json:"src_app_id" validate:"required"`
	DstAppID string   `json:"dst_app_id" validate:"required"`
	Vnfs     []string `json:"vnfs" validate:"required,dive,required"`
}
