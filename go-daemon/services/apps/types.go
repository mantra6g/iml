package apps

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/services/eventbus"
	"net"
)

type AppService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *eventbus.EventBus
}

type AppInstanceRegistrationRequest struct {
	ApplicationID string `json:"application_id" validate:"required,mongodb"`
	ContainerID   string `json:"container_id" validate:"required"`
}

type AppInstanceDetails struct {
	ContainerID string
	AppIP       *net.IPNet
}

type AppInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}