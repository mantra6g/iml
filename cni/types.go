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
	IP       string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName string `json:"peer_name"`
	Route    struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type NFConfigRequest struct {
	VnfID       string `json:"vnf_id"`
	ContainerID string `json:"container_id"`
}

type NFConfigResponse struct {
	SID      string `json:"sid"`
	MacAddress  string `json:"mac_address"`
	PeerName string `json:"peer_name"`
	Route    struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type AppTeardownRequest struct {
	ContainerID   string `json:"container_id"`
}

type NfTeardownRequest struct {
	ContainerID   string `json:"container_id"`
}