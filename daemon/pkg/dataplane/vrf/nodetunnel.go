package vrf

import (
	"errors"
	"fmt"
	"strconv"

	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	vrfutils "iml-daemon/pkg/dataplane/vrf/util"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/types"
)

const (
	GeneveTunnelVNI           = 121
	GeneveTunnelPort          = 6018
	IPTablesRootChainName     = "IML-TUNNEL"
	IPTablesSubchainPrefix    = "IML-TUNNEL"
	IPTablesSubchainRandChars = 6
	PacketAcceptedMark        = "0x100"
	MarkCleanupMask           = "0xFFFFFEFF"
)

type NodeTunnelManager interface {
	UpdateNodeTunnels(node *infrav1alpha1.LoomNode) error
	DeleteNodeTunnels(nodeID types.UID) error
	GetTunnelInterface(nodeID types.UID) (string, error)
	Close() error
}

type GeneveNodeTunnelManager struct {
	tunnelInterface string
	tunnels         map[types.UID]*GeneveNodeTunnel
	ip4t            *iptables.IPTables
	ip6t            *iptables.IPTables
}

func NewGeneveNodeTunnelManager() (NodeTunnelManager, error) {
	ifName, err := vrfutils.GenerateRandomName("imltun", 4)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tunnel interface name: %v", err)
	}
	ip4t, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to init iptables for IPv4 address family: %v", err)
	}
	ip6t, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv6))
	if err != nil {
		return nil, fmt.Errorf("failed to init iptables for IPv6 address family: %v", err)
	}
	v4ChainExists, err := ip4t.ChainExists("filter", IPTablesRootChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to check whether IP4 chain exists: %v", err)
	}
	v6ChainExists, err := ip6t.ChainExists("filter", IPTablesRootChainName)
	if err != nil {
		return nil, fmt.Errorf("failed to check whether IP6 chain exists: %v", err)
	}
	if !v4ChainExists {
		err = ip4t.NewChain("filter", IPTablesRootChainName)
		if err != nil {
			return nil, fmt.Errorf("failed to create IP4 iptables chain: %v", err)
		}
	}
	// Accepts marked packets
	err = ip4t.AppendUnique("filter", IPTablesRootChainName,
		"-m", "mark", "--mark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark),
		"-j", "RETURN")
	if err != nil {
		return nil, fmt.Errorf("failed to append return rule in %s chain: %v", IPTablesRootChainName, err)
	}
	// Drops anything unmarked
	err = ip4t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "DROP")
	if err != nil {
		return nil, fmt.Errorf("failed to append drop rule in %s chain: %v", IPTablesRootChainName, err)
	}
	if !v6ChainExists {
		err = ip6t.NewChain("filter", IPTablesRootChainName)
		if err != nil {
			return nil, fmt.Errorf("failed to create IP6 iptables chain: %v", err)
		}
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-m", "mark", "--mark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark),
		"-j", "RETURN")
	if err != nil {
		return nil, fmt.Errorf("failed to append return rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "MARK", "--and-mark", MarkCleanupMask)
	if err != nil {
		return nil, fmt.Errorf("failed to append mark cleanup rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "DROP")
	if err != nil {
		return nil, fmt.Errorf("failed to append drop rule in %s chain: %v", IPTablesRootChainName, err)
	}
	if err = ip4t.InsertUnique("filter", "INPUT", 1,
		"-p", "udp", "--dport", strconv.Itoa(GeneveTunnelPort), "-j", IPTablesRootChainName); err != nil {
		return nil, fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}
	if err = ip6t.InsertUnique("filter", "INPUT", 1,
		"-p", "udp", "--dport", strconv.Itoa(GeneveTunnelPort), "-j", IPTablesRootChainName); err != nil {
		return nil, fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}

	tun := &netlink.Geneve{
		LinkAttrs: netlink.LinkAttrs{
			Name: ifName,
		},
		Dport:     GeneveTunnelPort,
		FlowBased: true,
	}
	if err = netlink.LinkAdd(tun); err != nil {
		return nil, fmt.Errorf("failed to add Geneve tunnel: %v", err)
	}
	if err = netlink.LinkSetUp(tun); err != nil {
		return nil, fmt.Errorf("failed to set up Geneve tunnel: %v", err)
	}

	return &GeneveNodeTunnelManager{
		tunnelInterface: ifName,
		tunnels:         make(map[types.UID]*GeneveNodeTunnel),
		ip4t:            ip4t,
		ip6t:            ip6t,
	}, nil
}

func (mgr *GeneveNodeTunnelManager) Close() error {
	tunInterface, err := netlink.LinkByName(mgr.tunnelInterface)
	if err != nil && errors.Is(err, netlink.LinkNotFoundError{}) {
		// Interface is missing, skip teardown
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to lookup interface %s: %v", mgr.tunnelInterface, err)
	}
	err = netlink.LinkDel(tunInterface)
	if err != nil {
		return fmt.Errorf("failed to delete interface %s: %v", mgr.tunnelInterface, err)
	}
	for _, tunnel := range mgr.tunnels {
		if err := tunnel.Teardown(); err != nil {
			return fmt.Errorf("error while tearing down Geneve tunnel: %v", err)
		}
	}
	return nil
}

func (mgr *GeneveNodeTunnelManager) UpdateNodeTunnels(node *infrav1alpha1.LoomNode) error {
	tunnel, exists := mgr.tunnels[node.UID]
	if !exists {
		if err := tunnel.UpdateDestinationNode(node); err != nil {
			return fmt.Errorf("failed to update destination node %s: %v", node.Name, err)
		}
		return nil
	}
	tunnel, err := NewGeneveNodeTunnel(node, mgr.ip4t, mgr.ip6t)
	if err != nil {
		return fmt.Errorf("failed to create Geneve tunnel for node %s: %v", node.Name, err)
	}
	mgr.tunnels[node.UID] = tunnel
	return nil
}

func (mgr *GeneveNodeTunnelManager) DeleteNodeTunnels(nodeID types.UID) error {
	tunnel, exists := mgr.tunnels[nodeID]
	if !exists {
		return nil // Tunnel already doesn't exist, skip
	}
	if err := tunnel.Teardown(); err != nil {
		return fmt.Errorf("error while tearing down Geneve tunnel for node %s: %v", nodeID, err)
	}
	delete(mgr.tunnels, nodeID)
	return nil
}

func (mgr *GeneveNodeTunnelManager) GetTunnelInterface(_ types.UID) (string, error) {
	return mgr.tunnelInterface, nil
}

type GeneveNodeTunnel struct {
	chainName   string
	lastVersion string
	ip4t        *iptables.IPTables
	ip6t        *iptables.IPTables
}

func NewGeneveNodeTunnel(
	node *infrav1alpha1.LoomNode, ip4tables *iptables.IPTables, ip6tables *iptables.IPTables,
) (*GeneveNodeTunnel, error) {
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
	nodeTunnel := &GeneveNodeTunnel{
		chainName: chainName,
		ip4t:      ip4tables,
		ip6t:      ip6tables,
	}
	if err = nodeTunnel.UpdateDestinationNode(node); err != nil {
		return nil, fmt.Errorf("failed to update destination node: %v", err)
	}
	return nodeTunnel, nil
}

func (t *GeneveNodeTunnel) UpdateDestinationNode(node *infrav1alpha1.LoomNode) (err error) {
	if t.lastVersion >= node.ResourceVersion {
		return // skip
	}
	defer func() {
		if err == nil {
			t.lastVersion = node.ResourceVersion
		}
	}()
	addr, err := vrfutils.ParseDualStackAddressFromStrings(node.Status.TransportIPs)
	if err != nil {
		return fmt.Errorf("failed to parse node addresses: %v", err)
	}
	cidrs, err := vrfutils.ParseDualStackNetworkFromStrings(node.Spec.TunnelCIDRs)
	if err != nil {
		return fmt.Errorf("failed to parse node CIDRs: %v", err)
	}
	if addr.IsEmpty() || cidrs.IsEmpty() {
		return nil // Node is not yet ready
	}
	if cidrs.IPv4Net != nil {
		err = t.ip4t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP4 iptables chain: %v", err)
		}
		err = t.ip4t.Insert("filter", t.chainName, 1,
			"-s", cidrs.IPv4Net.String(), "-j", "MARK", "--or-mark", PacketAcceptedMark)
		if err != nil {
			return fmt.Errorf("failed to insert mark rule: %v", err)
		}
	}
	if cidrs.IPv6Net != nil {
		err = t.ip6t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP6 iptables chain: %v", err)
		}
		err = t.ip6t.Insert("filter", t.chainName, 1,
			"-s", cidrs.IPv6Net.String(), "-j", "mark", "--or-mark", PacketAcceptedMark)
		if err != nil {
			return fmt.Errorf("failed to insert mark rule: %v", err)
		}
	}
	return nil
}

func (t *GeneveNodeTunnel) Teardown() error {
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
