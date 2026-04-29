package vrf

import (
	"fmt"
	"net"

	"iml-daemon/pkg/dataplane"
	netutils "iml-daemon/pkg/utils/net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type SubnetCIDR = string

type RoutingSubnet struct {
	Network       *net.IPNet
	Gateway       net.IP
	Vrf           *netlink.Vrf
	Bridge        *netlink.Bridge
	IP6Allocator  *dataplane.IPv6Allocator
	DecapSIDv6    *net.IPNet
	DecapSIDv4    *net.IPNet
	SubnetTunnels map[SubnetCIDR]*netlink.Veth
	Log           logr.Logger
}

func NewRoutingSubnet(logger logr.Logger, network *net.IPNet, tableID uint32) (subnet *RoutingSubnet, err error) {
	if network == nil {
		return nil, fmt.Errorf("invalid network")
	}
	subnet = &RoutingSubnet{
		Network: network,
		Log:     logger,
	}

	routingIP6Allocator, err := dataplane.NewIPv6Allocator(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 allocator for routing subnet: %w", err)
	}
	subnet.IP6Allocator = routingIP6Allocator

	routerVrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{
			Name: RoutingVRFName,
		},
		Table: tableID,
	}
	if err = netlink.LinkAdd(routerVrf); err != nil {
		return nil, fmt.Errorf("failed to add router VRF: %w", err)
	}
	if err = netlink.LinkSetUp(routerVrf); err != nil {
		return nil, fmt.Errorf("failed to set up router VRF: %w", err)
	}

	decapIface := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: DecapInterfaceName,
		},
	}
	if err = netlink.LinkAdd(decapIface); err != nil {
		return nil, fmt.Errorf("failed to add decap interface: %w", err)
	}
	if err = netlink.LinkSetMaster(decapIface, routerVrf); err != nil {
		return nil, fmt.Errorf("failed to set master for decap interface: %w", err)
	}
	if err = netlink.LinkSetUp(decapIface); err != nil {
		return nil, fmt.Errorf("failed to set up decap interface: %w", err)
	}

	decapSidv6, err := subnet.IP6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate decap SID: %w", err)
	}
	// ip -6 route add <decap sid> table <router vrf table> encap seg6local action End.DT6 table <router vrf table> dev <decap iface>
	var flagsEndDt6Encaps [nl.SEG6_LOCAL_MAX]bool
	flagsEndDt6Encaps[nl.SEG6_LOCAL_ACTION] = true
	flagsEndDt6Encaps[nl.SEG6_LOCAL_TABLE] = true
	decapRoutev6 := &netlink.Route{
		Dst:   decapSidv6,
		Table: int(routerVrf.Table),
		Encap: &netlink.SEG6LocalEncap{
			Flags:  flagsEndDt6Encaps,
			Action: nl.SEG6_LOCAL_ACTION_END_DT6,
			Table:  int(routerVrf.Table),
		},
		LinkIndex: decapIface.Attrs().Index,
	}
	if err = netlink.RouteAdd(decapRoutev6); err != nil {
		err = fmt.Errorf(
			"failed to execute `ip -6 route add %s table %d encap seg6local action End.DT6 vrftable %d dev %s`: %s",
			decapSidv6.String(), routerVrf.Table, routerVrf.Table, decapIface.Attrs().Name, err)
		return nil, fmt.Errorf("failed to add decap route: %w", err)
	}
	decapSidv4, err := subnet.IP6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate decap SID: %w", err)
	}
	// ip -6 route add <decap sid> table <router vrf table> encap seg6local action End.DT4 table <router vrf table> dev <decap iface>
	var flagsEndDt4Encaps [nl.SEG6_LOCAL_MAX]bool
	flagsEndDt4Encaps[nl.SEG6_LOCAL_ACTION] = true
	flagsEndDt4Encaps[nl.SEG6_LOCAL_TABLE] = true
	decapRoutev4 := &netlink.Route{
		Dst:   decapSidv4,
		Table: int(routerVrf.Table),
		Encap: &netlink.SEG6LocalEncap{
			Flags:  flagsEndDt6Encaps,
			Action: nl.SEG6_LOCAL_ACTION_END_DT4,
			Table:  int(routerVrf.Table),
		},
		LinkIndex: decapIface.Attrs().Index,
	}
	if err = netlink.RouteAdd(decapRoutev4); err != nil {
		err = fmt.Errorf(
			"failed to execute `ip -6 route add %s table %d encap seg6local action End.DT4 vrftable %d dev %s`: %s",
			decapSidv6.String(), routerVrf.Table, routerVrf.Table, decapIface.Attrs().Name, err)
		return nil, fmt.Errorf("failed to add decap route: %w", err)
	}

	subnet.DecapSIDv6 = decapSidv6
	subnet.DecapSIDv4 = decapSidv4
	subnet.Vrf = routerVrf
	return subnet, nil
}

