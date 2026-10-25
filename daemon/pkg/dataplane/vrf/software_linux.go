package vrf

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mantra6g/iml/daemon/env"
	"github.com/mantra6g/iml/daemon/pkg/dataplane"
	vrfutil "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	"github.com/mantra6g/iml/daemon/pkg/tunnel/geneve"
	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/go-logr/logr"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=core.loom.io,resources=applications,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.loom.io,resources=applications/status,verbs=get;update;patch

const (
	// RoutingVRFName is the name of the VRF that will be used to interconnect the different Application subnets
	RoutingVRFName = "router-vrf"

	// DefaultMTU sets the standard Maximum Transfer Unit for interfaces in the dataplane.
	//
	// This value comes from calculating the maximum packet size that can be sent with SRv6 encapsulation (8B)
	// with a maximum of 8 segments (8*16B), while also being sent through a VXLAN tunnel (70B), which results in
	// a maximum of 1500 - 8 - 128 - 70 = 1294 bytes.
	DefaultMTU = 1294

	// DecapInterfaceName sets the name for the SRv6 decapsulation interface in the router VRF.
	DecapInterfaceName = "decap0"

	// DecapPeerInterfaceName sets the name for the peer of the SRv6 decapsulation interface in the router VRF.
	DecapPeerInterfaceName = DecapInterfaceName + "pipe"

	// DefaultSRv6TableName set the name for the iptable table used for allowing SRv6 traffic in the cluster.
	DefaultSRv6TableName = "IML-SRV6"

	// CTZoneChainName is the raw-table chain that assigns the router VRF's conntrack zone to traffic entering the router VRF.
	CTZoneChainName = "IML-CT-ZONES"

	// SRv6PacketMark
	SRv6PacketMark = "0x10000000"

	// NoSNATRuleComment is the comment attached to the nat rule that prevents SNAT of intra-cluster traffic.
	NoSNATRuleComment = "iml-srv6: no SNAT inside fabric"
)

type Software struct {
	appSubnets         map[client.ObjectKey][]AppSubnet
	appMu              sync.Mutex
	p4Targets          map[client.ObjectKey]*P4TargetInstance
	p4Mu               sync.Mutex
	routingSubnet      *RoutingSubnet
	serviceChainRoutes map[client.ObjectKey][]dataplane.SRv6Route
	ipt                *iptables.IPTables
	ip4t               *iptables.IPTables

	appNet6Allocator *dataplane.Subnet6Allocator
	appNet4Allocator *dataplane.Subnet4Allocator
	tunNet6Allocator *dataplane.Subnet6Allocator
	tunNet4Allocator *dataplane.Subnet4Allocator
	tableAllocator   *dataplane.TableAllocator

	cfg    *env.GlobalConfig
	Client client.Client
	log    logr.Logger
}

type StackType = string

const (
	UnknownStack StackType = ""
	IPv4Only     StackType = "IPv4Only"
	IPv6Only     StackType = "IPv6Only"
	DualStack    StackType = "DualStack"
)

type Subnet interface {
	GetNetwork() netutils.DualStackNetwork
	GetGateway() netutils.DualStackAddress
	GetStack() StackType
	GetVRFName() string
	GetVRFTable() uint32
	SetTunnel(string)
}

type P4TargetInstance struct {
	TargetIPs netutils.DualStackNetwork
	links     *p4TargetLinks
}

