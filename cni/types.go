package main

import (
	"github.com/containernetworking/cni/pkg/types"
)

type IMLCNIConfig struct {
	types.PluginConf
	Args struct {
		CNI struct {
			AppId   string `json:"app_id,omitempty"`
			AppType string `json:"app_type"` // network function or application function
			NfID    string `json:"nf_id,omitempty"`
		} `json:"cni,omitempty"`
	} `json:"args,omitempty"`
}

type AppConfigRequest struct {
	ApplicationID string `json:"application_id"`
	ContainerID   string `json:"container_id"`
}

type AppConfigResponse struct {
	IPNet         string `json:"ip_net"`
	IfaceName     string `json:"iface_name"`
	ClusterCIDR   string `json:"cluster_cidr"`
	GatewayIP     string `json:"gateway_ip"`
	BridgeName    string `json:"bridge_name"`
}

type NFConfigRequest struct {
	VnfID       string `json:"vnf_id"`
	ContainerID string `json:"container_id"`
}

type NFConfigResponse struct {
	IPNet 		  string `json:"ip_net"`
	SID         string `json:"sid"`
	IfaceName   string `json:"iface_name"`
	ClusterCIDR string `json:"cluster_cidr"`
	GatewayIP   string `json:"gateway_ip"`
	BridgeName  string `json:"bridge_name"`
}

type AppTeardownRequest struct {
	ContainerID   string `json:"container_id"`
}

type NfTeardownRequest struct {
	ContainerID   string `json:"container_id"`
}