package types

import (
	netutils "imlcni/utils/net"

	"github.com/containernetworking/cni/pkg/types"
)

type AppType string

const (
	P4TargetType    AppType = "p4target"
	ApplicationType AppType = "application"
)

type IMLCNIConfig struct {
	types.PluginConf
	Args struct {
		CNI struct {
			AppType      AppType `json:"app_type"`
			TargetName   string  `json:"target_name,omitempty"`
			AppName      string  `json:"app_name,omitempty"`
			AppNamespace string  `json:"app_namespace,omitempty"`
		} `json:"cni,omitempty"`
	} `json:"args,omitempty"`
}

type K8sArgs struct {
	types.CommonArgs
	PodName      types.UnmarshallableString
	PodNamespace types.UnmarshallableString
}

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
