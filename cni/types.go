package main

import (
	"github.com/containernetworking/cni/pkg/types"
)

type IMLCNIConfig struct {
  types.PluginConf
  AppId string `json:"app_id"`
  AppType string `json:"app_type"`// network function or application function
}

type IMLConfigRequest struct {
  ApplicationID string `json:"application_id"`
  HostID string `json:"host_id"`
}

type IMLConfigResponse struct {
  IP string `json:"ip"`
  MacAddress string `json:"mac_address"`
  GatewayMac string `json:"gateway_mac"`
  Route struct {
    Destination string `json:"destination"`
    GatewayIP string `json:"gateway_ip"`
    GatewayMac string `json:"gateway_mac"`
  } `json:"routes"`
}