func NewSoftware(logger logr.Logger, cfg *env.GlobalConfig, k8sClient client.Client) (_ dataplane.Dataplane, err error) {
	if cfg == nil {
		return nil, fmt.Errorf("global config is nil")
	}
	if cfg.ClusterCIDR.IPv6Net == nil {
		return nil, fmt.Errorf("cluster IPv6 Range cannot nil")
	}

	net6Allocator, err := dataplane.NewSubnet6Allocator(cfg.PodCIDR.IPv6Net, 96)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 subnet allocator: %w", err)
	}
	var net4Allocator *dataplane.Subnet4Allocator
	if cfg.ClusterCIDR.IPv4Net != nil {
		net4Allocator, err = dataplane.NewSubnet4Allocator(cfg.PodCIDR.IPv4Net, 28)
		if err != nil {
			return nil, fmt.Errorf("failed to create application subnet allocator: %w", err)
		}
	}
	tunnel6Allocator, err := dataplane.NewSubnet6Allocator(cfg.TunnelCIDR.IPv6Net, 126)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel subnet allocator: %w", err)
	}
	var tunnel4Allocator *dataplane.Subnet4Allocator
	if cfg.TunnelCIDR.IPv4Net != nil {
		tunnel4Allocator, err = dataplane.NewSubnet4Allocator(cfg.TunnelCIDR.IPv4Net, 30)
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel subnet allocator: %w", err)
		}
	}
	routingIPNet, err := net6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to assign router-vrf's ip network: %w", err)
	}
	routingSIDNet, err := net6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to assign router-vrf's SID network: %w", err)
	}
	tableAllocator, err := dataplane.NewTableAllocator(1000)
	if err != nil {
		return nil, fmt.Errorf("failed to create table allocator: %w", err)
	}
	rtrVrfTable, err := tableAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to create routing table allocator: %w", err)
	}

	// Enable IPv6 forwarding. Required in host namespace to route packets between interfaces.
	if err = os.WriteFile("/proc/sys/net/ipv6/conf/all/forwarding", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
	}
	// Enable IPv4 forwarding
	if err = os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
	}
	// Enable SRv6 globally. Required in host namespace to decapsulate SRv6 packets.
	if err = os.WriteFile("/proc/sys/net/ipv6/conf/all/seg6_enabled", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to set seg6_enabled: %w", err)
	}
	// Enable SRv6 by default. Required in host namespace to decapsulate SRv6 packets.
	if err = os.WriteFile("/proc/sys/net/ipv6/conf/default/seg6_enabled", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to set seg6_enabled: %w", err)
	}
	// Enable VRF strict mode. Recommended because of SRv6 and VRF interaction.
	if err := os.WriteFile("/proc/sys/net/vrf/strict_mode", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to enable VRF strict mode: %s", err)
	}
	// Disable reverse-path filtering. Needed for the routing to work properly in the presence of asymmetric routes, which can happen with SRv6.
	if err = os.WriteFile("/proc/sys/net/ipv4/conf/all/rp_filter", []byte("0"), 0644); err != nil {
		return nil, fmt.Errorf("failed to disable rp_filter: %w", err)
	}
	// Enable LWTunnel hooks.
	if err = os.WriteFile("/proc/sys/net/netfilter/nf_hooks_lwtunnel", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to enable LWTunnel hooks: %w", err)
	}
	rtrSubnet, err := NewRoutingSubnet(logger.WithName("routing-subnet"), routingIPNet, routingSIDNet, rtrVrfTable)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing subnet: %w", err)
	}
	// Nothing is registered with the janitor until NewSoftware returns, so undo the routing subnet here.
	defer func() {
		if err != nil {
			rtrSubnet.Teardown()
		}
	}()

	ip6t, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv6))
	if err != nil {
		return nil, fmt.Errorf("failed to create ip6t: %w", err)
	}
	err = ensureFilterIPTablesRulesArePresent(ip6t)
	if err != nil {
		return nil, fmt.Errorf("failed to create filter iptables rules: %w", err)
	}
	err = ensureRawIPTablesRulesArePresent(ip6t)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw iptables rules: %w", err)
	}

	var ip4t *iptables.IPTables
	if cfg.ClusterCIDR.IPv4Net != nil {
		ip4t, err = iptables.New(iptables.IPFamily(iptables.ProtocolIPv4))
		if err != nil {
			return nil, fmt.Errorf("failed to create ip4t: %w", err)
		}
		err = ensureNatIPTables4RulesArePresent(ip4t, cfg.ClusterCIDR.IPv4Net)
		if err != nil {
			return nil, fmt.Errorf("failed to create nat IPv4 iptables rules: %w", err)
		}
		err = ensureRawIPTablesRulesArePresent(ip4t)
		if err != nil {
			return nil, fmt.Errorf("failed to create raw IPv4 iptables rules: %w", err)
		}
	}

	d := &Software{
		appNet4Allocator:   net4Allocator,
		appNet6Allocator:   net6Allocator,
		tunNet4Allocator:   tunnel4Allocator,
		tunNet6Allocator:   tunnel6Allocator,
		tableAllocator:     tableAllocator,
		routingSubnet:      rtrSubnet,
		appSubnets:         make(map[client.ObjectKey][]AppSubnet),
		p4Targets:          make(map[client.ObjectKey]*P4TargetInstance),
		serviceChainRoutes: make(map[client.ObjectKey][]dataplane.SRv6Route),
		ipt:                ip6t,
		ip4t:               ip4t,
		cfg:                cfg,
		Client:             k8sClient,
		log:                logger,
	}
	err = d.addRouterVRFCTZoneRules()
	if err != nil {
		return nil, fmt.Errorf("failed to add router VRF CT zone rules: %w", err)
	}
	return d, nil
}

