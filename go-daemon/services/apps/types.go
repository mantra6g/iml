package apps

import (
	"iml-daemon/apps"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/services/events"
	"net"
)

type AppService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *events.EventBus
	appFactory *apps.InstanceFactory
}

type AppInstanceRegistrationRequest struct {
	ApplicationID string
	ContainerID   string
}

type AppInstanceDetails struct {
	ContainerID string
	AppIP       *net.IPNet
}

type AppInstanceTeardownRequest struct {
	ContainerID string
}