package geneve

import (
	"errors"
	"fmt"
	"net"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	vrfutils "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type Tunnel struct {
	chainName   string
	ip4t        *iptables.IPTables
	ip6t        *iptables.IPTables
	tunnelIface string
	// vrfName is the VRF that egress routes are installed into, so that packets routed towards
	// this tunnel's destination node are looked up in the right routing table.
	vrfName string
	// endpoint is the remote node's real (underlay) address, used as the Geneve tunnel
	// destination for packets routed towards this node.
	endpoint net.IP
	// egress holds the destination network(s) currently routed towards this tunnel, if any.
	egress netutils.DualStackNetwork
}

func NewTunnel(
	loomNode *infrav1alpha1.LoomNode, ip4tables *iptables.IPTables, ip6tables *iptables.IPTables,
	tunnelIface string, vrfName string,
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
		vrfName:     vrfName,
	}
	if err = nodeTunnel.UpdateDestinationNode(loomNode); err != nil {
		return nil, fmt.Errorf("failed to update destination node: %v", err)
	}
	return nodeTunnel, nil
}

func (t *Tunnel) UpdateDestinationNode(loomNode *infrav1alpha1.LoomNode) (err error) {
	addr := netutils.DualStackAddress{
		IPv4: vrfutils.GetNodeIP(loomNode.Spec.Addresses, false),
	}
	if addr.IsEmpty() {
		return nil // Node is not yet ready
	}
	if addr.IPv4 != nil {
		err = t.ip4t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP4 iptables chain: %v", err)
		}
		err = t.ip4t.Insert("filter", t.chainName, 1,
			"-s", addr.IPv4.String(), "-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark))
		if err != nil {
			return fmt.Errorf("failed to insert mark rule: %v", err)
		}
		err = t.ip4t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
		}
		t.endpoint = addr.IPv4
	} else if addr.IPv6 != nil {
		err = t.ip6t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP6 iptables chain: %v", err)
		}
		err = t.ip6t.Insert("filter", t.chainName, 1,
			"-s", addr.IPv6.String(), "-j", "MARK", "--set-xmark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark))
		if err != nil {
			return fmt.Errorf("failed to insert mark rule: %v", err)
		}
		err = t.ip6t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP6 iptables chain: %v", err)
		}
		t.endpoint = addr.IPv6
	}
	// If egress routes were already installed, refresh them so they encapsulate towards the
	// (possibly new) endpoint address(es) we just resolved.
	if !t.egress.IsEmpty() {
		if err = t.AddEgressRoute(t.egress); err != nil {
			return fmt.Errorf("failed to refresh egress routes: %v", err)
		}
	}
	return nil
}

// AddEgressRoute installs (or updates) the routes that send traffic destined to dst towards this
// tunnel's destination node, with Geneve tunnel metadata (destination address and port) attached
// directly to the route via the kernel's lightweight tunnel (lwtunnel) encap. This is required
// because the shared tunnel interface is Flow-based: it carries no static remote endpoint of its
// own, so without this metadata attached to the route, outgoing packets are silently dropped by
// the Geneve driver instead of being encapsulated and sent out.
func (t *Tunnel) AddEgressRoute(dst netutils.DualStackNetwork) error {
	link, err := netlink.LinkByName(t.tunnelIface)
	if err != nil {
		return fmt.Errorf("failed to get tunnel link %s: %v", t.tunnelIface, err)
	}
	table, err := t.vrfTable()
	if err != nil {
		return fmt.Errorf("failed to resolve VRF %s: %v", t.vrfName, err)
	}
	if t.endpoint == nil {
		return fmt.Errorf("tunnel endpoint is not set, cannot add egress route")
	}
	var encap netlink.Encap
	if t.endpoint.To4() == nil {
		encap = &netlink.IP6tnlEncap{Dst: t.endpoint, Src: net.IPv6zero}
	} else {
		encap = &ipTunnelEncap{dst: t.endpoint}
	}
	if dst.IsEmpty() {
		return nil // Nothing to route, so nothing to do.
	}
	if dst.IPv4Net != nil {
		if err = t.replaceEgressRoute(link, table, dst.IPv4Net, encap); err != nil {
			return fmt.Errorf("failed to install IPv4 egress route: %v", err)
		}
		t.egress.IPv4Net = dst.IPv4Net
	}
	if dst.IPv6Net != nil {
		if err = t.replaceEgressRoute(link, table, dst.IPv6Net, encap); err != nil {
			return fmt.Errorf("failed to install IPv6 egress route: %v", err)
		}
		t.egress.IPv6Net = dst.IPv6Net
	}
	return nil
}