func (d *Software) Shutdown(ctx context.Context) error {
	// Delete the router subnet
	d.routingSubnet.Teardown()

	// Delete all application subnets
	d.appMu.Lock()
	defer d.appMu.Unlock()
	for _, subnets := range d.appSubnets {
		for i := range subnets {
			subnets[i].Teardown()
		}
	}

	// Delete iptables rules
	err := ensureFilterIPTablesRulesAreRemoved(d.ipt)
	if err != nil {
		d.log.Error(err, "failed to remove filter iptables rules. Ignoring error...")
	}
	err = ensureRawIPTablesRulesAreRemoved(d.ipt)
	if err != nil {
		d.log.Error(err, "failed to remove raw iptables rules. Ignoring error...")
	}
	if d.ip4t != nil {
		err = ensureNatIPTables4RulesAreRemoved(d.ip4t, d.cfg.ClusterCIDR.IPv4Net)
		if err != nil {
			d.log.Error(err, "failed to remove nat IPv4 iptables rules. Ignoring error...")
		}
		err = ensureRawIPTablesRulesAreRemoved(d.ip4t)
		if err != nil {
			d.log.Error(err, "failed to remove raw IPv4 iptables rules. Ignoring error...")
		}
	}

	return nil
}

func ensureFilterIPTablesRulesArePresent(ipt *iptables.IPTables) error {
	err := ipt.ClearChain("filter", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to clear iptables chain: %w", err)
	}
	err = ipt.Append("filter", DefaultSRv6TableName,
		"-m", "rt", "--rt-type", "4", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to append SRv6 accept rule: %w", err)
	}
	err = ipt.Append("filter", DefaultSRv6TableName,
		"-j", "RETURN")
	if err != nil {
		return fmt.Errorf("failed to append return rule: %w", err)
	}
	err = ipt.DeleteIfExists("filter", "FORWARD",
		"-j", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to delete existing hook rule to FORWARD chain: %w", err)
	}
	err = ipt.InsertUnique("filter", "FORWARD", 1,
		"-j", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to insert hook rule to FORWARD chain: %w", err)
	}
	err = deleteKubeForwardInvalidDropRules(ipt)
	if err != nil {
		return fmt.Errorf("failed to delete INVALID drop rule from KUBE-FORWARD chain: %w", err)
	}
	return nil
}

// deleteKubeForwardInvalidDropRules removes kube-proxy's conntrack INVALID drop rules from KUBE-FORWARD,
// which would otherwise drop forwarded SRv6 traffic. Rules are matched by content rather than position
// so that repeated daemon restarts never delete unrelated kube-proxy rules.
func deleteKubeForwardInvalidDropRules(ipt *iptables.IPTables) error {
	exists, err := ipt.ChainExists("filter", "KUBE-FORWARD")
	if err != nil || !exists {
		return err
	}
	rules, err := ipt.List("filter", "KUBE-FORWARD")
	if err != nil {
		return err
	}
	// rules[0] is the "-N KUBE-FORWARD" chain declaration, so rules[i] is rule number i.
	// Delete from the end so the remaining rule numbers stay valid.
	for i := len(rules) - 1; i >= 1; i-- {
		if !isInvalidDropRule(rules[i]) {
			continue
		}
		if err = ipt.DeleteById("filter", "KUBE-FORWARD", i); err != nil {
			return err
		}
	}
	return nil
}

// isInvalidDropRule reports whether an `iptables -S` rule drops conntrack INVALID packets, e.g. kube-proxy's
// "-A KUBE-FORWARD -m conntrack --ctstate INVALID -m nfacct --nfacct-name ct_state_invalid_dropped_pkts -j DROP".
func isInvalidDropRule(rule string) bool {
	return strings.Contains(rule, "--ctstate INVALID") && strings.HasSuffix(rule, "-j DROP")
}

// ensureRawIPTablesRulesArePresent creates (or flushes) the chain holding the per-tunnel conntrack zone
// rules and hooks it at the top of the raw PREROUTING chain.
func ensureRawIPTablesRulesArePresent(ipt *iptables.IPTables) error {
	err := ipt.ClearChain("raw", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to clear %s chain: %w", CTZoneChainName, err)
	}
	err = ipt.DeleteIfExists("raw", "PREROUTING",
		"-j", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to delete existing hook rule to PREROUTING chain: %w", err)
	}
	err = ipt.InsertUnique("raw", "PREROUTING", 1,
		"-j", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to insert hook rule to PREROUTING chain: %w", err)
	}
	return nil
}

func ensureRawIPTablesRulesAreRemoved(ipt *iptables.IPTables) error {
	err := ipt.ClearChain("raw", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to clear %s chain: %w", CTZoneChainName, err)
	}
	err = ipt.DeleteIfExists("raw", "PREROUTING",
		"-j", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to delete existing hook rule to PREROUTING chain: %w", err)
	}
	err = ipt.DeleteChain("raw", CTZoneChainName)
	if err != nil {
		return fmt.Errorf("failed to delete %s chain: %w", CTZoneChainName, err)
	}
	return nil
}

// addRouterVRFCTZoneRules places traffic entering the router VRF through its static interfaces into the
// router VRF's conntrack zone.
func (d *Software) addRouterVRFCTZoneRules() error {
	ifaces := []string{
		geneve.TunnelName,
		d.routingSubnet.Bridge.Name,
		DecapInterfaceName,
		DecapPeerInterfaceName,
	}
	for _, iface := range ifaces {
		err := d.addCTZoneRule(iface, d.routingSubnet.Vrf.Table)
		if err != nil {
			return err
		}
	}
	return nil
}

