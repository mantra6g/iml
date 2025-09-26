package iml

import (
	"iml-daemon/logger"
	"iml-daemon/mqtt"
)

func (c *Client) handleAppUpdate(app mqtt.ApplicationDefinition) {
	// Find the application in the local database
	localApp, err := c.repo.FindActiveAppByGlobalID(appID)
	if err != nil {
		logger.DebugLogger().Printf("iml.handleAppUpdate: application %s not found in local database: %v", appID, err)
		return
	}

	// Extract application details
	appID := app.ID
	appStatus := app.Status

	// Find the application in the local database
	localApp, err = c.repo.FindActiveAppByGlobalID(appID)
	if err != nil {
		logger.DebugLogger().Printf("iml.handleAppUpdate: application %s not found in local database: %v", appID, err)
		return
	}

	// Update the application in the local database
	c.repo.SaveApp()
}

func (c *Client) handleAppServicesUpdates(svc mqtt.ApplicationServiceChains) {

}

func (c *Client) handleNfUpdates(nf mqtt.NetworkFunctionDefinition) {

}

func (c *Client) handleServiceChainUpdates(sc mqtt.ServiceChainDefinition) {
	
}