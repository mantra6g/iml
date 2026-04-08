package vrf

import (
	"fmt"
	"net"

	"iml-daemon/logger"
	"iml-daemon/pkg/dataplane"
	netutils "iml-daemon/pkg/utils/net"

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
	DecapSID      *net.IPNet
	SubnetTunnels map[SubnetCIDR]*netlink.Veth
}

func NewRoutingSubnet(network *net.IPNet, tableID uint32) (subnet *RoutingSubnet, err error) {
	if network == nil {
		return nil, fmt.Errorf("invalid network")
	}
	subnet = &RoutingSubnet{
		Network: network,
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

	decapSid, err := subnet.IP6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate decap SID: %w", err)
	}
	// ip -6 route add <decap sid> table <router vrf table> encap seg6local action End.DT6 table <router vrf table> dev <decap iface>
	var flagsEndDt6Encaps [nl.SEG6_LOCAL_MAX]bool
	flagsEndDt6Encaps[nl.SEG6_LOCAL_ACTION] = true
	flagsEndDt6Encaps[nl.SEG6_LOCAL_TABLE] = true
	decapRoute := &netlink.Route{
		Dst:   decapSid,
		Table: int(routerVrf.Table),
		Encap: &netlink.SEG6LocalEncap{
			Flags:  flagsEndDt6Encaps,
			Action: nl.SEG6_LOCAL_ACTION_END_DT6,
			Table:  int(routerVrf.Table),
		},
		LinkIndex: decapIface.Attrs().Index,
	}
	if err = netlink.RouteAdd(decapRoute); err != nil {
		logger.ErrorLogger().Printf(
			"Failed to execute `ip -6 route add %s table %d encap seg6local action End.DT6 vrftable %d dev %s`: %s",
			decapSid.String(), routerVrf.Table, routerVrf.Table, decapIface.Attrs().Name, err)
		return nil, fmt.Errorf("failed to add decap route: %w", err)
	}
	subnet.DecapSID = decapSid
	subnet.Vrf = routerVrf
	return subnet, nil
}

func (r *RoutingSubnet) Teardown() {
	routerVrf, err := netlink.LinkByName(RoutingVRFName)
	if err == nil {
		err = netlink.LinkDel(routerVrf)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to tear down router VRF link: %s", err)
		}
	}
	decapIface, err := netlink.LinkByName(DecapInterfaceName)
	if err == nil {
		err = netlink.LinkDel(decapIface)
		if err != nil {
			logger.ErrorLogger().Printf("Failed to tear down router VRF decap0: %s", err)
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

func (r *RoutingSubnet) AddRouteToSubnet(subnet2 Subnet, gatewayIPs netutils.DualStackAddress, tunnelInterfaceName string) error {
	if subnet2.GetNetwork().IPv4Net == nil && subnet2.GetNetwork().IPv6Net == nil {
		return fmt.Errorf(
			"subnet's IPv4Net and IPv6Net are both nil: subnet must contain at least one non-nil network")
	}
	if subnet2.GetNetwork().IPv4Net == nil && gatewayIPs.IPv4 != nil {
		return fmt.Errorf(
			"subnet's IPv4Net is nil but gatewayIPs contains non-nil IPv4Gateway: " +
				"gateway IPs are inconsistent with subnet network")
	}
	if subnet2.GetNetwork().IPv6Net == nil && gatewayIPs.IPv6 != nil {
		return fmt.Errorf(
			"subnet's IPv6Net is nil but gatewayIPs contains non-nil IPv6Gateway: " +
				"gateway IPs are inconsistent with subnet network")
	}
	if subnet2.GetNetwork().IPv4Net != nil && gatewayIPs.IPv4 == nil {
		return fmt.Errorf(
			"subnet2's IPv4Net is non-nil but gatewayIPs contains nil IPv4Gateway: " +
				"gateway IPs are inconsistent with subnet2 network")
	}
	if subnet2.GetNetwork().IPv6Net != nil && gatewayIPs.IPv6 == nil {
		return fmt.Errorf(
			"subnet2's IPv6Net is nil but gatewayIPs contains nil IPv6Gateway: " +
				"gateway IPs are inconsistent with subnet2 network")
	}

	err := r.AddRoute(subnet2.GetNetwork(), gatewayIPs, tunnelInterfaceName)
	if err != nil {
		return fmt.Errorf("failed to add route: %w", err)
	}
	return nil
}

func (r *RoutingSubnet) AddDefaultRoute(gatewayIPs netutils.DualStackAddress, tunnelInterfaceName string) error {
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
		ipv6DefaultRoute := &netlink.Route{
			Dst: &net.IPNet{
				IP:   net.IPv6zero,
				Mask: net.CIDRMask(0, 128),
			},
			Gw:        gw.IPv6,
			Table:     int(r.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv6DefaultRoute); err != nil {
			logger.DebugLogger().Printf("failed to execute: `ip -6 route add ::/0 table %d dev %s`",
				r.Vrf.Table, outIf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	if gw.IPv4 != nil {
		ipv4DefaultRoute := &netlink.Route{
			Dst: &net.IPNet{
				IP:   net.IPv4zero,
				Mask: net.CIDRMask(0, 32),
			},
			Gw:        gw.IPv4,
			Table:     int(r.Vrf.Table),
			LinkIndex: outIf.Attrs().Index,
		}
		if err = netlink.RouteAdd(ipv4DefaultRoute); err != nil {
			logger.DebugLogger().Printf("failed to execute: `ip route add 0.0.0.0/0 table %d dev %s`",
				r.Vrf.Table, outIf.Attrs().Name)
			return fmt.Errorf("failed to add route to app subnet in routing VRF: %w", err)
		}
	}

	return nil
}

func (r *RoutingSubnet) RemoveRoute(dst netutils.DualStackNetwork) error {
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
			logger.DebugLogger().Printf("failed to execute: `ip route del %s table %d`",
				dst.IPv4Net.String(), r.Vrf.Table)
			return fmt.Errorf("failed to delete IPv4 route to app subnet in routing VRF: %w", err)
		}
	}
	if dst.IPv6Net != nil {
		ipv6Route := &netlink.Route{
			Dst:   dst.IPv6Net,
			Table: int(r.Vrf.Table),
		}
		if err := netlink.RouteDel(ipv6Route); err != nil {
			logger.DebugLogger().Printf("failed to execute: `ip -6 route del %s table %d`",
				dst.IPv6Net.String(), r.Vrf.Table)
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
