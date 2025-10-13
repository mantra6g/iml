package env

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

const IML_ADDR  = "updates-service.desire6g-system.svc.cluster.local"
const API_PORT  = "1810"
const MQTT_PORT = "1816" 
const API_URL   = "http://" + IML_ADDR + ":" + API_PORT
const MQTT_URL  = "mqtt://" + IML_ADDR + ":" + MQTT_PORT

type GlobalConfig struct {
	AppSubnet  *net.IPNet
	NFSubnet   *net.IPNet
	NFRouterAppIP string
	NFRouterVNFIP string
	NodeID		    string
}

// Singleton instance of GlobalConfig
var globalConfig *GlobalConfig

func (e *GlobalConfig) getSubnetFromIML() error {
	resp, err := http.Get(IML_ADDR + "/api/v1/nodemanager/subnet")
	if err != nil {
		return fmt.Errorf("failed contacting IML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("IML returned status code %d", resp.StatusCode)
	}

	var subnetResponse struct {
		AppSubnet net.IPNet `json:"app_subnet"`
		NFSubnet  net.IPNet `json:"nf_subnet"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&subnetResponse); err != nil {
		return fmt.Errorf("failed to decode IML response: %w", err)
	}

	e.AppSubnet = &subnetResponse.AppSubnet
	e.NFSubnet = &subnetResponse.NFSubnet
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

	return env, nil
}

func Config() (*GlobalConfig, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}
	return nil, fmt.Errorf("global config not initialized")
}