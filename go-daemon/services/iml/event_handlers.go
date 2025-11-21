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

	// TODO: Race condition possible here if temporaryAppDependency ends before we get here
	app, err := c.repo.FindActiveAppByLocalID(appGroup.AppID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupCreated: error finding active app by local ID %s: %v", appGroup.AppID, err)
		return
	}

	err = c.manager.AddDependency(&subscriptions.LocalAppDependency{
		AppID: app.GlobalID,
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

	// TODO: Race condition possible here if temporaryAppDependency ends before we get here
	app, err := c.repo.FindActiveAppByLocalID(appGroup.AppID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupCreated: error finding active app by local ID %s: %v", appGroup.AppID, err)
		return
	}

	err = c.manager.RemoveDependency(&subscriptions.LocalAppDependency{
		AppID: app.GlobalID,
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupRemoved: error removing local app group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfGroupCreated(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfGroupCreated: %v", event)
	vnfGroup, ok := event.Payload.(models.SimpleVnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error casting event payload to VnfGroup")
		return
	}

	// TODO: Race condition possible here if temporaryVnfDependency ends before we get here
	vnf, err := c.repo.FindActiveNetworkFunctionByLocalID(vnfGroup.VnfID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error finding active VNF by local ID %s: %v", vnfGroup.VnfID, err)
		return
	}

	err = c.manager.AddDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnf.GlobalID,
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error adding local vnf group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfMultiplexedGroupCreated(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfMultiplexedGroupCreated: %v", event)
	vnfGroup, ok := event.Payload.(models.MultiplexedVnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfMultiplexedGroupCreated: error casting event payload to VnfGroup")
		return
	}

	vnf, err := c.repo.FindActiveNetworkFunctionByLocalID(vnfGroup.VnfID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfMultiplexedGroupCreated: error finding active VNF by local ID %s: %v", vnfGroup.VnfID, err)
		return
	}

	err = c.manager.AddDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnf.GlobalID,
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfMultiplexedGroupCreated: error adding local vnf multiplexed group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfGroupRemoved(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfGroupRemoved: %v", event)
	vnfGroup, ok := event.Payload.(models.SimpleVnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupRemoved: error casting event payload to VnfGroup")
		return
	}

	// TODO: Race condition possible here if temporaryVnfDependency ends before we get here
	vnf, err := c.repo.FindActiveNetworkFunctionByLocalID(vnfGroup.VnfID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error finding active VNF by local ID %s: %v", vnfGroup.VnfID, err)
		return
	}

	err = c.manager.RemoveDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnf.GlobalID,
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupRemoved: error removing local vnf group dependency: %v", err)
	}
}

func (c *Client) handleLocalVnfMultiplexedGroupRemoved(event events.Event) {
	logger.DebugLogger().Printf("iml.handleLocalVnfMultiplexedGroupRemoved: %v", event)
	vnfGroup, ok := event.Payload.(models.MultiplexedVnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfMultiplexedGroupRemoved: error casting event payload to VnfMultiplexedGroup")
		return
	}

	// TODO: Race condition possible here if temporaryVnfDependency ends before we get here
	vnf, err := c.repo.FindActiveNetworkFunctionByLocalID(vnfGroup.VnfID)
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error finding active VNF by local ID %s: %v", vnfGroup.VnfID, err)
		return
	}

	err = c.manager.RemoveDependency(&subscriptions.LocalVnfDependency{
		VnfID: vnf.GlobalID,
	})
	if err != nil {
		logger.ErrorLogger().Printf("iml.handleLocalVnfMultiplexedGroupRemoved: error removing local vnf multiplexed group dependency: %v", err)
	}
}