func (r *RoutingSubnet) Teardown() {
	logger := r.Log
	logger.Info("Tearing down routing subnet")

	routerVrf, err := netlink.LinkByName(RoutingVRFName)
	if err == nil {
		err = netlink.LinkDel(routerVrf)
		if err != nil {
			logger.Error(err, "Failed to tear down router VRF link")
		}
	}
	decapIface, err := netlink.LinkByName(DecapInterfaceName)
	if err == nil {
		err = netlink.LinkDel(decapIface)
		if err != nil {
			logger.Error(err, "Failed to tear down router VRF decap0")
		}
	}
}

// HasIPsAvailable returns true if there are both IPv4 and IPv6 addresses available for allocation in
// the subnet, and false otherwise. This is used to determine whether the subnet can be used for a new
// application that requires both IPv4 and IPv6 connectivity. If the subnet only has one of the two IP
// versions available, it may not be suitable for applications that require dual-stack connectivity.
func (r *RoutingSubnet) HasIPsAvailable() bool {
	return true
}

// AllocateIPs returns a list of allocated IPs matching the subnet's stack when called
// For example, if the stack is IPv4Only, it will return one allocated IPv4 address.
// If the stack is DualStack, it will always return one allocated IPv4 FIRST and then one allocated IPv6 address.
// The returned IPs are expected to be used for the gateway of an application instance connected to this subnet.
func (r *RoutingSubnet) AllocateIPs() (netutils.DualStackNetwork, error) {
	//switch r.GetStack() {
	//case IPv4Only:
	//  ipv4, err := r.IPv4Allocator.Allocate()
	//  if err != nil {
	//    return netutils.DualStackNetwork{}, err
	//  }
	//  return netutils.DualStackNetwork{
	//    IPv4Net: ipv4,
	//  }, nil
	//case IPv6Only:
	ipv6, err := r.IP6Allocator.Allocate()
	if err != nil {
		return netutils.DualStackNetwork{}, err
	}
	return netutils.DualStackNetwork{
		IPv4Net: ipv6,
	}, nil
	//case DualStack:
	//  ipv6, err := r.IPv6Allocator.Allocate()
	//  if err != nil {
	//    return netutils.DualStackNetwork{}, err
	//  }
	//  ipv4, err := r.IPv4Allocator.Allocate()
	//  if err != nil {
	//    return netutils.DualStackNetwork{}, err
	//  }
	//  return netutils.DualStackNetwork{
	//    IPv4Net: ipv4,
	//    IPv6Net: ipv6,
	//  }, nil
	//}
	//return netutils.DualStackNetwork{}, fmt.Errorf("unknown stack type: %s", r.GetStack())
}

func (r *RoutingSubnet) AddDefaultRouteViaSubnet(dstSubnet Subnet) error {
	logger := r.Log
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
	return r.AddRouteViaVRF(defaultNetwork, dstSubnet.GetVRFName())
}

func (r *RoutingSubnet) AddRouteToSubnet(dstSubnet Subnet) error {
	logger := r.Log
	logger.V(1).Info("Adding subnet route to routing subnet", "dstSubnet", dstSubnet)
	if dstSubnet == nil {
		return fmt.Errorf("destination subnet cannot be nil")
	}
	dualNet := dstSubnet.GetNetwork()
	return r.AddRouteViaVRF(dualNet, dstSubnet.GetVRFName())
}