// RemoveEgressRoute removes routes previously installed by AddEgressRoute for dst.
func (t *Tunnel) RemoveEgressRoute(dst netutils.DualStackNetwork) error {
	if _, err := netlink.LinkByName(t.tunnelIface); err != nil {
		if errors.Is(err, netlink.LinkNotFoundError{}) {
			// The shared tunnel interface is gone, which takes every route bound to it down with
			// it, so there's nothing left to remove.
			t.egress = netutils.DualStackNetwork{}
			return nil
		}
		return fmt.Errorf("failed to get tunnel link %s: %v", t.tunnelIface, err)
	}
	table, err := t.vrfTable()
	if err != nil {
		return fmt.Errorf("failed to resolve VRF %s: %v", t.vrfName, err)
	}
	if dst.IPv4Net != nil && t.egress.IPv4Net != nil {
		if err = netlink.RouteDel(&netlink.Route{Dst: dst.IPv4Net, Table: table}); err != nil {
			return fmt.Errorf("failed to remove IPv4 egress route: %v", err)
		}
		t.egress.IPv4Net = nil
	}
	if dst.IPv6Net != nil && t.egress.IPv6Net != nil {
		if err = netlink.RouteDel(&netlink.Route{Dst: dst.IPv6Net, Table: table}); err != nil {
			return fmt.Errorf("failed to remove IPv6 egress route: %v", err)
		}
		t.egress.IPv6Net = nil
	}
	return nil
}

// vrfTable resolves the routing table ID backing this tunnel's VRF.
func (t *Tunnel) vrfTable() (int, error) {
	link, err := netlink.LinkByName(t.vrfName)
	if err != nil {
		return 0, fmt.Errorf("failed to get VRF link %s: %v", t.vrfName, err)
	}
	vrf, ok := link.(*netlink.Vrf)
	if !ok {
		return 0, fmt.Errorf("interface %s is not a VRF", t.vrfName)
	}
	return int(vrf.Table), nil
}

func (t *Tunnel) replaceEgressRoute(link netlink.Link, table int, dst *net.IPNet, encap netlink.Encap) error {
	route := &netlink.Route{
		Dst:       dst,
		Table:     table,
		LinkIndex: link.Attrs().Index,
		Encap:     encap,
	}
	if err := netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("failed to replace route: %v", err)
	}
	return nil
}

const (
	LWTUNNEL_IP6_UNSPEC = iota
	LWTUNNEL_IP6_ID
	LWTUNNEL_IP6_DST
	LWTUNNEL_IP6_SRC
	LWTUNNEL_IP6_HOPLIMIT
	LWTUNNEL_IP6_TC
	LWTUNNEL_IP6_FLAGS
	LWTUNNEL_IP6_PAD // not implemented
	LWTUNNEL_IP6_OPTS // not implemented
	__LWTUNNEL_IP6_MAX
)

const (
	LWTUNNEL_IP_UNSPEC = iota
	LWTUNNEL_IP_ID
	LWTUNNEL_IP_DST
	LWTUNNEL_IP_SRC
	LWTUNNEL_IP_TTL
	LWTUNNEL_IP_TOS
	LWTUNNEL_IP_FLAGS
	LWTUNNEL_IP_PAD
	LWTUNNEL_IP_OPTS
	__LWTUNNEL_IP_MAX
)

// The kernel's lwtunnel_ip_t (encap ip) and lwtunnel_ip6_t (encap ip6) netlink attribute enums
// assign the same ordinal values to their ID and DST attributes (only the address length in the
// DST payload differs between the two). vishvananda/netlink only exports these as the
// nl.LWTUNNEL_IP6_* constants (in nl/ip6tnl_linux.go) with no nl.LWTUNNEL_IP_* counterpart, so
// ipTunnelEncap below aliases them under IPv4-appropriate names rather than referencing the IP6
// constants directly.
const (
	lwtunnelIPID  = nl.LWTUNNEL_IP6_ID
	lwtunnelIPDst = nl.LWTUNNEL_IP6_DST
)

// ipTunnelEncap implements netlink.Encap for the kernel's IPv4 lightweight tunnel
// (LWTUNNEL_ENCAP_IP, i.e. `ip route ... encap ip id 0 dst <addr>`). vishvananda/netlink only
// ships the IPv6 variant (netlink.IP6tnlEncap).
type ipTunnelEncap struct {
	dst net.IP
}

func (e *ipTunnelEncap) Type() int {
	return nl.LWTUNNEL_ENCAP_IP
}

func (e *ipTunnelEncap) Decode([]byte) error {
	return fmt.Errorf("decoding ipTunnelEncap is not supported")
}

func (e *ipTunnelEncap) Encode() ([]byte, error) {
	dst := e.dst.To4()
	if dst == nil {
		return nil, fmt.Errorf("ipTunnelEncap destination %s is not a valid IPv4 address", e.dst)
	}
	native := nl.NativeEndian()

	id := make([]byte, 12)
	native.PutUint16(id, 12)
	native.PutUint16(id[2:], uint16(lwtunnelIPID))
	native.PutUint64(id[4:], 0)

	dstAttr := make([]byte, 4, 8)
	native.PutUint16(dstAttr, 8)
	native.PutUint16(dstAttr[2:], uint16(lwtunnelIPDst))
	dstAttr = append(dstAttr, dst...)

	return append(id, dstAttr...), nil
}

func (e *ipTunnelEncap) String() string {
	return fmt.Sprintf("id 0 dst %s", e.dst)
}

func (e *ipTunnelEncap) Equal(x netlink.Encap) bool {
	o, ok := x.(*ipTunnelEncap)
	if !ok {
		return false
	}
	return e.dst.Equal(o.dst)
}

func (t *Tunnel) Teardown() error {
	if err := t.RemoveEgressRoute(t.egress); err != nil {
		return fmt.Errorf("failed to remove egress routes: %v", err)
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
