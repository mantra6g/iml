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
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/mqtt"
	"iml-daemon/services/events"
	"iml-daemon/services/iml/subscriptions"
	"net/http"
)

type Client struct {
	eb   *events.EventBus
	repo *db.Registry

	manager *subscriptions.SubscriptionManager
}

func NewClient(eb *events.EventBus, repo *db.Registry, manager *subscriptions.SubscriptionManager) (*Client, error) {
	if eb == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if manager == nil {
		return nil, fmt.Errorf("subscription manager is required")
	}

	client := &Client{
		eb:      eb,
		repo:    repo,
		manager: manager,
	}

	client.eb.Subscribe(events.EventLocalAppGroupCreated, client.handleLocalAppGroupCreated)
	client.eb.Subscribe(events.EventLocalAppGroupRemoved, client.handleLocalAppGroupRemoved)
	client.eb.Subscribe(events.EventLocalSimpleVnfGroupCreated, client.handleLocalVnfGroupCreated)
	client.eb.Subscribe(events.EventLocalVnfMultiplexedGroupCreated, client.handleLocalVnfMultiplexedGroupCreated)
	client.eb.Subscribe(events.EventLocalSimpleVnfGroupRemoved, client.handleLocalVnfGroupRemoved)
	client.eb.Subscribe(events.EventLocalVnfMultiplexedGroupRemoved, client.handleLocalVnfMultiplexedGroupRemoved)
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
	resp, err := http.Get(env.API_URL + "/api/v1/apps/" + id)
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
	logger.DebugLogger().Printf("Retrieved network function definition from IML: %+v", nfDefinition)
	if nfDefinition.Status != "active" {
		return nil, fmt.Errorf("network function %s is not active in IML", id)
	}

	// If it is active, save its current state locally and temporarily subscribe to its updates.
	nf = &models.VirtualNetworkFunction{
		GlobalID: nfDefinition.ID,
		Status:   models.VNFStatusActive,
		Type:     models.NetworkFunctionType(nfDefinition.Type),
	}
	if err := c.repo.SaveVnf(nf); err != nil {
		return nil, fmt.Errorf("failed to save network function %s: %v", id, err)
	}
	logger.DebugLogger().Printf("Saved network function: %+v", nf)

	if nf.Type == models.NetworkFunctionTypeMultiplexed {
		subfunctions := []models.Subfunction{}
		for _, sfDef := range nfDefinition.SubFunctions {
			sf := models.Subfunction{
				SubfunctionID: sfDef.ID,
				VnfID:         nf.ID,
			}
			subfunctions = append(subfunctions, sf)
		}
		if err := c.repo.SaveSubfunctions(subfunctions); err != nil {
			return nil, fmt.Errorf("failed to save subfunctions for network function %s: %v", id, err)
		}
		logger.DebugLogger().Printf("Saved subfunctions for network function %s", id)
	}

	err = c.manager.AddDependency(&subscriptions.TemporaryVnfDependency{VnfID: nf.GlobalID})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to VNF updates: %v", err)
	}

	// Retrieve the saved network function to include all related data
	nf, err = c.repo.FindCompleteActiveNetworkFunctionByGlobalID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve saved network function %s: %v", id, err)
	}

	return nf, nil
}