func (r *RoutingSubnet) AddRouteViaVRF(dst netutils.DualStackNetwork, vrfName string) error {
	logger := r.Log
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
			Table:     int(r.Vrf.Table),
			LinkIndex: vrf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv6Route); err != nil {
			logger.Error(err, "failed to add IPv6 route to app subnet in routing VRF",
				"dst_net", dst.IPv6Net.String(), "table", r.Vrf.Table, "dst_vrf", vrf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}
	if dst.IPv4Net != nil {
		ipv4Route := &netlink.Route{
			Dst:       dst.IPv4Net,
			Table:     int(r.Vrf.Table),
			LinkIndex: vrf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv4Route); err != nil {
			logger.Error(err, "failed to add IPv4 route to app subnet in routing VRF",
				"dst_net", dst.IPv4Net.String(), "table", r.Vrf.Table, "dst_vrf", vrf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}
	return nil
}

func (r *RoutingSubnet) AddDefaultRoute(gatewayIPs netutils.DualStackAddress, tunnelInterfaceName string) error {
	logger := r.Log
	logger.V(1).Info("Adding default route to routing subnet", "gatewayIPs", gatewayIPs, "tunnelInterfaceName", tunnelInterfaceName)

	err := r.AddRoute(netutils.DualStackNetwork{
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

func (r *RoutingSubnet) AddRoute(dst netutils.DualStackNetwork, gw netutils.DualStackAddress, outInterface string) error {
	logger := r.Log
	logger.V(1).Info("Adding route to routing subnet", "dst", dst, "gw", gw, "outInterface", outInterface)

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
	if err = netlink.LinkSetMaster(outIf, r.Vrf); err != nil {
		return fmt.Errorf("failed to set master for router tunnel in routing subnet: %w", err)
	}

	if gw.IPv6 != nil {
		// Create a route in the application VRF to reach the router subnet using
		// the router tunnel as the outgoing interface.
		ipv6Route := &netlink.Route{
			Dst:       dst.IPv6Net,
			Gw:        gw.IPv6,
			Table:     int(r.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv6Route); err != nil {
			logger.Error(err, "failed to add IPv6 route to app subnet in routing VRF",
				"dst", dst.IPv6Net.String(), "table", r.Vrf.Table)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	if gw.IPv4 != nil {
		ipv4Route := &netlink.Route{
			Dst:       dst.IPv4Net,
			Gw:        gw.IPv4,
			Table:     int(r.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv4Route); err != nil {
			logger.Error(err, "failed to add IPv4 route to app subnet in routing VRF",
				"dst", dst.IPv4Net.String(), "table", r.Vrf.Table)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	return nil
}

func (r *RoutingSubnet) RemoveRoute(dst netutils.DualStackNetwork) error {
	logger := r.Log
	logger.V(1).Info("Removing route from routing subnet", "dst", dst)

	if dst.IPv4Net == nil && dst.IPv6Net == nil {
		return fmt.Errorf(
			"destination's IPv4Net and IPv6Net are both nil: destination must contain at least one non-nil network")
	}
	if dst.IPv4Net != nil {
		ipv4Route := &netlink.Route{
			Dst:   dst.IPv4Net,
			Table: int(r.Vrf.Table),
		}
		if err := netlink.RouteDel(ipv4Route); err != nil {
			logger.Error(err, "failed to delete IPv4 route to app subnet in routing VRF", "dst", dst.IPv4Net.String(), "table", r.Vrf.Table)
			return fmt.Errorf("failed to delete IPv4 route to app subnet in routing VRF: %w", err)
		}
	}
	if dst.IPv6Net != nil {
		ipv6Route := &netlink.Route{
			Dst:   dst.IPv6Net,
			Table: int(r.Vrf.Table),
		}
		if err := netlink.RouteDel(ipv6Route); err != nil {
			logger.Error(err, "failed to delete IPv6 route to app subnet in routing VRF", "dst", dst.IPv6Net.String(), "table", r.Vrf.Table)
			return fmt.Errorf("failed to delete IPv6 route to app subnet in routing VRF: %w", err)
		}
	}
	return nil
}

func (r *RoutingSubnet) GetNetwork() netutils.DualStackNetwork {
	return netutils.DualStackNetwork{
		IPv4Net: nil,
		IPv6Net: r.Network,
	}
}

func (r *RoutingSubnet) GetGateway() netutils.DualStackAddress {
	return netutils.DualStackAddress{
		IPv4: nil,
		IPv6: r.Gateway,
	}
}

func (r *RoutingSubnet) GetStack() StackType {
	return IPv6Only
}

func (r *RoutingSubnet) GetVRFName() string {
	return r.Vrf.Name
}
