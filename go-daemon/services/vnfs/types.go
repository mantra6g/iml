package vnfs

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/services/eventbus"
	"net"
)

type VnfService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *eventbus.EventBus
}

// =================================================
//              INSTANCE REGISTRATION
// =================================================

type VnfInstanceRegistrationRequest struct {
	VnfID       string `json:"vnf_id" validate:"required,mongodb"`
	ContainerID string `json:"container_id" validate:"required"`
}

type VnfInstanceRegistrationResponse struct {
	ContainerID string
	VnfIP       *net.IPNet
}

// =================================================
//                INSTANCE TEARDOWN
// =================================================

type VnfInstanceTeardownRequest struct {
	ContainerID string `json:"container_id" validate:"required"`
}

type VnfInstanceTeardownResponse struct {
	ContainerID string
}
