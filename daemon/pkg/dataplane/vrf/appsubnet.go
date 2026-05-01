package vrf

import (
	"fmt"
	"iml-daemon/pkg/dataplane"
	"net"

	vrfutil "iml-daemon/pkg/dataplane/vrf/util"
	netutils "iml-daemon/pkg/utils/net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type AppSubnet struct {
	Networks      netutils.DualStackNetwork
	GatewayIPs    netutils.DualStackAddress
	Bridge        *netlink.Bridge
	Vrf           *netlink.Vrf
	IPv6Allocator *dataplane.IPv6Allocator
	IPv4Allocator *dataplane.IPv4Allocator
	Log           logr.Logger
}

func NewAppSubnet(logger logr.Logger, ip4Net *net.IPNet, ip6Net *net.IPNet, tableID uint32) (subnet *AppSubnet, err error) {
	if ip6Net == nil && ip4Net == nil {
		return nil, fmt.Errorf("ip6Net and ip4Net cant both be nil. Provide at least one of them")
	}
	subnet = &AppSubnet{}
	subnet.Log = logger

	var gatewayIPv4 *net.IPNet
	var ip4Allocator *dataplane.IPv4Allocator
	if ip4Net != nil {
		ip4Allocator, err = dataplane.NewIPv4Allocator(ip4Net)
		if err != nil {
			return nil, fmt.Errorf("failed to create IPv4 allocator for application subnet: %w", err)
		}
		subnet.IPv4Allocator = ip4Allocator

		gatewayIPv4, err = subnet.IPv4Allocator.Allocate()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate gateway IPv4 for application subnet: %w", err)
		}
		subnet.Networks.IPv4Net = ip4Net
		subnet.GatewayIPs.IPv4 = gatewayIPv4.IP
	}

	var gatewayIPv6 *net.IPNet
	var ip6Allocator *dataplane.IPv6Allocator
	if ip6Net != nil {
		ip6Allocator, err = dataplane.NewIPv6Allocator(ip6Net)
		if err != nil {
			return nil, fmt.Errorf("failed to create IPv6 allocator for application subnet: %w", err)
		}
		subnet.IPv6Allocator = ip6Allocator

		gatewayIPv6, err = subnet.IPv6Allocator.Allocate()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate gateway IPv6 for application subnet: %w", err)
		}
		subnet.Networks.IPv6Net = ip6Net
		subnet.GatewayIPs.IPv6 = gatewayIPv6.IP
	}

	appVrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{
			Name: vrfutil.GetVRFName(tableID),
		},
		Table: tableID,
	}
	if err = netlink.LinkAdd(appVrf); err != nil {
		return nil, fmt.Errorf("failed to add VRF for application subnet: %w", err)
	}
	subnet.Vrf = appVrf
	if err = netlink.LinkSetUp(appVrf); err != nil {
		subnet.Teardown()
		return nil, fmt.Errorf("failed to set up VRF for application subnet: %w", err)
	}

	// From now on, whenever the function returns, issue a cleanup if the error is not nil
	defer func() {
		if err != nil {
			subnet.Teardown()
		}
	}()

	// Create a bridge for this subnet
	bridgeName, err := vrfutil.GenerateRandomName("br", 8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate bridge name: %w", err)
	}

	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:        bridgeName,
			MasterIndex: appVrf.Attrs().Index,
		},
	}
	if err = netlink.LinkAdd(bridge); err != nil {
		return nil, fmt.Errorf("failed to add bridge %s: %w", bridgeName, err)
	}
	subnet.Bridge = bridge

	if err = netlink.AddrAdd(bridge, &netlink.Addr{IPNet: gatewayIPv6}); err != nil {
		return nil, fmt.Errorf("failed to add IPv6 address to bridge %s: %w", bridge.Name, err)
	}
	if err = netlink.AddrAdd(bridge, &netlink.Addr{IPNet: gatewayIPv4}); err != nil {
		return nil, fmt.Errorf("failed to add IPv4 address to bridge %s: %w", bridge.Name, err)
	}
	if err = netlink.LinkSetUp(bridge); err != nil {
		return nil, fmt.Errorf("failed to set up bridge %s: %w", bridgeName, err)
	}
	return
}

