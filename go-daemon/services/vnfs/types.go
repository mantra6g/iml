package vnfs

import (
	"iml-daemon/db"
	"iml-daemon/helpers"
	"net"
)

type VnfService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
}

type VnfInstanceDetails struct {
	ContainerID string
	VnfIP       *net.IPNet
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
