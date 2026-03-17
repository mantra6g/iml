package env

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

const P4_CONTROLLER_ADDR = "iml-p4-controller.loom-system.svc.cluster.local"
const IML_ADDR = "iml-updates-service.loom-system.svc.cluster.local"
const API_PORT = "1810"
const MQTT_PORT = "1816"
const API_URL = "http://" + IML_ADDR + ":" + API_PORT
const MQTT_URL = "mqtt://" + IML_ADDR + ":" + MQTT_PORT
const P4_CONTROLLER_API_URL = "http://" + P4_CONTROLLER_ADDR

type GlobalConfig struct {
	ClusterCIDR *net.IPNet
	AppSubnet   *net.IPNet
	NFSubnet    *net.IPNet
	SIDSubnet   *net.IPNet
	TunSubnet   *net.IPNet
	DecapSID    *net.IPNet
	NodeID      string
}

type controllerResponse struct {
	ClusterCIDR net.IPNet `json:"cluster_cidr"`
	AppSubnet   net.IPNet `json:"app_subnet"`
	NFSubnet    net.IPNet `json:"nf_subnet"`
	SIDSubnet   net.IPNet `json:"sid_subnet"`
	TunSubnet   net.IPNet `json:"tun_subnet"`
}

// Singleton instance of GlobalConfig
var globalConfig *GlobalConfig

func (e *GlobalConfig) getSubnetFromIML() error {
	resp, err := http.Get(API_URL + "/api/v1/nodemanager/subnet")
	if err != nil {
		return fmt.Errorf("failed contacting IML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IML returned status code %d", resp.StatusCode)
	}

	var subnetResponse controllerResponse
	if err := json.NewDecoder(resp.Body).Decode(&subnetResponse); err != nil {
		return fmt.Errorf("failed to decode IML response: %w", err)
	}

	e.AppSubnet = &subnetResponse.AppSubnet
	e.NFSubnet = &subnetResponse.NFSubnet
	e.SIDSubnet = &subnetResponse.SIDSubnet
	e.TunSubnet = &subnetResponse.TunSubnet
	e.ClusterCIDR = &subnetResponse.ClusterCIDR
	return nil
}

// RequestConfigFromIML requests the NodeManager subnet from IML
func RequestConfigFromIML() (*GlobalConfig, error) {
	env := &GlobalConfig{}

	nodeID, err := K8sGetNodeID()
	if err != nil {
		return nil, fmt.Errorf("error getting node ID: %w", err)
	}
	env.NodeID = nodeID

	err = env.getSubnetFromIML()
	if err != nil {
		return nil, fmt.Errorf("error getting subnet from IML: %w", err)
	}

	globalConfig = env
	return env, nil
}

func Config() (*GlobalConfig, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}
	return nil, fmt.Errorf("global config not initialized")
}