func (s *AppSubnet) Teardown() {
	if s == nil {
		return
	}
	logger := s.Log
	//if s.VethBridgeVRF != nil {
	//	if err := netlink.LinkDel(s.VethBridgeVRF); err != nil {
	//		logger.Error(err, "failed to delete veth", "name", s.VethBridgeVRF.Attrs().Name)
	//	}
	//}
	if s.Bridge != nil {
		if err := netlink.LinkDel(s.Bridge); err != nil {
			logger.Error(err, "failed to delete bridge", "name", s.Bridge.Attrs().Name)
		}
	}
	//if s.Tunnel != nil {
	//	if err := netlink.LinkDel(s.Tunnel); err != nil {
	//		logger.Error(err, "failed to delete tunnel", "name", s.Tunnel.Attrs().Name)
	//	}
	//}
	if s.Vrf != nil {
		if err := netlink.LinkDel(s.Vrf); err != nil {
			logger.Error(err, "failed to delete VRF", "name", s.Vrf.Attrs().Name)
		}
	}
}

// HasIPsAvailable returns true if there are both IPv4 and IPv6 addresses available for allocation in
// the subnet, and false otherwise. This is used to determine whether the subnet can be used for a new
// application that requires both IPv4 and IPv6 connectivity. If the subnet only has one of the two IP
// versions available, it may not be suitable for applications that require dual-stack connectivity.
func (s *AppSubnet) HasIPsAvailable() bool {
	return true
}

// AllocateIPs returns a list of allocated IPs matching the subnet's stack when called
// For example, if the stack is IPv4Only, it will return one allocated IPv4 address.
// If the stack is DualStack, it will always return one allocated IPv4 FIRST and then one allocated IPv6 address.
// The returned IPs are expected to be used for the gateway of an application instance connected to this subnet.
func (s *AppSubnet) AllocateIPs() (netutils.DualStackNetwork, error) {
	switch s.GetStack() {
	case IPv4Only:
		ipv4, err := s.IPv4Allocator.Allocate()
		if err != nil {
			return netutils.DualStackNetwork{}, err
		}
		return netutils.DualStackNetwork{
			IPv4Net: ipv4,
		}, nil
	case IPv6Only:
		ipv6, err := s.IPv6Allocator.Allocate()
		if err != nil {
			return netutils.DualStackNetwork{}, err
		}
		return netutils.DualStackNetwork{
			IPv4Net: ipv6,
		}, nil
	case DualStack:
		ipv6, err := s.IPv6Allocator.Allocate()
		if err != nil {
			return netutils.DualStackNetwork{}, err
		}
		ipv4, err := s.IPv4Allocator.Allocate()
		if err != nil {
			return netutils.DualStackNetwork{}, err
		}
		return netutils.DualStackNetwork{
			IPv4Net: ipv4,
			IPv6Net: ipv6,
		}, nil
	}
	return netutils.DualStackNetwork{}, fmt.Errorf("unknown stack type: %s", s.GetStack())
}

func (s *AppSubnet) AddDefaultRouteViaSubnet(dstSubnet Subnet) error {
	logger := s.Log
	logger.V(1).Info("Adding subnet route to routing subnet", "dstSubnet", dstSubnet)
	if dstSubnet == nil {
		return fmt.Errorf("destination subnet cannot be nil")
	}
	defaultNetwork := netutils.DualStackNetwork{
		IPv4Net: &net.IPNet{
			IP:   net.IPv4zero,
			Mask: net.CIDRMask(0, 32),
		},
		IPv6Net: &net.IPNet{
			IP:   net.IPv6zero,
			Mask: net.CIDRMask(0, 128),
		},
	}
	return s.AddRouteViaVRF(defaultNetwork, dstSubnet.GetVRFName())
}

