package cni

import (
	netutils "iml-daemon/pkg/utils/net"
)

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

type NetworkConfig struct {
	IPNets       netutils.DualStackNetwork `json:"ip_nets"`
	ClusterCIDRs netutils.DualStackNetwork `json:"cluster_cidrs"`
	Gateways     netutils.DualStackAddress `json:"gateways"`
	IfaceName    string                    `json:"iface_name"`
	BridgeName   string                    `json:"bridge_name"`
	MTU          uint32                    `json:"mtu"`
}

// =========== Applications ===========

type AppInstanceConfigRequest struct {
	ContainerID  string `json:"container_id"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	AppName      string `json:"app_name"`
	AppNamespace string `json:"app_namespace"`
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
