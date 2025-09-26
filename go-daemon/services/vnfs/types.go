package vnfs

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/vnfs"
	"net"
)

type VnfService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *events.EventBus
	imlClient *iml.Client
	vnfFactory *vnfs.InstanceFactory
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
