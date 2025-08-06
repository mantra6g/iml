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

func newNFRouter(appIP string, vnfIP string) (*NFRouter, error) {
	// Ensure the nfrouter interface does not exist
	nfrouter, err := netlink.LinkByName("nfrouter")
	if nfrouter != nil {
		return nil, fmt.Errorf("NFRouter interface already exists: %w", err)
	}

	// Create the nfrouter bridge
	nfrouter = &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "nfrouter",
			MTU:  1500,
		},
	}
	if err := netlink.LinkAdd(nfrouter); err != nil {
		return nil, fmt.Errorf("failed to create nfrouter interface: %w", err)
	}

	// Set the APP domain address on the nfrouter
	addr, err := netlink.ParseAddr(appIP)
	if err != nil {
		return nil, fmt.Errorf("failed to parse application domain address: %w", err)
	}
	if err := netlink.AddrAdd(nfrouter, addr); err != nil {
		return nil, fmt.Errorf("failed to add application domain address to nfrouter interface: %w", err)
	}

	// Set the VNF domain address on the nfrouter
	vnfAddr, err := netlink.ParseAddr(vnfIP)
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

func (r *NFRouter) AddRoute(srcIP string, dstIP string, sids []net.IP) error {
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
		Encap:     &netlink.SEG6Encap{
			Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
			Segments: sids,
		},
	}

	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add route from %s to %s: %w", srcIP, dstIP, err)
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