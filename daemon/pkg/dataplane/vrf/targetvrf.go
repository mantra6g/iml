package vrf

import (
	"errors"
	"fmt"
	"net"

	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// physicalLinkType is the netlink type reported for interfaces without a more specific kind, i.e. NICs.
const physicalLinkType = "device"

// TargetVRF is a host VRF attaching a P4Target to the routing VRF. The daemon owns the VRF and the veth
// pair connecting it to the routing VRF; the target's plugin owns everything else inside it.
type TargetVRF struct {
	Vrf    *netlink.Vrf
	NfCIDR netutils.DualStackNetwork

	// Tunnel is the VRF-side end of the veth pair connecting the VRF to the routing VRF.
	Tunnel      string
	TunnelAddrs netutils.DualStackAddress

	// RouterTunnel is the routing-VRF-side end of the veth pair.
	RouterTunnel      string
	RouterTunnelAddrs netutils.DualStackAddress

	Log logr.Logger
}

var _ Subnet = &TargetVRF{}

// TargetVRFName returns the name of the VRF device of a target VRF bound to tableID.
func TargetVRFName(tableID uint32) string {
	return fmt.Sprintf("tvrf-%d", tableID)
}

// NewTargetVRF creates the VRF device bound to tableID and installs an unreachable route for nfCIDR in it,
// so that addresses of the NfCIDR not programmed by the plugin are dropped instead of being sent back
// to the routing VRF (which would send them back here until their hop limit expires).
func NewTargetVRF(logger logr.Logger, tableID uint32, nfCIDR netutils.DualStackNetwork) (t *TargetVRF, err error) {
	if nfCIDR.IsEmpty() {
		return nil, fmt.Errorf("nf CIDR cannot be empty")
	}
	name := TargetVRFName(tableID)
	// A VRF left behind by a previous run that exited without tearing down would make LinkAdd fail.
	if stale, lookupErr := netlink.LinkByName(name); lookupErr == nil {
		if staleVrf, ok := stale.(*netlink.Vrf); ok {
			logger.Info("Deleting stale target VRF", "vrf", name)
			(&TargetVRF{Vrf: staleVrf, Log: logger}).Teardown()
		}
	}

	t = &TargetVRF{NfCIDR: nfCIDR, Log: logger}
	vrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: name},
		Table:     tableID,
	}
	if err = netlink.LinkAdd(vrf); err != nil {
		return nil, fmt.Errorf("failed to add target VRF %s: %w", name, err)
	}
	t.Vrf = vrf
	defer func() {
		if err != nil {
			t.Teardown()
		}
	}()
	if err = netlink.LinkSetUp(vrf); err != nil {
		return nil, fmt.Errorf("failed to set up target VRF %s: %w", name, err)
	}
	for _, dst := range []*net.IPNet{nfCIDR.IPv6Net, nfCIDR.IPv4Net} {
		if dst == nil {
			continue
		}
		route := &netlink.Route{
			Dst:   dst,
			Table: int(tableID),
			Type:  unix.RTN_UNREACHABLE,
		}
		if err = netlink.RouteReplace(route); err != nil {
			return nil, fmt.Errorf("failed to add unreachable route for %s in target VRF %s: %w", dst, name, err)
		}
	}
	return t, nil
}

// AddDefaultRoute routes everything not handled inside the VRF back to the routing VRF through the tunnel.
// Only the families for which the routing-VRF side of the tunnel has an address are routed.
func (t *TargetVRF) AddDefaultRoute() error {
	tun, err := netlink.LinkByName(t.Tunnel)
	if err != nil {
		return fmt.Errorf("failed to get tunnel %s: %w", t.Tunnel, err)
	}
	defaults := []struct {
		dst *net.IPNet
		gw  net.IP
	}{
		{&net.IPNet{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}, t.RouterTunnelAddrs.IPv6},
		{&net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, t.RouterTunnelAddrs.IPv4},
	}
	for _, d := range defaults {
		if d.gw == nil {
			continue
		}
		route := &netlink.Route{
			Dst:       d.dst,
			Gw:        d.gw,
			Table:     int(t.Vrf.Table),
			LinkIndex: tun.Attrs().Index,
		}
		if err = netlink.RouteReplace(route); err != nil {
			return fmt.Errorf("failed to add default route via %s in target VRF %s: %w", d.gw, t.Vrf.Name, err)
		}
	}
	return nil
}

