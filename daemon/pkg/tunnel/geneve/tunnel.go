package geneve

import (
	"fmt"

	vrfutils "iml-daemon/pkg/dataplane/vrf/util"
	netutils "iml-daemon/pkg/utils/net"

	"github.com/coreos/go-iptables/iptables"
	corev1 "k8s.io/api/core/v1"
)

type Tunnel struct {
	chainName string
	ip4t      *iptables.IPTables
	ip6t      *iptables.IPTables
}

func NewTunnel(
	node *corev1.Node, ip4tables *iptables.IPTables, ip6tables *iptables.IPTables,
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
		chainName: chainName,
		ip4t:      ip4tables,
		ip6t:      ip6tables,
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
				"-s", ip.String(), "-j", "MARK", "--or-mark", PacketAcceptedMark)
			if err != nil {
				return fmt.Errorf("failed to insert mark rule: %v", err)
			}
		}
		err = t.ip4t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP4 iptables chain: %v", err)
		}
	}
	if len(addr.IPv6Addresses) != 0 {
		err = t.ip6t.ClearChain("filter", t.chainName)
		if err != nil {
			return fmt.Errorf("failed to clear IP6 iptables chain: %v", err)
		}
		for _, ip := range addr.IPv6Addresses {
			err = t.ip6t.Insert("filter", t.chainName, 1,
				"-s", ip.String(), "-j", "mark", "--or-mark", PacketAcceptedMark)
			if err != nil {
				return fmt.Errorf("failed to insert mark rule: %v", err)
			}
		}
		err = t.ip6t.Append("filter", t.chainName, "-j", "RETURN")
		if err != nil {
			return fmt.Errorf("failed to append rule to IP6 iptables chain: %v", err)
		}
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
