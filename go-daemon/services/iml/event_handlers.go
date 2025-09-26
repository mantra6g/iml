package iml

import (
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
)

func (c *Client) handleLocalAppGroupCreated(event events.Event) {
	appGroup, ok := event.Payload.(models.AppGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupCreated: error casting event payload to LocalAppGroup")
		return
	}

	c.graph.Subscribe(appGroup.ID.String(), &LocalAppGroup{
		AppID: appGroup.AppID.String(),
	})
}

func (c *Client) handleLocalAppGroupRemoved(event events.Event) {
	appGroup, ok := event.Payload.(models.AppGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalAppGroupRemoved: error casting event payload to LocalAppGroup")
		return
	}

	c.graph.RemoveDependent(appGroup.ID.String(), &LocalAppGroup{
		AppID: appGroup.AppID.String(),
	})
}

func (c *Client) handleLocalVnfGroupCreated(event events.Event) {
	vnfGroup, ok := event.Payload.(models.VnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupCreated: error casting event payload to VnfGroup")
		return
	}

	c.graph.AddDependent(vnfGroup.ID.String(), &LocalVnfGroup{
		VnfID: vnfGroup.VnfID.String(),
	})
}

func (c *Client) handleLocalVnfGroupRemoved(event events.Event) {
	vnfGroup, ok := event.Payload.(models.VnfGroup)
	if !ok {
		logger.ErrorLogger().Printf("iml.handleLocalVnfGroupRemoved: error casting event payload to VnfGroup")
		return
	}

	c.graph.RemoveDependent(vnfGroup.ID.String(), &LocalVnfGroup{
		VnfID: vnfGroup.VnfID.String(),
	})
}