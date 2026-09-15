package geneve

import (
	"errors"
	"fmt"
	"net"

	vrfutils "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	corev1 "k8s.io/api/core/v1"
)

type Tunnel struct {
	chainName   string
	ip4t        *iptables.IPTables
	ip6t        *iptables.IPTables
	tunnelIface string
	// priority uniquely identifies this tunnel's tc filters on the shared tunnel interface, so
	// that different node tunnels never clobber each other's egress classification rules.
	priority uint16
	// endpoint is the remote node's real (underlay) address, used as the Geneve tunnel
	// destination for packets classified towards this node.
	endpoint netutils.DualStackAddress
	// egress holds the destination network(s) currently routed towards this tunnel via tc, if any.
	egress netutils.DualStackNetwork
}

func NewTunnel(
	node *corev1.Node, ip4tables *iptables.IPTables, ip6tables *iptables.IPTables,
	tunnelIface string, priority uint16,
) (*Tunnel, error) {
	chainName, err := vrfutils.GenerateRandomName(IPTablesSubchainPrefix, IPTablesSubchainRandChars)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chain name: %v", err)
	}
	err = ip4tables.NewChain("filter", chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to init IP4 iptables chain: %v", err)
	}
	err = ip6tables.NewChain("filter", chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to init IP6 iptables chain: %v", err)
	}
	err = ip4tables.InsertUnique("filter", IPTablesRootChainName, 1, "-j", chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to append rule to chain %s: %v", IPTablesRootChainName, err)
	}
	err = ip6tables.InsertUnique("filter", IPTablesRootChainName, 1, "-j", chainName)
	if err != nil {
		return nil, fmt.Errorf("failed to append rule to chain %s: %v", IPTablesRootChainName, err)
	}
	err = ip4tables.Append("filter", chainName, "-j", "RETURN")
	if err != nil {
		return nil, fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}
	err = ip6tables.Append("filter", chainName, "-j", "RETURN")
	if err != nil {
		return nil, fmt.Errorf("failed to append rule to IP6 iptables chain: %v", err)
	}
	nodeTunnel := &Tunnel{
		chainName:   chainName,
		ip4t:        ip4tables,
		ip6t:        ip6tables,
		tunnelIface: tunnelIface,
		priority:    priority,
	}
	if err = nodeTunnel.UpdateDestinationNode(node); err != nil {
		return nil, fmt.Errorf("failed to update destination node: %v", err)
	}
	return nodeTunnel, nil
}

func (t *Tunnel) UpdateDestinationNode(node *corev1.Node) (err error) {
	possibleAddrs := listAllInternalAndExternalIPAddresses(node)
	addr, err := netutils.ParseDualStackAddressListFromStrings(possibleAddrs)
	if err != nil {
		return fmt.Errorf("failed to parse node addresses: %v", err)
	}
	if addr.IsEmpty() {
		return nil // Node is not yet ready
	}
	if len(addr.IPv4Addresses) != 0 {
		err = t.ip4t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP4 iptables chain: %v", err)
		}
		for _, ip := range addr.IPv4Addresses {
			err = t.ip4t.Insert("filter", t.chainName, 1,
				"-s", ip.String(), "-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark))
			if err != nil {
				return fmt.Errorf("failed to insert mark rule: %v", err)
			}
		}
		err = t.ip4t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
		}
		t.endpoint.IPv4 = addr.IPv4Addresses[0]
	}
	if len(addr.IPv6Addresses) != 0 {
		err = t.ip6t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP6 iptables chain: %v", err)
		}
		for _, ip := range addr.IPv6Addresses {
			err = t.ip6t.Insert("filter", t.chainName, 1,
				"-s", ip.String(), "-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark))
			if err != nil {
				return fmt.Errorf("failed to insert mark rule: %v", err)
			}
		}
		err = t.ip6t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP6 iptables chain: %v", err)
		}
		t.endpoint.IPv6 = addr.IPv6Addresses[0]
	}
	// If egress tc routes were already installed, refresh them so they encapsulate towards the
	// (possibly new) endpoint address(es) we just resolved.
	if !t.egress.IsEmpty() {
		if err = t.AddEgressRoute(t.egress); err != nil {
			return fmt.Errorf("failed to refresh egress tc routes: %v", err)
		}
	}
	return nil
}

// AddEgressRoute installs (or updates) the tc rules that classify traffic destined to dst as
// belonging to this tunnel, and attach Geneve tunnel metadata (destination address and port) to
// it so the kernel knows how to encapsulate it. This is required because the shared tunnel
// interface is Flow-based: it carries no static remote endpoint of its own, so without this
// metadata attached by tc, outgoing packets are silently dropped by the Geneve driver instead of
// being encapsulated and sent out.
func (t *Tunnel) AddEgressRoute(dst netutils.DualStackNetwork) error {
	link, err := netlink.LinkByName(t.tunnelIface)
	if err != nil {
		return fmt.Errorf("failed to get tunnel link %s: %v", t.tunnelIface, err)
	}
	if dst.IPv4Net != nil {
		if t.endpoint.IPv4 == nil {
			return fmt.Errorf("no IPv4 endpoint address known yet for this tunnel's destination node")
		}
		if err = t.replaceEgressFilter(link, unix.ETH_P_IP, dst.IPv4Net, t.endpoint.IPv4); err != nil {
			return fmt.Errorf("failed to install IPv4 egress tc filter: %v", err)
		}
		t.egress.IPv4Net = dst.IPv4Net
	}
	if dst.IPv6Net != nil {
		if t.endpoint.IPv6 == nil {
			return fmt.Errorf("no IPv6 endpoint address known yet for this tunnel's destination node")
		}
		if err = t.replaceEgressFilter(link, unix.ETH_P_IPV6, dst.IPv6Net, t.endpoint.IPv6); err != nil {
			return fmt.Errorf("failed to install IPv6 egress tc filter: %v", err)
		}
		t.egress.IPv6Net = dst.IPv6Net
	}
	return nil
}

