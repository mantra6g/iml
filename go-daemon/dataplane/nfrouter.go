package dataplane

import (
	"fmt"
	"iml-daemon/config"

	"github.com/vishvananda/netlink"
)

func configureNFRouter(env *config.Environment) error {
	// Ensure the nfrouter interface does not exist
	nfrouter, err := netlink.LinkByName("nfrouter")
	if nfrouter != nil {
		return fmt.Errorf("NFRouter interface already exists: %w", err)
	}

	// Create the nfrouter bridge
	nfrouter = &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: "nfrouter",
			MTU:  1500,
		},
	}
	if err := netlink.LinkAdd(nfrouter); err != nil {
		return fmt.Errorf("failed to create nfrouter interface: %w", err)
	}

	// Get an IP from the Application domain for the nfrouter
	appIp, err := env.AppSIDAllocator.Next()
	if err != nil {
		return fmt.Errorf("failed to allocate IP from Application domain")
	}

	// Set it on the nfrouter
	addr, err := netlink.ParseAddr(appIp.String())
	if err != nil {
		return fmt.Errorf("failed to parse application domain address: %w", err)
	}
	if err := netlink.AddrAdd(nfrouter, addr); err != nil {
		return fmt.Errorf("failed to add application domain address to nfrouter interface: %w", err)
	}

	// Get an IP from the VNF domain for the nfrouter
	vnfIp, err := env.NFSIDAllocator.Next()
	if err != nil {
		return fmt.Errorf("failed to allocate IP from VNF domain")
	}

	// Set it on the nfrouter
	vnfAddr, err := netlink.ParseAddr(vnfIp.String())
	if err != nil {
		return fmt.Errorf("failed to parse VNF domain address: %w", err)
	}
	if err := netlink.AddrAdd(nfrouter, vnfAddr); err != nil {
		return fmt.Errorf("failed to add VNF domain address to nfrouter interface: %w", err)
	}

	// Set the interface up
	if err := netlink.LinkSetUp(nfrouter); err != nil {
		return fmt.Errorf("failed to set nfrouter interface up: %w", err)
	}

	return nil
}

func teardownNFRouter() error {
	// Find the nfrouter interface
	nfrouter, err := netlink.LinkByName("nfrouter")
	if err != nil {
		return fmt.Errorf("failed to find nfrouter interface: %w", err)
	}

	// Set the interface down
	if err := netlink.LinkSetDown(nfrouter); err != nil {
		return fmt.Errorf("failed to set nfrouter interface down: %w", err)
	}

	// Delete the nfrouter interface
	if err := netlink.LinkDel(nfrouter); err != nil {
		return fmt.Errorf("failed to delete nfrouter interface: %w", err)
	}

	return nil
}