// addAppSubnetCTZoneRules places traffic entering an app subnet's VRF through its static interfaces
// into that VRF's conntrack zone.
func (d *Software) addAppSubnetCTZoneRules(subnet *AppSubnet) error {
	for _, iface := range appSubnetCTZoneInterfaces(subnet) {
		err := d.addCTZoneRule(iface, subnet.Vrf.Table)
		if err != nil {
			return err
		}
	}
	return nil
}

// removeAppSubnetCTZoneRules removes the conntrack zone rules of an app subnet's static interfaces.
func (d *Software) removeAppSubnetCTZoneRules(subnet *AppSubnet) error {
	for _, iface := range appSubnetCTZoneInterfaces(subnet) {
		err := d.removeCTZoneRule(iface, subnet.Vrf.Table)
		if err != nil {
			return err
		}
	}
	return nil
}

// appSubnetCTZoneInterfaces returns the static interfaces through which traffic enters an app subnet's VRF:
// the VRF end of the bridge-VRF veth pair and the bridge itself.
func appSubnetCTZoneInterfaces(subnet *AppSubnet) []string {
	return []string{
		subnet.Bridge.Name,
	}
}

func ctZoneRule(ifaceName string, zone uint32) []string {
	return []string{"-i", ifaceName, "-j", "CT", "--zone", strconv.FormatUint(uint64(zone), 10)}
}

// removeCTZoneRule removes the rule added by addCTZoneRule.
func (d *Software) removeCTZoneRule(ifaceName string, zone uint32) error {
	rule := ctZoneRule(ifaceName, zone)
	err := d.ipt.DeleteIfExists("raw", CTZoneChainName, rule...)
	if err != nil {
		return fmt.Errorf("failed to delete IPv6 CT zone rule for %s: %w", ifaceName, err)
	}
	if d.ip4t != nil {
		err = d.ip4t.DeleteIfExists("raw", CTZoneChainName, rule...)
		if err != nil {
			return fmt.Errorf("failed to delete IPv4 CT zone rule for %s: %w", ifaceName, err)
		}
	}
	return nil
}

// addCTZoneRule places traffic entering a VRF through ifaceName into conntrack zone `zone`.
func (d *Software) addCTZoneRule(ifaceName string, zone uint32) error {
	rule := ctZoneRule(ifaceName, zone)
	err := d.ipt.AppendUnique("raw", CTZoneChainName, rule...)
	if err != nil {
		return fmt.Errorf("failed to append IPv6 CT zone rule for %s: %w", ifaceName, err)
	}
	if d.ip4t != nil {
		err = d.ip4t.AppendUnique("raw", CTZoneChainName, rule...)
		if err != nil {
			return fmt.Errorf("failed to append IPv4 CT zone rule for %s: %w", ifaceName, err)
		}
	}
	return nil
}

func noSNATRule(clusterNet *net.IPNet) []string {
	return []string{
		"-s", clusterNet.String(), "-d", clusterNet.String(),
		"-m", "comment", "--comment", NoSNATRuleComment,
		"-j", "ACCEPT",
	}
}

// ensureNatIPTables4RulesArePresent prevents intra-cluster traffic from being SNATed by
// other POSTROUTING rules (e.g. kube-proxy masquerading).
func ensureNatIPTables4RulesArePresent(ipt *iptables.IPTables, clusterNet *net.IPNet) error {
	rule := noSNATRule(clusterNet)
	err := ipt.DeleteIfExists("nat", "POSTROUTING", rule...)
	if err != nil {
		return fmt.Errorf("failed to delete existing no-SNAT rule from POSTROUTING chain: %w", err)
	}
	err = ipt.Insert("nat", "POSTROUTING", 1, rule...)
	if err != nil {
		return fmt.Errorf("failed to insert no-SNAT rule to POSTROUTING chain: %w", err)
	}
	return nil
}

func ensureNatIPTables4RulesAreRemoved(ipt *iptables.IPTables, clusterNet *net.IPNet) error {
	err := ipt.DeleteIfExists("nat", "POSTROUTING", noSNATRule(clusterNet)...)
	if err != nil {
		return fmt.Errorf("failed to delete no-SNAT rule from POSTROUTING chain: %w", err)
	}
	return nil
}

func ensureFilterIPTablesRulesAreRemoved(ipt *iptables.IPTables) error {
	err := ipt.ClearChain("filter", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to clear %s chain: %w", DefaultSRv6TableName, err)
	}
	err = ipt.DeleteIfExists("filter", "FORWARD",
		"-j", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to delete existing hook rule to FORWARD chain: %w", err)
	}
	err = ipt.DeleteChain("filter", DefaultSRv6TableName)
	if err != nil {
		return fmt.Errorf("failed to delete %s chain: %w", DefaultSRv6TableName, err)
	}
	return nil
}

