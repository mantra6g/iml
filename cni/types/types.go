package types

import (
	netutils "github.com/mantra6g/iml/cni/utils/net"

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
			NFInterfaces uint8   `json:"nf_interfaces,omitempty"`
		} `json:"cni,omitempty"`
	} `json:"args,omitempty"`
}

type K8sArgs struct {
	types.CommonArgs
	K8S_POD_NAME      types.UnmarshallableString
	K8S_POD_NAMESPACE types.UnmarshallableString
}

/**************************************************************
*********************** CNI Requests **************************
**************************************************************/

type NetworkConfig struct {
	IPNets       netutils.DualStackNetwork `json:"ip_nets"`
	ClusterCIDRs netutils.DualStackNetwork `json:"cluster_cidrs"`
	ServiceCIDRs netutils.DualStackNetwork `json:"service_cidrs"`
	Gateways     netutils.DualStackAddress `json:"gateways"`
	// BridgeName is the bridge an application's host-side veth is attached to. P4Targets are
	// attached by the daemon instead.
	BridgeName string `json:"bridge_name"`
	// BridgeVethName is the host-side peer of the container's iml0 interface.
	BridgeVethName string `json:"bridge_veth_name"`
	// MultiInterfaceNames are the host-side peers of a multi-interface P4Target's nfX interfaces.
	// When set, no iml0 interface is created.
	MultiInterfaceNames []string `json:"multi_interface_names"`
	MTU                 uint32   `json:"mtu"`
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

// P4TargetRegistrationCallbackRequest tells the daemon the target's interfaces exist, so it can
// wire their host side.
type P4TargetRegistrationCallbackRequest struct {
	ContainerID  string `json:"container_id" validate:"required"`
	P4TargetName string `json:"p4target_name" validate:"required"`
}
