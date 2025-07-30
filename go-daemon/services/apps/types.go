package apps

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"net"
)

type AppService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
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