func (d *Software) AddServiceChainRoutes(chain *corev1alpha1.ServiceChain, routes []dataplane.SRv6Route) error {
	if d.serviceChainRoutes[client.ObjectKeyFromObject(chain)] == nil {
		d.serviceChainRoutes[client.ObjectKeyFromObject(chain)] = make([]dataplane.SRv6Route, 0)
	}
	for _, route := range routes {
		sourceAppSubnets, exists := d.appSubnets[route.SourceApp]
		if !exists {
			return fmt.Errorf("source app subnet %s does not exist", route.SourceApp)
		}
		if len(sourceAppSubnets) == 0 {
			return fmt.Errorf("source app subnet %s has no subnets in use", route.SourceApp)
		}
		for i := range sourceAppSubnets {
			subnet := &sourceAppSubnets[i]
			err := subnet.AddSRv6Route(route.DestNet, route.FunctionIPs, d.routingSubnet.DecapSIDv4, d.routingSubnet.DecapSIDv6)
			if err != nil {
				return fmt.Errorf("failed to add SRv6 route to subnet %s: %w", route.DestNet, err)
			}
		}
		d.serviceChainRoutes[client.ObjectKeyFromObject(chain)] = append(d.serviceChainRoutes[client.ObjectKeyFromObject(chain)], route)
	}
	return nil
}

func (d *Software) ListServiceChainRoutes(chain *corev1alpha1.ServiceChain) ([]dataplane.SRv6Route, error) {
	chainRoutes, exists := d.serviceChainRoutes[client.ObjectKeyFromObject(chain)]
	if !exists {
		return []dataplane.SRv6Route{}, nil
	}
	return chainRoutes, nil
}

func (d *Software) DeleteServiceChainRoute(chain client.ObjectKey, route dataplane.SRv6Route) error {
	if d.serviceChainRoutes[chain] == nil {
		return nil
	}
	sourceAppSubnets, exists := d.appSubnets[route.SourceApp]
	if !exists {
		return nil
	}
	for i := range sourceAppSubnets {
		subnet := &sourceAppSubnets[i]
		err := subnet.DeleteSRv6Route(route.DestNet)
		if err != nil {
			return fmt.Errorf("failed to add SRv6 route to subnet %s: %w", route.DestNet, err)
		}
	}
	d.serviceChainRoutes[chain] = append(d.serviceChainRoutes[chain], route)
	return nil
}

func (d *Software) DeleteAllServiceChainRoutes(chain client.ObjectKey) error {
	chainRoutes, exists := d.serviceChainRoutes[chain]
	if !exists {
		return nil
	}
	for _, route := range chainRoutes {
		err := d.DeleteServiceChainRoute(chain, route)
		if err != nil {
			return err
		}
	}
	delete(d.serviceChainRoutes, chain)
	return nil
}

// Creates a subnet into the dataplane and returns the configured bridge name.
func (d *Software) addApplicationSubnet(appID types.NamespacedName) (subnet *AppSubnet, err error) {
	logger := d.log
	logger.V(1).Info("Adding application subnet", "appID", appID)

	var appNet4, appNet6 *net.IPNet
	if d.appNet4Allocator != nil {
		appNet4, err = d.appNet4Allocator.Allocate()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate IPv4 application subnet: %w", err)
		}
	}
	if d.appNet6Allocator != nil {
		appNet6, err = d.appNet6Allocator.Allocate()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate IPv6 application subnet: %w", err)
		}
	}
	tableID, err := d.tableAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate application table: %w", err)
	}
	loggerName := fmt.Sprintf("app-%s-%s-%d", appID.Namespace, appID.Name, tableID)
	subnet, err = NewAppSubnet(logger.WithName(loggerName), appNet4, appNet6, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to create application subnet: %w", err)
	}

	// From now on, if any errors happen when configuring this subnet, tear it down.
	// The subnet is captured here because returning nil resets the named result before the defer runs.
	newSubnet := subnet
	var routingSubnetTunData, newSubnetTunData *subnetTunData
	defer func() {
		if err != nil {
			logger.Error(err, "Failed to add application subnet")
			if ctErr := d.removeAppSubnetCTZoneRules(newSubnet); ctErr != nil {
				logger.Error(ctErr, "failed to remove app subnet CT zone rules. Ignoring error...")
			}
			if routingSubnetTunData != nil && newSubnetTunData != nil {
				if ctErr := d.removeCTZoneRule(routingSubnetTunData.InterfaceName, d.routingSubnet.Vrf.Table); ctErr != nil {
					logger.Error(ctErr, "failed to remove routing subnet tunnel CT zone rule. Ignoring error...")
				}
				if ctErr := d.removeCTZoneRule(newSubnetTunData.InterfaceName, newSubnet.Vrf.Table); ctErr != nil {
					logger.Error(ctErr, "failed to remove app subnet tunnel CT zone rule. Ignoring error...")
				}
			}
			newSubnet.Teardown()
		}
	}()

	err = d.addAppSubnetCTZoneRules(subnet)
	if err != nil {
		return nil, fmt.Errorf("failed to add app subnet CT zone rules: %w", err)
	}

	routingSubnetTunData, newSubnetTunData, err = d.createSubnetToSubnetTunnels(d.routingSubnet, subnet)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet to application subnet: %w", err)
	}
	logger.V(1).Info("Created tunnels between routing subnet and application subnet",
		"routingSubnetTunData", routingSubnetTunData, "newSubnetTunData", newSubnetTunData)

	err = d.routingSubnet.AddRoute(subnet.Networks, newSubnetTunData.Addrs, routingSubnetTunData.InterfaceName)
	if err != nil {
		err = fmt.Errorf("failed to install routes towards app subnet in routing subnet: %w", err)
		return
	}
	err = subnet.AddDefaultRoute(routingSubnetTunData.Addrs, newSubnetTunData.InterfaceName)
	if err != nil {
		err = fmt.Errorf("failed to install default routes towards routing subnet in app subnet: %w", err)
	}
	existingSubnets, ok := d.appSubnets[appID]
	if !ok {
		d.appSubnets[appID] = []AppSubnet{*subnet}
	} else {
		d.appSubnets[appID] = append(existingSubnets, *subnet)
	}
	err = d.addSubnetToAppStatus(appID, appNet4, appNet6)
	if err != nil {
		err = fmt.Errorf("failed to update application status with subnet info: %w", err)
		return
	}
	return subnet, nil
}

