package cni

import (
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"
)

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

type NetworkConfig struct {
	IPNets       netutils.DualStackNetwork `json:"ip_nets"`
	ClusterCIDRs netutils.DualStackNetwork `json:"cluster_cidrs"`
	ServiceCIDRs netutils.DualStackNetwork `json:"service_cidrs"`
	Gateways     netutils.DualStackAddress `json:"gateways"`
	BridgeVethName  string                 `json:"bridge_veth_name"`
	OtherVethName   string                 `json:"other_veth_name"`
	MultiInterfaceNames []string           `json:"multi_interface_names"`
	CreateInterfaces bool                  `json:"create_interfaces"`
	EnableSRv6Proxy bool                   `json:"enable_srv6_proxy"`
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

type P4TargetRegistrationCallbackRequest struct {
	P4TargetName string `json:"p4target_name" validate:"required"`
}