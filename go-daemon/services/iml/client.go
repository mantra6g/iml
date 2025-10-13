// The IML package provides a client to interact with the IML (Infrastructure Management Layer) service.
//
// It allows pulling application, network function, and service chain details from IML,
// and it keeps them synchronized via MQTT subscriptions. It works as a kind of dependency manager
// as every object pulled from IML also has its own dependencies pulled and kept in sync.
// When an object is no longer needed, it handles unsubscribing from its updates and its dependencies.
package iml

import (
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/models"
	"iml-daemon/mqtt"
	"iml-daemon/services/events"
	"iml-daemon/services/iml/subscriptions"
	"net/http"
)

type Client struct {
	eb   *events.EventBus
	mqtt *mqtt.Client
	repo *db.Registry

	manager *subscriptions.SubscriptionManager
}

func NewClient(eb *events.EventBus, manager *subscriptions.SubscriptionManager) (*Client, error) {
	if eb == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if manager == nil {
		return nil, fmt.Errorf("subscription manager is required")
	}

	client := &Client{
		eb:      eb,
		manager: manager,
	}

	client.eb.Subscribe(events.EventAppGroupCreated, client.handleLocalAppGroupCreated)
	client.eb.Subscribe(events.EventAppGroupRemoved, client.handleLocalAppGroupRemoved)
	client.eb.Subscribe(events.EventVnfGroupCreated, client.handleLocalVnfGroupCreated)
	client.eb.Subscribe(events.EventVnfGroupRemoved, client.handleLocalVnfGroupRemoved)
	return client, nil
}

// GetApplication retrieves the application details from IML.
//
// It returns the application details or an error if the application is not found or if there was an issue during the process.
// IMPORTANT: This method only gets the current state of the application. It does NOT subscribe to updates for the application.
// If you want to keep the application updated, use PullApplication instead.
//
// This method is the correct way to check the state of an application.
func (c *Client) GetApplication(id string) (*models.Application, error) {
	// First, check if the application already exists locally
	// If the application exists, and it is active, then it means that
	// it is already synchronized with IML via MQTT, so we can return it directly.
	// If the application does not exist locally or is not active, then we need to
	// pull it from IML again to check if a new one with the same ID has been created.
	app, err := c.repo.FindActiveAppByGlobalID(id)
	if err == nil {
		// Application already exists locally and is active
		return app, nil
	}

	// If not, then retrieve it from IML
	resp, err := http.Get(env.API_URL + "/api/v1/applications/" + id)
	if err != nil {
		return nil, fmt.Errorf("failed to contact IML: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("application %s not found in IML", id)
	}
	defer resp.Body.Close()

	var appDefinition mqtt.ApplicationDefinition
	if err := json.NewDecoder(resp.Body).Decode(&appDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode application response: %v", err)
	}
	if appDefinition.Status != "active" {
		return nil, fmt.Errorf("application %s is not active in IML", id)
	}

	// If it is active, save its current state locally and temporarily subscribe to its updates.
	app = &models.Application{
		GlobalID: appDefinition.ID,
		Status:   models.AppStatusActive,
		Etag:     appDefinition.Seq,
	}
	if err := c.repo.SaveApp(app); err != nil {
		return nil, fmt.Errorf("failed to save application %s: %v", id, err)
	}

	err = c.manager.AddDependency(&subscriptions.TemporaryAppDependency{AppID: app.GlobalID})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to app updates: %v", err)
	}

	return app, nil
}

func (c *Client) GetNetworkFunction(id string) (*models.VirtualNetworkFunction, error) {
	// First, check if the network function already exists locally
	// If the network function exists, and it is active, then it means that
	// it is already synchronized with IML via MQTT, so we can return it directly.
	// If the network function does not exist locally or is not active, then we need to
	// pull it from IML again to check if a new one with the same ID has been created.
	nf, err := c.repo.FindActiveNetworkFunctionByGlobalID(id)
	if err == nil {
		// Network function already exists locally and is active
		return nf, nil
	}

	// If not, then retrieve it from IML
	resp, err := http.Get(env.API_URL + "/api/v1/nfs/" + id)
	if err != nil {
		return nil, fmt.Errorf("failed to contact IML: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("network function %s not found in IML", id)
	}
	defer resp.Body.Close()

	var nfDefinition mqtt.NetworkFunctionDefinition
	if err := json.NewDecoder(resp.Body).Decode(&nfDefinition); err != nil {
		return nil, fmt.Errorf("failed to decode VNF response: %v", err)
	}
	if nfDefinition.Status != "active" {
		return nil, fmt.Errorf("network function %s is not active in IML", id)
	}

	// If it is active, save its current state locally and temporarily subscribe to its updates.
	nf = &models.VirtualNetworkFunction{
		GlobalID: nfDefinition.ID,
		Status:   models.VNFStatusActive,
	}
	if err := c.repo.SaveVnf(nf); err != nil {
		return nil, fmt.Errorf("failed to save network function %s: %v", id, err)
	}

	err = c.manager.AddDependency(&subscriptions.TemporaryVnfDependency{VnfID: nf.GlobalID})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to VNF updates: %v", err)
	}

	return nf, nil
}

// // PullApplication retrieves the application details from IML and subscribes to its updates via MQTT.
// //
// // It returns the application details or an error if the application is not found or if there was an issue during the process.
// // It also subscribes to updates for the application and its services, maintaining real-time synchronization with IML.
// //
// // This method is the correct way to pull application details locally. Use it when there is no local copy of the application.
// func (c *Client) PullApplication(id string) (*models.Application, error) {
// 	// First, check the status of the application
// 	resp, err := http.Get(env.API_URL + "/api/v1/applications/" + id)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to contact IML: %v", err)
// 	}
// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("application %s not found in IML", id)
// 	}
// 	defer resp.Body.Close()

