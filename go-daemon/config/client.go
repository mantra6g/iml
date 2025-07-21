package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"iml-daemon/helpers"
)

const IML_ADDR = "http://iml-nfvo.desire6g-system.svc.cluster.local:5000"

type Environment struct {
	AppSubnet net.IPNet
	NFSubnet	net.IPNet
	AppSIDAllocator *helpers.IPAllocator
	NFSIDAllocator *helpers.IPAllocator
}

func (e *Environment) getSubnetFromIML() (error) {
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
		NFSubnet net.IPNet `json:"nf_subnet"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&subnetResponse); err != nil {
		return fmt.Errorf("failed to decode IML response: %w", err)
	}

	// Create IP allocators for the subnets
	appSidAllocator, err := helpers.NewIPAllocator(subnetResponse.AppSubnet)
	if err != nil {
		return fmt.Errorf("failed to create IP allocator for app subnet: %w", err)
	}
	e.AppSubnet = subnetResponse.AppSubnet
	e.AppSIDAllocator = appSidAllocator

	nfSidAllocator, err := helpers.NewIPAllocator(subnetResponse.NFSubnet)
	if err != nil {
		return fmt.Errorf("failed to create IP allocator for NF subnet: %w", err)
	}
	e.NFSubnet = subnetResponse.NFSubnet
	e.NFSIDAllocator = nfSidAllocator

	return nil
}

// RequestConfigFromIML requests the NodeManager subnet from IML
func RequestConfigFromIML() (*Environment, error) {
	env := &Environment{}

	err := env.getSubnetFromIML()
	if err != nil {
		return nil, fmt.Errorf("error getting subnet from IML: %w", err)
	}

	return env, nil
}

func (e *Environment) GetNewAppIP() (*net.IPNet, error) {
	ip, err := e.AppSIDAllocator.Next()
	if err != nil {
		return nil, fmt.Errorf("error getting new app IP: %w", err)
	}
	return ip, nil
}

func (e *Environment) GetNewNFIP() (*net.IPNet, error) {
	ip, err := e.NFSIDAllocator.Next()
	if err != nil {
		return nil, fmt.Errorf("error getting new nf IP: %w", err)
	}
	return ip, nil
}