// Teardown deletes the VRF together with everything wired to it, and flushes its routing table.
// Every interface whose chain of masters leads to the VRF (including the plugin's, and e.g. the ports
// of a bridge inside the VRF) is deleted if virtual, or released if physical.
// Errors are logged and teardown carries on, so that as much as possible is removed.
func (t *TargetVRF) Teardown() {
	if t == nil || t.Vrf == nil {
		return
	}
	logger := t.Log.WithValues("vrf", t.Vrf.Name, "table", t.Vrf.Table)

	vrf, err := netlink.LinkByName(t.Vrf.Name)
	if err == nil {
		links, err := netlink.LinkList()
		if err != nil {
			logger.Error(err, "failed to list links. Interfaces enslaved to the VRF may be left behind")
		}
		for _, link := range vrfDescendants(vrf.Attrs().Index, links) {
			if link.Type() == physicalLinkType {
				logger.Info("Releasing physical interface from target VRF", "interface", link.Attrs().Name)
				err = netlink.LinkSetNoMaster(link)
			} else {
				logger.Info("Deleting interface enslaved to target VRF", "interface", link.Attrs().Name)
				err = netlink.LinkDel(link)
			}
			// A veth's peer disappears together with it, so it may already be gone.
			if err != nil && !errors.Is(err, unix.ENODEV) {
				logger.Error(err, "failed to remove interface from target VRF", "interface", link.Attrs().Name)
			}
		}
		if err = netlink.LinkDel(vrf); err != nil {
			logger.Error(err, "failed to delete target VRF")
		}
	} else if !errors.As(err, &netlink.LinkNotFoundError{}) {
		logger.Error(err, "failed to get target VRF")
	}

	// The kernel never deletes a routing table: routes not bound to a deleted device (e.g. unreachable
	// routes) would stay and leak into whatever VRF gets bound to this table next.
	if err = flushTable(int(t.Vrf.Table)); err != nil {
		logger.Error(err, "failed to flush target VRF table")
	}
}

// vrfDescendants returns every link whose chain of masters leads to the link with index vrfIndex, e.g. the
// VRF's direct slaves, the ports of a bridge enslaved to the VRF, the members of a bond that is one of
// those ports, and so on. Links are returned deepest first, so that each one is handled before its master.
func vrfDescendants(vrfIndex int, links []netlink.Link) []netlink.Link {
	children := make(map[int][]netlink.Link)
	for _, link := range links {
		if master := link.Attrs().MasterIndex; master != 0 {
			children[master] = append(children[master], link)
		}
	}
	// Breadth-first walk from the VRF. A device has a single master and the kernel forbids master
	// loops, but visited guards against a malformed listing anyway.
	var ordered []netlink.Link
	visited := map[int]bool{vrfIndex: true}
	queue := []int{vrfIndex}
	for len(queue) > 0 {
		master := queue[0]
		queue = queue[1:]
		for _, child := range children[master] {
			index := child.Attrs().Index
			if visited[index] {
				continue
			}
			visited[index] = true
			ordered = append(ordered, child)
			queue = append(queue, index)
		}
	}
	return reversed(ordered)
}

// flushTable deletes every IPv4 and IPv6 route in the given routing table.
func flushTable(table int) error {
	var errs []error
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to list routes of table %d: %w", table, err))
			continue
		}
		for i := range routes {
			err = netlink.RouteDel(&routes[i])
			if err != nil && !errors.Is(err, unix.ESRCH) {
				errs = append(errs, fmt.Errorf("failed to delete route %s from table %d: %w", routes[i], table, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (t *TargetVRF) GetNetwork() netutils.DualStackNetwork {
	return t.NfCIDR
}

func (t *TargetVRF) GetGateway() netutils.DualStackAddress {
	return t.RouterTunnelAddrs
}

func (t *TargetVRF) GetStack() StackType {
	switch {
	case t.NfCIDR.IPv4Net != nil && t.NfCIDR.IPv6Net != nil:
		return DualStack
	case t.NfCIDR.IPv4Net != nil:
		return IPv4Only
	case t.NfCIDR.IPv6Net != nil:
		return IPv6Only
	}
	return UnknownStack
}

func (t *TargetVRF) GetVRFName() string {
	return t.Vrf.Name
}

func (t *TargetVRF) GetVRFTable() uint32 {
	return t.Vrf.Table
}

func (t *TargetVRF) SetTunnel(tunnel string) {
	t.Tunnel = tunnel
}
