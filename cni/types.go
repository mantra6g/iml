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
			SrcInterface string `json:"src_if,omitempty"`
			DstInterface string `json:"dst_if,omitempty"`
		} `json:"cni,omitempty"`
	} `json:"args,omitempty"`
}

type AppConfigRequest struct {
	ApplicationID string `json:"application_id"`
	HostID        string `json:"host_id"`
}

type AppConfigResponse struct {
	IP          string `json:"ip"`
	MacAddress  string `json:"mac_address"`
	PeerName    string `json:"peer_name"`
	Route       struct {
		Destination string `json:"destination"`
		GatewayIP   string `json:"gateway_ip"`
		GatewayMac  string `json:"gateway_mac"`
	} `json:"route"`
}

type NFConfigRequest struct {
	NFID string `json:"nf_id"`
}

type NFConfigResponse struct {
	Interfaces []struct {
		Name        string `json:"name"`
		MacAddress  string `json:"mac_address"`
	} `json:"interfaces"`
}

type AppTeardownRequest struct {
	ApplicationID string `json:"application_id"`
}