// 	var appDefinition mqtt.ApplicationDefinition
// 	if err := json.NewDecoder(resp.Body).Decode(&appDefinition); err != nil {
// 		return nil, fmt.Errorf("failed to decode application response: %v", err)
// 	}

// 	updateLocalApp := true
// 	localApp, err := c.repo.FindActiveAppByGlobalID(id)
// 	if localApp != nil && localApp.Etag >= appDefinition.Seq {
// 		// The case of the local app being more recent than the remote one
// 		// should never happen, but if it does, we just skip the update
// 		updateLocalApp = false
// 	}

// 	if updateLocalApp {
// 		localApp.Etag = appDefinition.Seq
// 		switch appDefinition.Status {
// 		case "active":
// 			localApp.Status = models.AppStatusActive
// 		case "deleted":
// 			localApp.Status = models.AppStatusDeletionPending
// 		default:
// 			return nil, fmt.Errorf("application %s has unknown status %s in IML", id, appDefinition.Status)
// 		}
// 		if err := c.repo.SaveApp(localApp); err != nil {
// 			return nil, fmt.Errorf("failed to update application %s: %v", id, err)
// 		}
// 	}

// 	err = c.mqtt.SubscribeToAppUpdates(id, c.handleAppUpdate)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to subscribe to app updates: %v", err)
// 	}

// 	err = c.mqtt.SubscribeToAppServices(id, c.handleAppServicesUpdates)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to subscribe to app services updates: %v", err)
// 	}

// 	return localApp, nil
// }

// // PullNetworkFunction retrieves the network function details from IML and subscribes to its updates via MQTT.
// //
// // It returns the network function details or an error if the network function is not found or if there was an issue during the process.
// // It also subscribes to updates for the network function and its services, maintaining real-time synchronization with IML.
// //
// // This method is the correct way to pull network function details locally. Use it when there is no local copy of the network function.
// func (c *Client) PullNetworkFunction(id string) (*models.VirtualNetworkFunction, error) {
// 	resp, err := http.Get(env.API_URL + "/api/v1/nfs/" + id)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to contact IML: %v", err)
// 	}
// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("network function %s not found in IML", id)
// 	}
// 	defer resp.Body.Close()

// 	var nfDefinition mqtt.NetworkFunctionDefinition
// 	if err := json.NewDecoder(resp.Body).Decode(&nfDefinition); err != nil {
// 		return nil, fmt.Errorf("failed to decode VNF response: %v", err)
// 	}
// 	if nfDefinition.Status != "active" {
// 		return nil, fmt.Errorf("network function %s is not active in IML", id)
// 	}

// 	nf, err := c.repo.FindActiveNetworkFunctionByGlobalID(id)
// 	if err == nil {
// 		// Network function already exists locally and is active
// 		return nf, nil
// 	}

// 	nf = &models.VirtualNetworkFunction{
// 		GlobalID: nfDefinition.ID,
// 		Status:   models.VNFStatusActive,
// 	}
// 	if err := c.repo.SaveVnf(nf); err != nil {
// 		return nil, fmt.Errorf("failed to save network function %s: %v", id, err)
// 	}

// 	err = c.mqtt.SubscribeToVNFUpdates(id, c.handleNfUpdates)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to subscribe to VNF updates: %v", err)
// 	}

// 	return nf, nil
// }

// // PullServiceChain retrieves the service chain details from IML and subscribes to its updates via MQTT.
// //
// // It returns the service chain details or an error if the service chain is not found or if there was an issue during the process.
// // It also subscribes to updates for the service chain and its services, maintaining real-time synchronization with IML.
// //// Handle VNF group creation event
// // This method is the correct way to pull service chain details locally. Use it when there is no local copy of the service chain.
// func (c *Client) PullServiceChain(id string) (*models.ServiceChain, error) {
// 	resp, err := http.Get(env.API_URL + "/api/v1/chains/" + id)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to contact IML: %v", err)
// 	}
// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("service chain %s not found in IML", id)
// 	}
// 	defer resp.Body.Close()

// 	var scDefinition mqtt.ServiceChainDefinition
// 	if err := json.NewDecoder(resp.Body).Decode(&scDefinition); err != nil {
// 		return nil, fmt.Errorf("failed to decode service chain response: %v", err)
// 	}
// 	if scDefinition.Status != "active" {
// 		return nil, fmt.Errorf("service chain %s is not active in IML", id)
// 	}

// 	chain, err := c.repo.FindActiveNetworkServiceChainByGlobalID(id)
// 	if err == nil {
// 		// Service chain already exists locally and is active
// 		return chain, nil
// 	}

// 	// Ensure destination application exists
// 	err = c.pullRemoteGroupsOfApp(scDefinition.DstAppID)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to pull destination application %s: %v", scDefinition.DstAppID, err)
// 	}

// 	// Ensure vnfs exist
// 	for _, vnfUID := range scDefinition.Functions {
// 		err = c.pullRemoteGroupsOfVnf(vnfUID)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to pull VNF %s: %v", vnfUID, err)
// 		}
// 	}

// 	err = c.mqtt.SubscribeToServiceChainUpdates(id, c.handleServiceChainUpdates)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to subscribe to service chain updates: %v", err)
// 	}

// 	// TODO: Fix to return something
// 	return nil, nil
// }
