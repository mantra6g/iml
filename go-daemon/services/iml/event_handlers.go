package iml

import (
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml/subscriptions"
)

func (c *Client) handleLocalAppGroupCreated(event events.Event) {
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