package netutils

import (
	"net"
)

type DualStackNetwork struct {
	IPv4Net *net.IPNet `json:"inet4,omitempty"`
	IPv6Net *net.IPNet `json:"inet6,omitempty"`
}

func (net *DualStackNetwork) IsEmpty() bool {
	return net.IPv4Net == nil && net.IPv6Net == nil
}

type DualStackAddress struct {
	IPv4 net.IP `json:"ip4,omitempty"`
	IPv6 net.IP `json:"ip6,omitempty"`
}

func (net *DualStackAddress) IsEmpty() bool {
	return net.IPv4 == nil && net.IPv6 == nil
}

type DualStackAddressList struct {
	IPv4Addresses []net.IP `json:"ip4s,omitempty"`
	IPv6Addresses []net.IP `json:"ip6s,omitempty"`
}

func (net *DualStackAddressList) IsEmpty() bool {
	return len(net.IPv4Addresses) == 0 && len(net.IPv6Addresses) == 0
}

type DualStackRoute struct {
	IPv4Route Route `json:"ip4route,omitempty"`
	IPv6Route Route `json:"ip6route,omitempty"`
}

func (net *DualStackRoute) IsEmpty() bool {
	return net.IPv4Route.IsEmpty() && net.IPv6Route.IsEmpty()
}

type Route struct {
	Destination *net.IPNet `json:"dst,omitempty"`
	Gateway     net.IP     `json:"gw,omitempty"`
}

func (r *Route) IsEmpty() bool {
	return r.Destination == nil && r.Gateway == nil
}
