package router

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type NFRouter struct {
	link netlink.Link
}

func newNFRouter(appIP *net.IPNet, vnfIP *net.IPNet) (*NFRouter, error) {
	// If the nfrouter interface already exists, remove it
	nfrouter, _ := netlink.LinkByName("nfrouter")
	if nfrouter != nil {
		if err := netlink.LinkDel(nfrouter); err != nil {
			return nil, fmt.Errorf("failed to delete existing nfrouter interface: %w", err)
		}
	}

	// Create the nfrouter bridge
	// ip link add name nfrouter type bridge
	nfrouter = &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "nfrouter",
			MTU:  1500,
		},
	}
	if err := netlink.LinkAdd(nfrouter); err != nil {
		return nil, fmt.Errorf("failed to create nfrouter interface: %w", err)
	}

	// Set the VNF domain address on the nfrouter
	vnfAddr, err := netlink.ParseAddr(vnfIP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse VNF domain address: %w", err)
	}
	if err := netlink.AddrAdd(nfrouter, vnfAddr); err != nil {
		return nil, fmt.Errorf("failed to add VNF domain address to nfrouter interface: %w", err)
	}

	// Set the interface up
	if err := netlink.LinkSetUp(nfrouter); err != nil {
		return nil, fmt.Errorf("failed to set nfrouter interface up: %w", err)
	}

	return &NFRouter{
		link: nfrouter,
	}, nil
}

func (r *NFRouter) AddIP(ip net.IP, network net.IPNet) error {
	if !network.Contains(ip) {
		return fmt.Errorf("IP %s is not in the network %s", ip.String(), network.String())
	}

	maskSize, _ := network.Mask.Size()
	addr, err := netlink.ParseAddr(fmt.Sprintf("%s/%d", ip.String(), maskSize))
	if err != nil {
		return fmt.Errorf("failed to parse IP address: %w", err)
	}
	if err := netlink.AddrAdd(r.link, addr); err != nil {
		return fmt.Errorf("failed to add IP address to nfrouter: %w", err)
	}
	return nil
}

// func (r *NFRouter) AddRoute(table uint32, srcIP string, dstIP string, sids []net.IP) error {
// 	// Parse the source and destination IP addresses
// 	srcAddr, err := netlink.ParseAddr(srcIP)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse source IP address: %w", err)
// 	}
// 	dstAddr, err := netlink.ParseAddr(dstIP)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse destination IP address: %w", err)
// 	}

// 	family := nl.GetIPFamily(srcAddr.IP)
// 	route := &netlink.Route{
// 		Dst:       dstAddr.IPNet,
// 		LinkIndex: r.link.Attrs().Index,
// 		Encap:     &netlink.SEG6Encap{
// 			Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
// 			Segments: sids,
// 		},
// 		Family: family,
// 		Table:  int(table),
// 	}

// 	if err := netlink.RouteAdd(route); err != nil {
// 		return fmt.Errorf("failed to add route from %s to %s via %s: %w", srcAddr.IP.String(), dstAddr.IPNet.String(), sids, err)
// 	}

// 	return nil
// }

func (r *NFRouter) AddRoute(table uint32, dstNet string, sids []net.IP) error {
	// Parse the destination network
	dst, err := netlink.ParseAddr(dstNet)
	if err != nil {
		return fmt.Errorf("failed to parse destination network: %w", err)
	}

	family := nl.GetIPFamily(dst.IP)
	route := &netlink.Route{
		Dst:       dst.IPNet,
		LinkIndex: r.link.Attrs().Index,
		Encap:     &netlink.SEG6Encap{
			Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
			Segments: sids,
		},
		Family: family,
		Table:  int(table),
	}

	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add route in table %d to %s with segs %s: %w", table, dst.IPNet.String(), sids, err)
	}

	return nil
}

func (r *NFRouter) RemoveRoute(srcIP string, dstIP string) error {
	// Parse the source and destination IP addresses
	srcAddr, err := netlink.ParseAddr(srcIP)
	if err != nil {
		return fmt.Errorf("failed to parse source IP address: %w", err)
	}
	dstAddr, err := netlink.ParseAddr(dstIP)
	if err != nil {
		return fmt.Errorf("failed to parse destination IP address: %w", err)
	}

	route := &netlink.Route{
		Dst:       dstAddr.IPNet,
		Gw:        srcAddr.IP,
		LinkIndex: r.link.Attrs().Index,
	}

	err = netlink.RouteDel(route)
	if err != nil {
		return fmt.Errorf("failed to delete route from %s to %s: %w", srcIP, dstIP, err)
	}

	return nil
}

func (r *NFRouter) Teardown() error {
	// Tear down the nfrouter interface
	if err := netlink.LinkDel(r.link); err != nil {
		return fmt.Errorf("failed to delete nfrouter interface: %w", err)
	}
	return nil
}