// RemoveEgressRoute removes tc rules previously installed by AddEgressRoute for dst.
func (t *Tunnel) RemoveEgressRoute(dst netutils.DualStackNetwork) error {
	link, err := netlink.LinkByName(t.tunnelIface)
	if err != nil {
		if errors.Is(err, netlink.LinkNotFoundError{}) {
			t.egress = netutils.DualStackNetwork{}
			return nil
		}
		return fmt.Errorf("failed to get tunnel link %s: %v", t.tunnelIface, err)
	}
	if dst.IPv4Net != nil && t.egress.IPv4Net != nil {
		if err = t.deleteEgressFilter(link, unix.ETH_P_IP); err != nil {
			return fmt.Errorf("failed to remove IPv4 egress tc filter: %v", err)
		}
		t.egress.IPv4Net = nil
	}
	if dst.IPv6Net != nil && t.egress.IPv6Net != nil {
		if err = t.deleteEgressFilter(link, unix.ETH_P_IPV6); err != nil {
			return fmt.Errorf("failed to remove IPv6 egress tc filter: %v", err)
		}
		t.egress.IPv6Net = nil
	}
	return nil
}

func (t *Tunnel) egressFilterAttrs(link netlink.Link, ethType uint16) netlink.FilterAttrs {
	return netlink.FilterAttrs{
		LinkIndex: link.Attrs().Index,
		Parent:    netlink.HANDLE_MIN_EGRESS,
		Priority:  t.priority,
		Protocol:  ethType,
		Handle:    1,
	}
}

func (t *Tunnel) replaceEgressFilter(link netlink.Link, ethType uint16, dst *net.IPNet, remote net.IP) error {
	// Let the kernel pick the source address it would normally use to reach remote, exactly like
	// it would for any other route, instead of hardcoding one of this node's own addresses.
	routes, err := netlink.RouteGet(remote)
	if err != nil || len(routes) == 0 || routes[0].Src == nil {
		return fmt.Errorf("failed to determine local source address to reach %s: %v", remote, err)
	}
	action := netlink.NewTunnelKeyAction()
	action.Action = netlink.TCA_TUNNEL_KEY_SET
	action.SrcAddr = routes[0].Src
	action.DstAddr = remote
	action.DestPort = TunnelPort
	filter := &netlink.Flower{
		FilterAttrs: t.egressFilterAttrs(link, ethType),
		EthType:     ethType,
		DestIP:      dst.IP,
		DestIPMask:  dst.Mask,
		Actions:     []netlink.Action{action},
	}
	if err = netlink.FilterReplace(filter); err != nil {
		return fmt.Errorf("failed to replace tc filter: %v", err)
	}
	return nil
}

func (t *Tunnel) deleteEgressFilter(link netlink.Link, ethType uint16) error {
	filter := &netlink.Flower{
		FilterAttrs: t.egressFilterAttrs(link, ethType),
	}
	if err := netlink.FilterDel(filter); err != nil {
		return fmt.Errorf("failed to delete tc filter: %v", err)
	}
	return nil
}

func listAllInternalAndExternalIPAddresses(node *corev1.Node) []string {
	addrs := make([]string, 0)
	for _, ip := range node.Status.Addresses {
		if ip.Type == corev1.NodeInternalIP || ip.Type == corev1.NodeExternalIP {
			addrs = append(addrs, ip.Address)
		}
	}
	return addrs
}

func (t *Tunnel) Teardown() error {
	if err := t.RemoveEgressRoute(t.egress); err != nil {
		return fmt.Errorf("failed to remove egress tc filters: %v", err)
	}
	if err := t.ip4t.ClearChain("filter", t.chainName); err != nil {
		return fmt.Errorf("failed to clear IP4 iptables chain: %v", err)
	}
	if err := t.ip6t.ClearChain("filter", t.chainName); err != nil {
		return fmt.Errorf("failed to clear IP6 iptables chain: %v", err)
	}
	if err := t.ip4t.DeleteIfExists("filter", IPTablesRootChainName, "-j", t.chainName); err != nil {
		return fmt.Errorf("failed to delete IP4 rules: %v", err)
	}
	if err := t.ip6t.DeleteIfExists("filter", IPTablesRootChainName, "-j", t.chainName); err != nil {
		return fmt.Errorf("failed to delete IP6 rules: %v", err)
	}
	if err := t.ip4t.DeleteChain("filter", t.chainName); err != nil {
		return fmt.Errorf("failed to delete IP4 iptables chain: %v", err)
	}
	if err := t.ip6t.DeleteChain("filter", t.chainName); err != nil {
		return fmt.Errorf("failed to delete IP6 iptables chain: %v", err)
	}
	return nil
}
