package registry

import (
	"net"
)

type AppRegistrationRequest struct {
	ApplicationID string
	ContainerID   string
}

type AppDetails struct {
	ContainerID string
	AppIP      *net.IPNet
}

type VnfDetails struct {
	ContainerID string
	VnfIP      *net.IPNet
}

type Registry struct {
	appRegistry map[string]*AppDetails // Maps appID to AppDetails
	nfvRegistry map[string]*VnfDetails // Maps vnfID to VnfDetails
}