package env

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

const IML_ADDR = "http://iml-nfvo.desire6g-system.svc.cluster.local:5000"

type GlobalConfig struct {
	AppSubnet  *net.IPNet
	NFSubnet   *net.IPNet
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

	err := env.getSubnetFromIML()
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