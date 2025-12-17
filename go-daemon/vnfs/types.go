package vnfs

import "net"

type InstanceFactory interface {
	NewLocalInstance(req *RegistrationRequest) (*InstanceRegistrationResponse, error)
	TeardownVnfInstance(containerID string) error
}

type RegistrationRequest struct {
	VnfID       string
	ContainerID string
}

type InstanceRegistrationResponse struct {
	IPNet       net.IPNet
	SIDs        []net.IPNet
	IfaceName   string
	ClusterCIDR net.IPNet
	GatewayIP   net.IP
	BridgeName  string
}