func (s *AppSubnet) AddRouteToSubnet(dstSubnet Subnet) error {
	logger := s.Log
	logger.V(1).Info("Adding subnet route to routing subnet", "dstSubnet", dstSubnet)
	if dstSubnet == nil {
		return fmt.Errorf("destination subnet cannot be nil")
	}
	dualNet := dstSubnet.GetNetwork()
	return s.AddRouteViaVRF(dualNet, dstSubnet.GetVRFName())
}

func (s *AppSubnet) AddRouteViaVRF(dst netutils.DualStackNetwork, vrfName string) error {
	logger := s.Log
	if dst.IsEmpty() {
		return fmt.Errorf(
			"subnet's IPv4Net and IPv6Net are both nil: subnet must contain at least one non-nil network")
	}
	vrf, err := netlink.LinkByName(vrfName)
	if err != nil {
		return fmt.Errorf("failed to get VRF for destination subnet: %w", err)
	}
	if dst.IPv6Net != nil {
		// Create a route in the application VRF to reach the router subnet using
		// the router tunnel as the outgoing interface.
		ipv6Route := &netlink.Route{
			Dst:       dst.IPv6Net,
			Table:     int(s.Vrf.Table),
			LinkIndex: vrf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv6Route); err != nil {
			logger.Error(err, "failed to add IPv6 route to app subnet in routing VRF",
				"dst_net", dst.IPv6Net.String(), "table", s.Vrf.Table, "dst_vrf", vrf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}
	if dst.IPv4Net != nil {
		ipv4Route := &netlink.Route{
			Dst:       dst.IPv4Net,
			Table:     int(s.Vrf.Table),
			LinkIndex: vrf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv4Route); err != nil {
			logger.Error(err, "failed to add IPv4 route to app subnet in routing VRF",
				"dst_net", dst.IPv4Net.String(), "table", s.Vrf.Table, "dst_vrf", vrf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}
	return nil
}

func (s *AppSubnet) AddDefaultRoute(gatewayIPs netutils.DualStackAddress, tunnelInterfaceName string) error {
	err := s.AddRoute(netutils.DualStackNetwork{
		IPv4Net: &net.IPNet{
			IP:   net.IPv4zero,
			Mask: net.CIDRMask(0, 32),
		},
		IPv6Net: &net.IPNet{
			IP:   net.IPv6zero,
			Mask: net.CIDRMask(0, 128),
		},
	}, gatewayIPs, tunnelInterfaceName)
	return err
}

func (s *AppSubnet) AddRoute(dst netutils.DualStackNetwork, gw netutils.DualStackAddress, outInterface string) error {
	logger := s.Log
	logger.V(1).Info("Adding route to application subnet", "dst", dst, "gw", gw, "outInterface", outInterface)

	if dst.IPv4Net == nil && dst.IPv6Net == nil {
		return fmt.Errorf(
			"destination's IPv4Net and IPv6Net are both nil: destination must contain at least one non-nil network")
	}
	if dst.IPv4Net != nil && gw.IPv4 == nil {
		return fmt.Errorf(
			"destination's IPv4 network is non-nil (%s) but gatewayIPs contains nil IPv4Gateway", dst.IPv4Net)
	}
	if dst.IPv6Net != nil && gw.IPv6 == nil {
		return fmt.Errorf(
			"destination's IPv6 network is non-nil (%s) but gatewayIPs contains nil IPv6Gateway", dst.IPv6Net)
	}

	outIf, err := netlink.LinkByName(outInterface)
	if err != nil {
		return fmt.Errorf("failed to get output interface %s from subnet: %w", outInterface, err)
	}
	if err = netlink.LinkSetMaster(outIf, s.Vrf); err != nil {
		return fmt.Errorf("failed to set master for router tunnel in routing subnet: %w", err)
	}

	if gw.IPv6 != nil {
		// Create a route in the application VRF to reach the router subnet using
		// the router tunnel as the outgoing interface.
		ipv6Route := &netlink.Route{
			Dst:       dst.IPv6Net,
			Gw:        gw.IPv6,
			Table:     int(s.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv6Route); err != nil {
			logger.Error(err, "failed to execute route add",
				"dst", ipv6Route.Dst, "gw", ipv6Route.Gw, "table", ipv6Route.Table, "dev", outIf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	if gw.IPv4 != nil {
		ipv4Route := &netlink.Route{
			Dst:       dst.IPv4Net,
			Gw:        gw.IPv4,
			Table:     int(s.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv4Route); err != nil {
			logger.Error(err, "failed to execute route add",
				"dst", ipv4Route.Dst, "gw", ipv4Route.Gw, "table", ipv4Route.Table, "dev", outIf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	return nil
}

func (s *AppSubnet) AddSRv6Route(dst netutils.DualStackNetwork, sids []net.IP, decapSIDv4, decapSIDv6 *net.IPNet) error {
	if dst.IsEmpty() {
		return fmt.Errorf("destination's IPv4Net and IPv6Net are both nil")
	}
	if dst.IPv4Net != nil {
		// ip route add <dstNet4> vrf <subnet.Vrf> encap seg6 mode encap segs <sids> dev <subnet.tunnel>
		ipv4Sids := reversed(append(sids, decapSIDv4.IP))
		route := &netlink.Route{
			Dst:   dst.IPv4Net,
			Table: int(s.Vrf.Table),
			Encap: &netlink.SEG6Encap{
				Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
				Segments: ipv4Sids,
			},
			LinkIndex: s.Bridge.Attrs().Index,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return fmt.Errorf("failed to add SRv6 route to %s with segs %s: %w", dst.IPv4Net.String(), sids, err)
		}
	}
	if dst.IPv6Net != nil {
		// same as above but in IPv6
		ipv6Sids := reversed(append(sids, decapSIDv6.IP))
		route := &netlink.Route{
			Dst:   dst.IPv6Net,
			Table: int(s.Vrf.Table),
			Encap: &netlink.SEG6Encap{
				Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
				Segments: ipv6Sids,
			},
			LinkIndex: s.Bridge.Attrs().Index,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return fmt.Errorf("failed to add SRv6 route to %s with segs %s: %w", dst.IPv6Net.String(), sids, err)
		}
	}
	return nil
}

func (s *AppSubnet) DeleteSRv6Route(dst netutils.DualStackNetwork) error {
	if dst.IsEmpty() {
		return fmt.Errorf("destination's IPv4Net and IPv6Net are both nil")
	}
	if dst.IPv4Net != nil {
		route := &netlink.Route{
			Dst:   dst.IPv4Net,
			Table: int(s.Vrf.Table),
		}
		err := netlink.RouteDel(route)
		if err != nil {
			return fmt.Errorf("failed to delete SRv6 route to %s: %w", dst.IPv4Net.String(), err)
		}
	}
	if dst.IPv6Net != nil {
		route := &netlink.Route{
			Dst:   dst.IPv6Net,
			Table: int(s.Vrf.Table),
		}
		err := netlink.RouteDel(route)
		if err != nil {
			return fmt.Errorf("failed to delete SRv6 route to %s: %w", dst.IPv6Net.String(), err)
		}
	}
	return nil
}

func (s *AppSubnet) GetNetwork() netutils.DualStackNetwork {
	return s.Networks
}

func (s *AppSubnet) GetGateway() netutils.DualStackAddress {
	return s.GatewayIPs
}

func (s *AppSubnet) GetStack() StackType {
	if s.Networks.IPv4Net != nil && s.Networks.IPv6Net != nil {
		return DualStack
	} else if s.Networks.IPv4Net != nil {
		return IPv4Only
	} else if s.Networks.IPv6Net != nil {
		return IPv6Only
	}
	return UnknownStack
}

func (s *AppSubnet) GetVRFName() string {
	return s.Vrf.Name
}

func reversed[T any](slice []T) []T {
	result := make([]T, len(slice))
	for i := range slice {
		result[len(slice)-1-i] = slice[i]
	}
	return result
}
