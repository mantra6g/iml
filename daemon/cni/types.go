package cni

import (
	netutils "iml-daemon/pkg/utils/net"
)

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

// =========== Applications ===========

type AppInstanceConfigRequest struct {
	ContainerID  string `json:"container_id"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	AppName      string `json:"app_name"`
	AppNamespace string `json:"app_namespace"`
}

type NetworkConfig struct {
	IPNets       netutils.DualStackNetwork `json:"ip_nets"`
	ClusterCIDRs netutils.DualStackNetwork `json:"cluster_cidrs"`
	Gateways     netutils.DualStackAddress `json:"gateways"`
	IfaceName    string                    `json:"iface_name"`
	BridgeName   string                    `json:"bridge_name"`
	MTU          uint32                    `json:"mtu"`
}

type AppInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}

// ========== P4Targets ===========

type ContainerizedP4TargetConfigRequest struct {
	ContainerID  string `json:"container_id" validate:"required"`
	P4TargetName string `json:"p4target_name" validate:"required"`
}

type ContainerizedP4TargetTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
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
