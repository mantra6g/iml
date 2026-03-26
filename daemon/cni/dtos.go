package cni

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

// =========== Applications ===========

type AppInstanceConfigRequest struct {
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	AppName      string `json:"app_name"`
	AppNamespace string `json:"app_namespace"`
}

type PodNetworkConfig struct {
	IPNets       []string `json:"ip_nets"`
	ClusterCIDRs []string `json:"cluster_cidrs"`
	Gateways     []string `json:"gateways"`
	IfaceName    string   `json:"iface_name"`
	BridgeName   string   `json:"bridge_name"`
}

type AppInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}

// ========== VNFs ===========

type P4TargetConfigRequest struct {
	P4TargetName string `json:"p4target_name" validate:"required"`
}

type VnfInstanceConfigRequest struct {
	VnfID       string `json:"vnf_id" validate:"required"`
	ContainerID string `json:"container_id" validate:"required"`
}

type VnfInstanceConfigResponse struct {
	IPNet       string   `json:"ip_net"`
	SIDs        []string `json:"sids"`
	IfaceName   string   `json:"iface_name"`
	ClusterCIDR string   `json:"cluster_cidr"`
	GatewayIP   string   `json:"gateway_ip"`
	BridgeName  string   `json:"bridge_name"`
}

type VnfInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}
