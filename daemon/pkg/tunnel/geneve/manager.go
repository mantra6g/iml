package geneve

import (
	"errors"
	"fmt"
	"strconv"

	vrfutils "iml-daemon/pkg/dataplane/vrf/util"
	"iml-daemon/pkg/tunnel"

	"github.com/coreos/go-iptables/iptables"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
)

const (
	TunnelPort                = 6018
	IPTablesRootChainName     = "IML-TUNNEL"
	IPTablesSubchainPrefix    = "IML-TUNNEL"
	IPTablesSubchainRandChars = 6
	PacketAcceptedMark        = "0x100"
	MarkCleanupMask           = "0xFFFFFEFF"
)

type NodeName = string

type TunnelManager struct {
	tunnelInterface string
	tunnels         map[NodeName]*Tunnel
	ip4t            *iptables.IPTables
	ip6t            *iptables.IPTables
	log 				    logr.Logger
}

func NewTunnelManager(logger logr.Logger) (tunnel.Manager, error) {
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
		"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName); err != nil {
		return nil, fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}
	if err = ip6t.InsertUnique("filter", "INPUT", 1,
		"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName); err != nil {
		return nil, fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}

	tun := &netlink.Geneve{
		LinkAttrs: netlink.LinkAttrs{
			Name: ifName,
		},
		Dport:     TunnelPort,
		FlowBased: true,
	}
	if err = netlink.LinkAdd(tun); err != nil {
		return nil, fmt.Errorf("failed to add Geneve tunnel: %v", err)
	}
	if err = netlink.LinkSetUp(tun); err != nil {
		return nil, fmt.Errorf("failed to set up Geneve tunnel: %v", err)
	}

	return &TunnelManager{
		tunnelInterface: ifName,
		tunnels:         make(map[NodeName]*Tunnel),
		ip4t:            ip4t,
		ip6t:            ip6t,
	}, nil
}

func (mgr *TunnelManager) Close() error {
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
	for _, tun := range mgr.tunnels {
		if err := tun.Teardown(); err != nil {
			return fmt.Errorf("error while tearing down Geneve tunnel: %v", err)
		}
	}
	return nil
}

func (mgr *TunnelManager) UpdateNodeTunnels(node *corev1.Node) error {
	tun, exists := mgr.tunnels[node.Name]
	if !exists {
		if err := tun.UpdateDestinationNode(node); err != nil {
			return fmt.Errorf("failed to update destination node %s: %v", node.Name, err)
		}
		return nil
	}
	tun, err := NewTunnel(node, mgr.ip4t, mgr.ip6t)
	if err != nil {
		return fmt.Errorf("failed to create Geneve tunnel for node %s: %v", node.Name, err)
	}
	mgr.tunnels[node.Name] = tun
	return nil
}

func (mgr *TunnelManager) DeleteNodeTunnels(nodeName string) error {
	tun, exists := mgr.tunnels[nodeName]
	if !exists {
		return nil // Tunnel already doesn't exist, skip
	}
	if err := tun.Teardown(); err != nil {
		return fmt.Errorf("error while tearing down Geneve tunnel for node %s: %v", nodeName, err)
	}
	delete(mgr.tunnels, nodeName)
	return nil
}

func (mgr *TunnelManager) GetTunnelInterface(_ string) (string, error) {
	return mgr.tunnelInterface, nil
}
