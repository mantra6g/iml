package geneve

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"
	"github.com/mantra6g/iml/daemon/pkg/tunnel"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
)

const (
	TunnelName                = "imlgnv0"
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
	log             logr.Logger
	vrfName         string
}

func NewTunnelManager(logger logr.Logger, vrfName string) (tunnel.Manager, error) {
	ip4t, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to init iptables for IPv4 address family: %v", err)
	}
	ip6t, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv6))
	if err != nil {
		return nil, fmt.Errorf("failed to init iptables for IPv6 address family: %v", err)
	}
	err = ensureIptables(ip4t, ip6t)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure iptables: %v", err)
	}
	vrf, err := netlink.LinkByName(vrfName)
	if err != nil {
		return nil, fmt.Errorf("failed to get VRF link %s: %v", vrfName, err)
	}
	err = ensureTunnel(TunnelName, TunnelPort, vrf)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure tunnel: %v", err)
	}
	return &TunnelManager{
		tunnelInterface: TunnelName,
		tunnels:         make(map[NodeName]*Tunnel),
		ip4t:            ip4t,
		ip6t:            ip6t,
		log:             logger,
		vrfName:         vrfName,
	}, nil
}

func ensureIptables(ip4t, ip6t *iptables.IPTables) error {
	// Delete leftover data
	if err := deleteChains(ip4t, ip6t); err != nil {
		return fmt.Errorf("failed to delete leftover iptables chains: %v", err)
	}
	err := ip4t.NewChain("filter", IPTablesRootChainName)
	if err != nil {
		return fmt.Errorf("failed to create %s chain in IP4 iptables: %v", IPTablesRootChainName, err)
	}
	err = ip6t.NewChain("filter", IPTablesRootChainName)
	if err != nil {
		return fmt.Errorf("failed to create %s chain in IP6 iptables: %v", IPTablesRootChainName, err)
	}
	// Accepts marked packets
	err = ip4t.AppendUnique("filter", IPTablesRootChainName,
		"-m", "mark", "--mark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark),
		"-j", "RETURN")
	if err != nil {
		return fmt.Errorf("failed to append return rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip4t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "MARK", "--set-xmark", fmt.Sprintf("0x0/%s", PacketAcceptedMark))
	if err != nil {
		return fmt.Errorf("failed to append mark cleanup rule in %s chain: %v", IPTablesRootChainName, err)
	}
	// Drops anything unmarked
	err = ip4t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "DROP")
	if err != nil {
		return fmt.Errorf("failed to append drop rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-m", "mark", "--mark", fmt.Sprintf("%s/%s", PacketAcceptedMark, PacketAcceptedMark),
		"-j", "RETURN")
	if err != nil {
		return fmt.Errorf("failed to append return rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "MARK", "--set-xmark", fmt.Sprintf("0x0/%s", PacketAcceptedMark))
	if err != nil {
		return fmt.Errorf("failed to append mark cleanup rule in %s chain: %v", IPTablesRootChainName, err)
	}
	err = ip6t.AppendUnique("filter", IPTablesRootChainName,
		"-j", "DROP")
	if err != nil {
		return fmt.Errorf("failed to append drop rule in %s chain: %v", IPTablesRootChainName, err)
	}
	if err = ip4t.InsertUnique("filter", "INPUT", 1,
		"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName); err != nil {
		return fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}
	if err = ip6t.InsertUnique("filter", "INPUT", 1,
		"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName); err != nil {
		return fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
	}
	return nil
}

func ensureTunnel(name string, port uint16, vrf netlink.Link) error {
	tunLink, err := netlink.LinkByName(name)
	if err != nil {
		tunLink, err = createTunnel(name, port, vrf)
		if err != nil {
			return fmt.Errorf("failed to create tunnel: %v", err)
		}
	}
	tun, ok := tunLink.(*netlink.Geneve)
	if !ok {
		return fmt.Errorf("a tunnel with the name %s already exists but is not a Geneve tunnel", name)
	}
	if tun.Dport != port || tun.FlowBased != true || tun.InnerProtoInherit != true {
		err = netlink.LinkDel(tunLink)
		if err != nil {
			return fmt.Errorf("failed to delete tunnel: %v", err)
		}
		tunLink, err = createTunnel(name, port, vrf)
		tun, _ = tunLink.(*netlink.Geneve)
	}
	if err = netlink.LinkSetUp(tun); err != nil {
		return fmt.Errorf("failed to set up Geneve tunnel: %v", err)
	}
	return nil
}

func createTunnel(name string, port uint16, vrf netlink.Link) (netlink.Link, error) {
	tun := &netlink.Geneve{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
		Dport: port,
		// FlowBased makes this a single "external"/metadata-driven device shared by every remote
		// node, instead of one device per fixed remote endpoint: the actual per-packet destination
		// and VNI are attached by the tc rules in Tunnel.AddEgressRoute.
		FlowBased: true,
		// InnerProtoInherit makes this an L3-only tunnel (no inner Ethernet header, the Geneve
		// protocol-type field is taken from the inner packet itself). Without it, the device would
		// carry full inner Ethernet frames and every route towards a remote node's CIDR would need a
		// resolvable neighbor (ARP/NDP) entry for its gateway address, which nothing in this overlay
		// ever provides — packets would stall in the neighbor-resolution queue and never reach the
		// tc egress filters at all.
		InnerProtoInherit: true,
	}
	if err := netlink.LinkAdd(tun); err != nil {
		return nil, fmt.Errorf("failed to add Geneve tunnel: %v", err)
	}
	if err := netlink.LinkSetMaster(tun, vrf); err != nil {
		return nil, fmt.Errorf("failed to set Geneve tunnel master: %v", err)
	}
	return tun, nil
}

func deleteChains(ip4t, ip6t *iptables.IPTables) error {
	exists, err := ip4t.ChainExists("filter", IPTablesRootChainName)
	if err != nil {
		return err
	}
	if exists {
		err := ip4t.DeleteIfExists("filter", "INPUT",
			"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName)
		if err != nil {
			return err
		}
		err = ip4t.ClearAndDeleteChain("filter", IPTablesRootChainName)
		if err != nil {
			return err
		}
	}
	exists, err = ip6t.ChainExists("filter", IPTablesRootChainName)
	if err != nil {
		return err
	}
	if exists {
		err = ip6t.DeleteIfExists("filter", "INPUT",
			"-p", "udp", "--dport", strconv.Itoa(TunnelPort), "-j", IPTablesRootChainName)
		if err != nil {
			return err
		}
		err = ip6t.ClearAndDeleteChain("filter", IPTablesRootChainName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (mgr *TunnelManager) Shutdown(ctx context.Context) error {
	mgr.log.V(1).Info("Closing tunnel manager")
	err := deleteChains(mgr.ip4t, mgr.ip6t)
	if err != nil {
		mgr.log.Error(err, "Failed to delete iptables chains during shutdown")
	}
	tunInterface, err := netlink.LinkByName(mgr.tunnelInterface)
	if err != nil && errors.Is(err, netlink.LinkNotFoundError{}) {
		// Interface is missing, skip teardown
		mgr.log.V(1).Info("TunnelManager already closed")
	} else if err != nil {
		mgr.log.Error(err, "Failed to get handle of tunnel interface", "interface", mgr.tunnelInterface)
	} else {
		err = netlink.LinkDel(tunInterface)
		if err != nil {
			mgr.log.Error(err, "Failed to delete tunnel interface", "interface", mgr.tunnelInterface)
		}
	}
	for _, tun := range mgr.tunnels {
		if err := tun.Teardown(); err != nil {
			mgr.log.Error(err, "Failed to tear down tunnel chain", "chain", tun.chainName)
		}
	}
	return nil
}

func (mgr *TunnelManager) UpdateNodeTunnels(loomNode *infrav1alpha1.LoomNode) error {
	tun, exists := mgr.tunnels[loomNode.Name]
	if exists {
		if err := tun.UpdateDestinationNode(loomNode); err != nil {
			return fmt.Errorf("failed to update destination node %s: %v", loomNode.Name, err)
		}
		return nil
	}
	tun, err := NewTunnel(loomNode, mgr.ip4t, mgr.ip6t, mgr.tunnelInterface, mgr.vrfName)
	if err != nil {
		return fmt.Errorf("failed to create Geneve tunnel for node %s: %v", loomNode.Name, err)
	}
	mgr.tunnels[loomNode.Name] = tun
	return nil
}

func (mgr *TunnelManager) AddEgressRoute(nodeName string, dst netutils.DualStackNetwork) error {
	tun, exists := mgr.tunnels[nodeName]
	if !exists {
		return fmt.Errorf("no Geneve tunnel exists yet for node %s", nodeName)
	}
	if err := tun.AddEgressRoute(dst); err != nil {
		return fmt.Errorf("failed to add egress route for node %s: %v", nodeName, err)
	}
	return nil
}

func (mgr *TunnelManager) RemoveEgressRoute(nodeName string, dst netutils.DualStackNetwork) error {
	tun, exists := mgr.tunnels[nodeName]
	if !exists {
		return nil // Tunnel already doesn't exist, nothing to remove
	}
	if err := tun.RemoveEgressRoute(dst); err != nil {
		return fmt.Errorf("failed to remove egress route for node %s: %v", nodeName, err)
	}
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
