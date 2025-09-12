package iml

import (
	"encoding/json"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/models"
	"iml-daemon/mqtt"
	"iml-daemon/services/eventbus"
	"net/http"
)

type Client struct {
	eb   *eventbus.EventBus
	mqtt *mqtt.Client
}

func NewClient(eb *eventbus.EventBus, mqttClient *mqtt.Client) (*Client, error) {
	return &Client{
		eb:   eb,
		mqtt: mqttClient,
	}, nil
}

// GetApp retrieves the application details from IML and subscribes to its updates via MQTT.
//
// It returns the application details or an error if the application is not found or if there was an issue during the process.
// It also subscribes to updates for the application and its services, maintaining real-time synchronization with IML.
//
// This method is the correct way to pull application details locally. Use it when there is no local copy of the application.
func (c *Client) GetApp(id string) (*models.Application, error) {
	// First, check the status of the application
	resp, err := http.Get(env.API_URL + "/api/v1/applications/" + id)
	if err != nil {
		return nil, fmt.Errorf("failed to contact IML: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("application %s not found in IML", id)
	}
	defer resp.Body.Close()

	var appStatus applicationStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&appStatus); err != nil {
		return nil, fmt.Errorf("failed to decode application response: %v", err)
	}

	err = c.mqtt.SubscribeToAppUpdates(id, func(app mqtt.ApplicationDefinition) {
		// TODO: Handle application update
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to app updates: %v", err)
	}

	err = c.mqtt.SubscribeToAppServices(id, func(services mqtt.ApplicationServiceChains) {
		// TODO: Handle application services update
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to app services updates: %v", err)
	}

	// TODO: Fix to return something
	return nil, nil
}

// GetNF retrieves the network function details from IML and subscribes to its updates via MQTT.
//
// It returns the network function details or an error if the network function is not found or if there was an issue during the process.
// It also subscribes to updates for the network function and its services, maintaining real-time synchronization with IML.
//
// This method is the correct way to pull network function details locally. Use it when there is no local copy of the network function.
func (c *Client) GetNF(id string) (*models.VirtualNetworkFunction, error) {
	resp, err := http.Get(env.API_URL + "/api/v1/nfs/" + id)
	if err != nil {
		return nil, fmt.Errorf("failed to contact IML: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("network function %s not found in IML", id)
	}
	defer resp.Body.Close()

	var vnfStatus nfStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&vnfStatus); err != nil {
		return nil, fmt.Errorf("failed to decode VNF response: %v", err)
	}

	err = c.mqtt.SubscribeToVNFUpdates(id, func(nf mqtt.NetworkFunctionDefinition) {
		// TODO: Handle VNF update
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to VNF updates: %v", err)
	}

	// TODO: Fix to return something
	return nil, nil
}

// GetServiceChain retrieves the service chain details from IML and subscribes to its updates via MQTT.
//
// It returns the service chain details or an error if the service chain is not found or if there was an issue during the process.
// It also subscribes to updates for the service chain and its services, maintaining real-time synchronization with IML.
//
// This method is the correct way to pull service chain details locally. Use it when there is no local copy of the service chain.
func (c *Client) GetServiceChain(id string) (*models.ServiceChain, error) {
	resp, err := http.Get(env.API_URL + "/api/v1/chains/" + id)
	if err != nil {
		return nil, fmt.Errorf("failed to contact IML: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service chain %s not found in IML", id)
	}
	defer resp.Body.Close()

	var scStatus scStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&scStatus); err != nil {
		return nil, fmt.Errorf("failed to decode service chain response: %v", err)
	}

	err = c.mqtt.SubscribeToServiceChainUpdates(id, func(sc mqtt.ServiceChainDefinition) {
		// TODO: Handle service chain update
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to service chain updates: %v", err)
	}

	// TODO: Fix to return something
	return nil, nil
}