type subnetTunData struct {
	InterfaceName string
	Addrs         netutils.DualStackAddress
}

func (d *Software) createSubnetToSubnetTunnels(sub1, sub2 Subnet) (sub1TunData, sub2TunData *subnetTunData, err error) {
	tunName1, err := vrfutil.GenerateRandomName("imltun", 4)
	if err != nil {
		return
	}
	tunName2, err := vrfutil.GenerateRandomName("imltun", 4)
	if err != nil {
		return
	}
	var tun1IPv4, tun2IPv4 *net.IPNet
	if d.tunNet4Allocator != nil {
		var allocErr error
		tunIPv4Net, allocErr := d.tunNet4Allocator.Allocate()
		if allocErr != nil {
			err = allocErr
			return
		}
		tunIPv4Allocator, allocErr := dataplane.NewIPv4Allocator(tunIPv4Net)
		if allocErr != nil {
			err = allocErr
			return
		}
		tun1IPv4, allocErr = tunIPv4Allocator.Allocate()
		if allocErr != nil {
			err = allocErr
			return
		}
		tun2IPv4, allocErr = tunIPv4Allocator.Allocate()
		if allocErr != nil {
			err = allocErr
			return
		}
	}
	tunIPv6Net, err := d.tunNet6Allocator.Allocate()
	if err != nil {
		return
	}
	tunIPv6Allocator, err := dataplane.NewIPv6Allocator(tunIPv6Net)
	if err != nil {
		return
	}
	tun1IPv6, err := tunIPv6Allocator.Allocate()
	if err != nil {
		return
	}
	tun2IPv6, err := tunIPv6Allocator.Allocate()
	if err != nil {
		return
	}
	vrf1, err := netlink.LinkByName(sub1.GetVRFName())
	if err != nil {
		return
	}
	vrf2, err := netlink.LinkByName(sub2.GetVRFName())
	if err != nil {
		return
	}
	tun1 := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: tunName1,
		},
		PeerName: tunName2,
	}
	err = netlink.LinkAdd(tun1)
	if err != nil {
		return
	}
	sub1.SetTunnel(tunName1)
	sub2.SetTunnel(tunName2)
	tun2, err := netlink.LinkByName(tunName2)
	if err != nil {
		return
	}
	err = netlink.LinkSetMaster(tun1, vrf1)
	if err != nil {
		return
	}
	err = netlink.LinkSetMaster(tun2, vrf2)
	if err != nil {
		return
	}
	// Traffic entering each VRF through its tunnel end is tracked in that VRF's conntrack zone.
	// The caller doesn't learn the tunnel names on failure, so the rules are removed here.
	defer func() {
		if err != nil {
			if ctErr := d.removeCTZoneRule(tunName1, sub1.GetVRFTable()); ctErr != nil {
				d.log.Error(ctErr, "failed to remove tunnel CT zone rule. Ignoring error...", "tunnel", tunName1)
			}
			if ctErr := d.removeCTZoneRule(tunName2, sub2.GetVRFTable()); ctErr != nil {
				d.log.Error(ctErr, "failed to remove tunnel CT zone rule. Ignoring error...", "tunnel", tunName2)
			}
		}
	}()
	err = d.addCTZoneRule(tunName1, sub1.GetVRFTable())
	if err != nil {
		return
	}
	err = d.addCTZoneRule(tunName2, sub2.GetVRFTable())
	if err != nil {
		return
	}
	err = netlink.AddrAdd(tun1, &netlink.Addr{IPNet: tun1IPv6})
	if err != nil {
		return
	}
	err = netlink.AddrAdd(tun2, &netlink.Addr{IPNet: tun2IPv6})
	if err != nil {
		return
	}
	if tun1IPv4 != nil && tun2IPv4 != nil {
		err = netlink.AddrAdd(tun1, &netlink.Addr{IPNet: tun1IPv4})
		if err != nil {
			return
		}
		err = netlink.AddrAdd(tun2, &netlink.Addr{IPNet: tun2IPv4})
		if err != nil {
			return
		}
	}
	err = netlink.LinkSetUp(tun1)
	if err != nil {
		return
	}
	err = netlink.LinkSetUp(tun2)
	if err != nil {
		return
	}
	var tun1Ip4, tun2Ip4 net.IP
	if tun1IPv4 != nil && tun2IPv4 != nil {
		tun1Ip4 = tun1IPv4.IP
		tun2Ip4 = tun2IPv4.IP
	}
	sub1TunData = &subnetTunData{
		InterfaceName: tunName1,
		Addrs: netutils.DualStackAddress{
			IPv4: tun1Ip4,
			IPv6: tun1IPv6.IP,
		},
	}
	sub2TunData = &subnetTunData{
		InterfaceName: tunName2,
		Addrs: netutils.DualStackAddress{
			IPv4: tun2Ip4,
			IPv6: tun2IPv6.IP,
		},
	}
	return
}

