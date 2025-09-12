package vnfs

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/services/eventbus"
	"iml-daemon/services/iml"
	"net"
)

type VnfService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *eventbus.EventBus
	imlClient *iml.Client
}

// =================================================
//              INSTANCE REGISTRATION
// =================================================

type VnfInstanceRegistrationRequest struct {
	VnfID       string
	ContainerID string
}

type VnfInstanceRegistrationResponse struct {
	ContainerID string
	VnfIP       *net.IPNet
}

// =================================================
//                INSTANCE TEARDOWN
// =================================================

type VnfInstanceTeardownRequest struct {
	ContainerID string
}

type VnfInstanceTeardownResponse struct {
	ContainerID string
}
