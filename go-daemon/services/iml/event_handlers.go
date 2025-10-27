package iml

import (
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml/subscriptions"
)

func (c *Client) handleLocalAppGroupCreated(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalAppGroupCreated: %v", event)
	appGroup, ok := event.Payload.(models.AppGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupCreated: error casting event payload to LocalAppGroup")
		return
	}

	err := c.manager.AddDependency(&subscriptions.LocalAppDependency{
		AppID: appGroup.AppID.String(),
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupCreated: error adding local app group dependency: %v", err)
	}
}

func (c *Client) handleLocalAppGroupRemoved(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalAppGroupRemoved: %v", event)
	appGroup, ok := event.Payload.(models.AppGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupRemoved: error casting event payload to LocalAppGroup")
		return
	}

	err := c.manager.RemoveDependency(&subscriptions.LocalAppDependency{
		AppID: appGroup.AppID.String(),
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupRemoved: error removing local app group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfGroupCreated(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfGroupCreated: %v", event)
	vnfGroup, ok := event.Payload.(models.VnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error casting event payload to VnfGroup")
		return
	}

	err := c.manager.AddDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnfGroup.VnfID.String(),
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error adding local vnf group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfGroupRemoved(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfGroupRemoved: %v", event)
	vnfGroup, ok := event.Payload.(models.VnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupRemoved: error casting event payload to VnfGroup")
		return
	}

	err := c.manager.RemoveDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnfGroup.VnfID.String(),
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupRemoved: error removing local vnf group dependency: %v", err)
	}
}