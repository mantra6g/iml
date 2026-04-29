package net

import "net"

type DualStackNetwork struct {
	IPv4Net *net.IPNet `json:"inet4,omitempty"`
	IPv6Net *net.IPNet `json:"inet6,omitempty"`
}

type DualStackGateway struct {
	IPv4Gateway net.IP `json:"gw4,omitempty"`
	IPv6Gateway net.IP `json:"gw6,omitempty"`
}

type DualStackAddress struct {
	IPv4 net.IP `json:"ip4,omitempty"`
	IPv6 net.IP `json:"ip6,omitempty"`
}
