package netutils

import (
	"fmt"
	"net"
	"net/netip"
)

type DualStackNetwork struct {
	IPv4Net *net.IPNet `json:"inet4,omitempty"`
	IPv6Net *net.IPNet `json:"inet6,omitempty"`
}

func (net DualStackNetwork) IsEmpty() bool {
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

func ParseDualStackGatewayFromStrings(gwStrings []string) (DualStackAddress, error) {
	addrs, err := ParseDualStackAddressFromStrings(gwStrings)
	if err != nil {
		return DualStackAddress{}, err
	}
	return DualStackAddress{
		IPv4: addrs.IPv4,
		IPv6: addrs.IPv6,
	}, nil
}

func ParseDualStackAddressFromStrings(ipStrings []string) (DualStackAddress, error) {
	result := DualStackAddress{}
	targetIPs := make([]netip.Addr, 0, len(ipStrings))
	if len(ipStrings) > 2 {
		return result, fmt.Errorf("too many IP addresses provided: expected at most 2 but got %d", len(ipStrings))
	}
	for _, targetIP := range ipStrings {
		ip, err := netip.ParseAddr(ipStrings[0])
		if err != nil {
			return result, fmt.Errorf("invalid IP address: %s", targetIP)
		}
		targetIPs = append(targetIPs, ip)
	}
	for i := range targetIPs {
		isIPv4 := targetIPs[i].Is4()
		if isIPv4 {
			if result.IPv4 != nil {
				return result, fmt.Errorf("multiple IPv4 addresses provided: %s and %s", result.IPv4, targetIPs[i])
			}
			result.IPv4 = targetIPs[i].AsSlice()
			continue
		}
		if result.IPv6 != nil {
			return result, fmt.Errorf("multiple IPv6 addresses provided: %s and %s", result.IPv6, targetIPs[i])
		}
		result.IPv6 = targetIPs[i].AsSlice()
	}
	return result, nil
}

func ParseDualStackNetworkFromStrings(networkStrings []string) (DualStackNetwork, error) {
	result := DualStackNetwork{}
	targetNets := make([]netip.Prefix, 0, len(networkStrings))
	if len(networkStrings) > 2 {
		return result, fmt.Errorf(
			"too many network addresses provided: expected at most 2 but got %d", len(networkStrings))
	}
	for _, networkString := range networkStrings {
		prefix, err := netip.ParsePrefix(networkString)
		if err != nil {
			return result, fmt.Errorf("invalid IP address: %s", networkString)
		}
		targetNets = append(targetNets, prefix)
	}
	for i := range targetNets {
		isIPv4 := targetNets[i].Addr().Is4()
		if isIPv4 {
			if result.IPv4Net != nil {
				return result, fmt.Errorf("multiple IPv4 networks provided: %s and %s", result.IPv4Net, targetNets[i])
			}
			result.IPv4Net = &net.IPNet{
				IP:   net.IP(targetNets[i].Addr().AsSlice()),
				Mask: net.CIDRMask(targetNets[i].Bits(), targetNets[i].Addr().BitLen()),
			}
			continue
		}
		if result.IPv6Net != nil {
			return result, fmt.Errorf("multiple IPv6 networks provided: %s and %s", result.IPv6Net, targetNets[i])
		}
		result.IPv6Net = &net.IPNet{
			IP:   net.IP(targetNets[i].Addr().AsSlice()),
			Mask: net.CIDRMask(targetNets[i].Bits(), targetNets[i].Addr().BitLen()),
		}
	}
	return result, nil
}

func ParseDualStackAddressListFromStrings(ipStrings []string) (DualStackAddressList, error) {
	result := DualStackAddressList{}
	targetIPs := make([]netip.Addr, 0, len(ipStrings))
	if len(ipStrings) > 2 {
		return result, fmt.Errorf("too many IP addresses provided: expected at most 2 but got %d", len(ipStrings))
	}
	for _, targetIP := range ipStrings {
		ip, err := netip.ParseAddr(ipStrings[0])
		if err != nil {
			return result, fmt.Errorf("invalid IP address: %s", targetIP)
		}
		targetIPs = append(targetIPs, ip)
	}
	for i := range targetIPs {
		isIPv4 := targetIPs[i].Is4()
		if isIPv4 {
			if result.IPv4Addresses == nil {
				result.IPv4Addresses = make([]net.IP, 0)
			}
			result.IPv4Addresses = append(result.IPv4Addresses, targetIPs[i].AsSlice())
			continue
		}
		if result.IPv6Addresses == nil {
			result.IPv6Addresses = make([]net.IP, 0)
		}
		result.IPv6Addresses = append(result.IPv6Addresses, targetIPs[i].AsSlice())
	}
	return result, nil
}
