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
	// BridgeName is the bridge the application's host-side veth is attached to.
	BridgeName string `json:"bridge_name"`
	// BridgeVethName is the host-side peer of the container's iml0 interface.
	BridgeVethName string `json:"bridge_veth_name"`
	MTU            uint32 `json:"mtu"`
}

// P4TargetNetworkConfig names the interfaces the CNI plugin creates for a P4Target, nf0, nf1, ...
// The daemon wires their host side.
type P4TargetNetworkConfig struct {
	// HostInterfaceNames are the host-side peers of the target's nfX interfaces.
	HostInterfaceNames []string `json:"host_interface_names"`
	MTU                uint32   `json:"mtu"`
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
	ContainerID  string `json:"container_id" validate:"required"`
	P4TargetName string `json:"p4target_name" validate:"required"`
}

// P4TargetRegistrationCallbackRequest is sent by the CNI plugin once it has created the target's
// interfaces, so the daemon can wire their host side.
type P4TargetRegistrationCallbackRequest struct {
	ContainerID  string `json:"container_id" validate:"required"`
	P4TargetName string `json:"p4target_name" validate:"required"`
}