func (d *Software) addSubnetToAppStatus(appID types.NamespacedName, appNet4 *net.IPNet, appNet6 *net.IPNet) error {
	var app = &corev1alpha1.Application{}
	err := d.Client.Get(context.Background(), appID, app)
	if err != nil {
		return fmt.Errorf("failed to get application: %w", err)
	}
	original := app.DeepCopy()
	if app.Status.Subnets == nil {
		app.Status.Subnets = make(map[string][]corev1alpha1.DualStackNetwork)
	}
	if app.Status.Subnets[d.cfg.NodeName] == nil {
		app.Status.Subnets[d.cfg.NodeName] = make([]corev1alpha1.DualStackNetwork, 0)
	}
	app.Status.Subnets[d.cfg.NodeName] = append(app.Status.Subnets[d.cfg.NodeName], corev1alpha1.DualStackNetwork{
		IPv4Net: appNet4.String(),
		IPv6Net: appNet6.String(),
	})
	err = d.Client.Status().Patch(context.Background(), app, client.MergeFrom(original))
	if err != nil {
		return fmt.Errorf("failed to patch application: %w", err)
	}
	return nil
}

func (d *Software) ConfigureAppInstance(
	app *corev1alpha1.Application, _ string,
) (*dataplane.AppConfig, error) {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	subnets, exists := d.appSubnets[client.ObjectKeyFromObject(app)]
	if !exists {
		subnets = []AppSubnet{}
	}
	subnet, err := getFirstSubnetWithAvailableIPs(subnets)
	if !exists || err != nil {
		subnet, err = d.addApplicationSubnet(client.ObjectKeyFromObject(app))
		if err != nil {
			return nil, fmt.Errorf("failed to add subnet: %w", err)
		}
	}
	ips, err := subnet.AllocateIPs()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IPs for application %s/%s: %w", app.Name, app.Namespace, err)
	}
	ifaceName, err := vrfutil.GenerateRandomName("nfr", 8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate interface name: %w", err)
	}

	return &dataplane.AppConfig{
		IPs: ips,
		ClusterCIDRs: netutils.DualStackNetwork{
			IPv4Net: d.cfg.ClusterCIDR.IPv4Net,
			IPv6Net: d.cfg.ClusterCIDR.IPv6Net,
		},
		ServiceCIDRs: d.cfg.ServiceCIDR,
		Gateways:     subnet.GatewayIPs,
		Bridge:       subnet.Bridge.Name,
		MTU:          DefaultMTU,
		IfaceName:    ifaceName,
	}, nil
}

func (d *Software) DeleteAppInstance(_ string) error {
	// Nothing to do here for now
	return nil
}

