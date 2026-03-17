package apps

import "net"

type InstanceFactory interface {
	NewLocalInstance(req *RegistrationRequest) (*InstanceRegistrationResponse, error)
	DeleteInstance(containerID string) error
}

type RegistrationRequest struct {
	ApplicationID string
	ContainerID   string
}

type InstanceRegistrationResponse struct {
	IPNet       net.IPNet
	IfaceName   string
	ClusterCIDR net.IPNet
	GatewayIP   net.IP
	BridgeName  string
}
