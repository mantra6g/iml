package netutils

import "net"

type DualStackNetwork struct {
	IPv4Net *net.IPNet `json:"inet4,omitempty"`
	IPv6Net *net.IPNet `json:"inet6,omitempty"`
}

func (net *DualStackNetwork) IsEmpty() bool {
	return net.IPv4Net == nil && net.IPv6Net == nil
}

type DualStackGateway struct {
	IPv4Gateway net.IP `json:"gw4,omitempty"`
	IPv6Gateway net.IP `json:"gw6,omitempty"`
}

func (net *DualStackGateway) IsEmpty() bool {
	return net.IPv4Gateway == nil && net.IPv6Gateway == nil
}

type DualStackAddress struct {
	IPv4 net.IP `json:"ip4,omitempty"`
	IPv6 net.IP `json:"ip6,omitempty"`
}

func (net *DualStackAddress) IsEmpty() bool {
	return net.IPv4 == nil && net.IPv6 == nil
}