func (d *Software) ConfigureP4TargetInstance(target *corev1alpha1.P4Target) (*dataplane.P4TargetConfig, error) {
	if dataplane.ReusesHostInterfaces(target) {
		return nil, fmt.Errorf("P4Target %s reuses existing host interfaces and is not set up through the CNI plugin", target.Name)
	}

	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	key := client.ObjectKeyFromObject(target)
	p4TargetConfig, exists := d.p4Targets[key]
	if !exists {
		p4TargetConfig = &P4TargetInstance{}
	}

	if p4TargetConfig.TargetIPs.IsEmpty() {
		// The daemon may have restarted since the IP was assigned. It's recorded in the spec.
		addrs, err := vrfutil.ParseDualStackGatewayFromStrings(target.Spec.TargetDataplaneIPs)
		if err == nil && addrs.IPv6 != nil {
			p4TargetConfig.TargetIPs.IPv6Net = &net.IPNet{IP: addrs.IPv6, Mask: net.CIDRMask(128, 128)}
		}
	}
	if p4TargetConfig.TargetIPs.IsEmpty() {
		ips, err := d.assignP4TargetIPs(context.Background(), target)
		if err != nil {
			return nil, fmt.Errorf("failed to assign IPs for P4Target %s/%s: %w", target.Name, target.Namespace, err)
		}
		p4TargetConfig.TargetIPs = ips
	}

	if p4TargetConfig.links == nil {
		links, err := newP4TargetLinks(target)
		if err != nil {
			return nil, fmt.Errorf("failed to name host interfaces for P4Target %s/%s: %w", target.Name, target.Namespace, err)
		}
		p4TargetConfig.links = links
	}

	d.p4Targets[key] = p4TargetConfig

	return &dataplane.P4TargetConfig{
		HostInterfaces: p4TargetConfig.links.Ports,
		MTU:            DefaultP4TargetMTU,
	}, nil
}

func (d *Software) AssignP4TargetIPs(ctx context.Context, target *corev1alpha1.P4Target) (netutils.DualStackNetwork, error) {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	if target.Status.NodeName != d.cfg.NodeName {
		return netutils.DualStackNetwork{}, fmt.Errorf("P4Target %s/%s is not scheduled on this node", target.Name, target.Namespace)
	}

	config, exists := d.p4Targets[client.ObjectKeyFromObject(target)]
	if exists && !config.TargetIPs.IsEmpty() {
		return config.TargetIPs, nil
	}

	ips, err := d.assignP4TargetIPs(ctx, target)
	if err != nil {
		return netutils.DualStackNetwork{}, err
	}

	if config == nil {
		config = &P4TargetInstance{}
	}
	config.TargetIPs = ips
	d.p4Targets[client.ObjectKeyFromObject(target)] = config

	return ips, nil
}

func (d *Software) assignP4TargetIPs(ctx context.Context, target *corev1alpha1.P4Target) (netutils.DualStackNetwork, error) {
	original := target.DeepCopy()
	ips, err := d.routingSubnet.AllocateIPs()
	if err != nil {
		return netutils.DualStackNetwork{}, fmt.Errorf("failed to allocate IPs for target \"%s\": %w", target.Name, err)
	}
	target.Spec.TargetDataplaneIPs = []string{ips.IPv6Net.IP.String()}
	err = d.Client.Patch(ctx, target, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	if err != nil {
		return netutils.DualStackNetwork{}, fmt.Errorf("failed to patch P4Target %s/%s with allocated IPs: %w", target.Name, target.Namespace, err)
	}
	return ips, nil
}

func (d *Software) UpdateAppRoutes(app *corev1alpha1.Application) error {
	return nil
}

func (d *Software) RemoveAppRoutes(app client.ObjectKey) error {
	return nil
}

// UpdateP4TargetRoutes routes the target's NF CIDR to the target through the routing bridge. It
// only applies on the node hosting the target, since the target's IPs are on-link there alone.
func (d *Software) UpdateP4TargetRoutes(target *corev1alpha1.P4Target) error {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	if target.Status.NodeName != d.cfg.NodeName {
		return nil
	}
	if len(target.Spec.TargetDataplaneIPs) == 0 || target.Spec.NfCIDR == "" {
		// We don't have enough information about the object yet to update its routes.
		return nil
	}
	targetAddrs, err := vrfutil.ParseDualStackGatewayFromStrings(target.Spec.TargetDataplaneIPs)
	if err != nil {
		return fmt.Errorf("failed to parse target IPs for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	nfCIDR, err := vrfutil.ParseDualStackNetworkFromStrings([]string{target.Spec.NfCIDR})
	if err != nil {
		return fmt.Errorf("failed to parse nf CIDR for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	err = d.routingSubnet.AddRoute(nfCIDR, targetAddrs, d.routingSubnet.Bridge.Attrs().Name)
	if err != nil {
		return fmt.Errorf("failed to add route for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	return nil
}

func (d *Software) RemoveP4TargetRoutes(target client.ObjectKey) error {
	return nil
}

func getFirstSubnetWithAvailableIPs(subnets []AppSubnet) (*AppSubnet, error) {
	for _, subnet := range subnets {
		if subnet.HasIPsAvailable() {
			return &subnet, nil
		}
	}
	return nil, fmt.Errorf("no available subnets found")
}
