package vrf

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"
	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	"iml-daemon/env"
	"iml-daemon/pkg/dataplane"
	vrfutil "iml-daemon/pkg/dataplane/vrf/util"
	"iml-daemon/pkg/tunnel"
	netutils "iml-daemon/pkg/utils/net"

	"github.com/go-logr/logr"
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
)

type Software struct {
	appSubnets    map[client.ObjectKey][]AppSubnet
	appMu         sync.Mutex
	p4Targets     map[client.ObjectKey]*P4TargetInstance
	p4Mu          sync.Mutex
	nodeConfigs   map[client.ObjectKey]*NodeConfig
	nodeMu        sync.Mutex
	tunnelManager tunnel.Manager
	routingSubnet *RoutingSubnet
	//routerVrf  *netlink.Vrf
	serviceChainRoutes map[client.ObjectKey][]dataplane.SRv6Route

	appNet6Allocator *dataplane.Subnet6Allocator
	appNet4Allocator *dataplane.Subnet4Allocator
	//routingIP6Allocator *dataplane.IPv6Allocator
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
}

type P4TargetInstance struct {
	TargetIPs netutils.DualStackNetwork
	ifaceName string
}

type NodeConfig struct {
	LastResourceVersion string
	Route               netutils.DualStackRoute
}

func NewSoftware(logger logr.Logger, cfg *env.GlobalConfig, tunnelManager tunnel.Manager, k8sClient client.Client) (dataplane.Dataplane, error) {
	if cfg == nil {
		return nil, fmt.Errorf("global config is nil")
	}
	if cfg.ClusterCIDR.IPv6Net == nil {
		return nil, fmt.Errorf("cluster IPv6 Range cannot nil")
	}

	net6Allocator, err := dataplane.NewSubnet6Allocator(cfg.ClusterCIDR.IPv6Net, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 subnet allocator: %w", err)
	}
	var net4Allocator *dataplane.Subnet4Allocator
	if cfg.ClusterCIDR.IPv4Net != nil {
		net4Allocator, err = dataplane.NewSubnet4Allocator(cfg.ClusterCIDR.IPv4Net, 28)
		if err != nil {
			return nil, fmt.Errorf("failed to create application subnet allocator: %w", err)
		}
	}
	routingBaseNet, err := net6Allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to create routing subnet's IP allocator: %w", err)
	}
	tunNet6Allocator, err := dataplane.NewSubnet6Allocator(cfg.TunCIDR.IPv6Net, 126)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel subnet allocator: %w", err)
	}
	var tunNet4Allocator *dataplane.Subnet4Allocator
	if cfg.TunCIDR.IPv4Net != nil {
		tunNet4Allocator, err = dataplane.NewSubnet4Allocator(cfg.TunCIDR.IPv4Net, 30)
		if err != nil {
			return nil, fmt.Errorf("failed to create tunnel subnet allocator: %w", err)
		}
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
	// Enable VRF strict mode. Recommended because of SRv6 and VRF interaction.
	//if err := os.WriteFile("/proc/sys/net/vrf/strict_mode", []byte("1"), 0644); err != nil {
	//	logger.ErrorLogger().Printf("Failed to enable VRF strict mode: %s", err)
	//}
	rtrSubnet, err := NewRoutingSubnet(logger.WithName("routing-subnet"), routingBaseNet, rtrVrfTable)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing subnet: %w", err)
	}
	cfg.DecapSID = rtrSubnet.DecapSID

	return &Software{
		appNet4Allocator:   net4Allocator,
		appNet6Allocator:   net6Allocator,
		tunNet6Allocator:   tunNet6Allocator,
		tunNet4Allocator:   tunNet4Allocator,
		tableAllocator:     tableAllocator,
		routingSubnet:      rtrSubnet,
		appSubnets:         make(map[client.ObjectKey][]AppSubnet),
		p4Targets:          make(map[client.ObjectKey]*P4TargetInstance),
		nodeConfigs:        make(map[client.ObjectKey]*NodeConfig),
		serviceChainRoutes: make(map[client.ObjectKey][]dataplane.SRv6Route),
		tunnelManager:      tunnelManager,
		cfg:                cfg,
		Client:             k8sClient,
	}, nil
}

func (d *Software) Close() error {
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
			err := subnet.AddSRv6Route(route.DestNet, route.FunctionIPs)
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

//func (d *Software) AddRoute(srcAppID types.UID, dstNet net.IPNet, sids []net.IP) error {
//	d.appMu.Lock()
//	defer d.appMu.Unlock()
//
//	subnets, exists := d.appSubnets[srcAppID]
//	if !exists {
//		return fmt.Errorf("application subnet for group %s does not exist", srcAppID)
//	}
//
//	for i := range subnets {
//		err := subnets[i].AddSRv6Route(dstNet, sids)
//		if err != nil {
//			return err
//		}
//	}
//	return nil
//}

//func (d *Software) RemoveRoute(srcAppID types.UID, dstNet net.IPNet) error {
//	d.appMu.Lock()
//	defer d.appMu.Unlock()
//
//	subnets, exists := d.appSubnets[srcAppID]
//	if !exists {
//		return fmt.Errorf("application subnet for group %s does not exist", srcAppID)
//	}
//
//	for i := range subnets {
//		err := subnets[i].DeleteSRv6Route(dstNet)
//		if err != nil {
//			return err
//		}
//	}
//	return nil
//}

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

	// From now on, if any errors happen when configuring this subnet, tear it down
	defer func() {
		if err != nil {
			logger.Error(err, "Failed to add application subnet")
			subnet.Teardown()
		}
	}()

	tunToRtrName, tunToRtrAddrs,
		tunToAppName, tunToAppAddrs,
		err := d.configureTunnelBetweenSubnets(d.routingSubnet, subnet)
	if err != nil {
		return nil, fmt.Errorf("failed to configure routing tunnel: %w", err)
	}

	err = d.routingSubnet.AddRouteToSubnet(subnet, tunToRtrAddrs, tunToAppName)
	if err != nil {
		return nil, fmt.Errorf("failed to install routes towards app subnet in routing subnet: %w", err)
	}

	err = subnet.AddDefaultRoute(tunToAppAddrs, tunToRtrName)
	if err != nil {
		return nil, fmt.Errorf("failed to install routes towards routing subnet in app subnet: %w", err)
	}

	existingSubnets, ok := d.appSubnets[appID]
	if !ok {
		d.appSubnets[appID] = []AppSubnet{*subnet}
	} else {
		d.appSubnets[appID] = append(existingSubnets, *subnet)
	}
	err = d.addSubnetToAppStatus(appID, appNet4, appNet6)
	if err != nil {
		return subnet, fmt.Errorf("failed to update application status with subnet info: %w", err)
	}
	return subnet, nil
}

func (d *Software) addSubnetToAppStatus(appID types.NamespacedName, appNet4 *net.IPNet, appNet6 *net.IPNet) error {
	var app = &corev1alpha1.Application{}
	err := d.Client.Get(context.Background(), appID, app)
	if err != nil {
		return fmt.Errorf("failed to get application: %w", err)
	}
	original := app.DeepCopy()
	if app.Status.Subnets[d.cfg.NodeName] == nil {
		app.Status.Subnets[d.cfg.NodeName] = make([]corev1alpha1.DualStackNetwork, 0)
	}
	app.Status.Subnets[d.cfg.NodeName] = append(app.Status.Subnets[d.cfg.NodeName], corev1alpha1.DualStackNetwork{
		IPv4Net: appNet4.String(),
		IPv6Net: appNet6.String(),
	})
	err = d.Client.Patch(context.Background(), app, client.MergeFrom(original))
	if err != nil {
		return fmt.Errorf("failed to patch application: %w", err)
	}
	return nil
}

func (d *Software) configureTunnelBetweenSubnets(
	rtrSubnet *RoutingSubnet, appSubnet *AppSubnet,
) (
	tunToRtrName string, tunToRtrAddrs netutils.DualStackAddress,
	tunToAppName string, tunToAppAddrs netutils.DualStackAddress,
	err error,
) {
	var ipv4Enabled = d.tunNet4Allocator != nil
	// Generate a /126 subnet from the tunnel range for the tunnel between router and app subnet
	tunNet6, err := d.tunNet6Allocator.Allocate()
	if err != nil {
		err = fmt.Errorf("failed to allocate tunnel subnet: %w", err)
		return
	}
	tunIP6Allocator, err := dataplane.NewIPv6Allocator(tunNet6)
	if err != nil {
		err = fmt.Errorf("failed to create IPv6 allocator for tunnel subnet: %w", err)
		return
	}
	var tunIP4Allocator *dataplane.IPv4Allocator
	if ipv4Enabled {
		var tunNet4 *net.IPNet
		tunNet4, err = d.tunNet4Allocator.Allocate()
		if err != nil {
			err = fmt.Errorf("failed to allocate tunnel subnet: %w", err)
		}
		tunIP4Allocator, err = dataplane.NewIPv4Allocator(tunNet4)
		if err != nil {
			err = fmt.Errorf("failed to create IPv4 allocator for tunnel subnet: %w", err)
			return
		}
	}

	tunToRtr := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("rt-tun-%d", appSubnet.Vrf.Table),
		},
		PeerName: fmt.Sprintf("app-tun-%d", rtrSubnet.Vrf.Table),
	}
	if err = netlink.LinkAdd(tunToRtr); err != nil {
		err = fmt.Errorf("failed to add veth pair for application subnet: %w", err)
		return
	}
	tunToRtrIPv6, err := tunIP6Allocator.Allocate()
	if err != nil {
		err = fmt.Errorf("failed to generate IPv6 address for router tunnel in application subnet: %w", err)
		return
	}
	if err = netlink.AddrAdd(tunToRtr, &netlink.Addr{IPNet: tunToRtrIPv6}); err != nil {
		err = fmt.Errorf("failed to add IPv6 address to router tunnel in application subnet: %w", err)
		return
	}
	var tunToRtrIPv4 *net.IPNet
	if ipv4Enabled {
		tunToRtrIPv4, err = tunIP4Allocator.Allocate()
		if err != nil {
			err = fmt.Errorf("failed to generate IPv4 address for router tunnel in application subnet: %w", err)
			return
		}
		if err = netlink.AddrAdd(tunToRtr, &netlink.Addr{IPNet: tunToRtrIPv4}); err != nil {
			err = fmt.Errorf("failed to add IPv4 address to router tunnel in application subnet: %w", err)
			return
		}
	}
	if err = netlink.LinkSetUp(tunToRtr); err != nil {
		err = fmt.Errorf("failed to set up router tunnel in application subnet: %w", err)
		return
	}

	tunToApp, err := netlink.LinkByName(tunToRtr.PeerName)
	if err != nil {
		err = fmt.Errorf("failed to get app tunnel in application subnet: %w", err)
		return
	}
	tunToAppIPv6, err := tunIP6Allocator.Allocate()
	if err != nil {
		err = fmt.Errorf("failed to generate IPv6 address for app tunnel in application subnet: %w", err)
		return
	}
	if err = netlink.AddrAdd(tunToApp, &netlink.Addr{IPNet: tunToAppIPv6}); err != nil {
		err = fmt.Errorf("failed to add address to app tunnel in application subnet: %w", err)
		return
	}
	var tunToAppIPv4 *net.IPNet
	if ipv4Enabled {
		tunToAppIPv4, err = tunIP4Allocator.Allocate()
		if err != nil {
			err = fmt.Errorf("failed to generate IPv4 address for app tunnel in application subnet: %w", err)
		}
		if err = netlink.AddrAdd(tunToApp, &netlink.Addr{IPNet: tunToAppIPv4}); err != nil {
			err = fmt.Errorf("failed to add IPv4 address to app tunnel in application subnet: %w", err)
		}
	}
	if err = netlink.LinkSetUp(tunToApp); err != nil {
		err = fmt.Errorf("failed to set up app tunnel in application subnet: %w", err)
		return
	}

	//// Get the mac addresses of the tunnels to create link-local addresses
	//tunToApp, err = netlink.LinkByName(tunToApp.Attrs().Name)
	//if err != nil {
	//	return fmt.Errorf("failed to get app tunnel link in application subnet: %w", err)
	//}
	//tunToRtr, err = netlink.LinkByName(tunToRtr.Attrs().Name)
	//if err != nil {
	//	return fmt.Errorf("failed to get router tunnel link in application subnet: %w", err)
	//}
	//rtTunLLAddr, err := vrfutil.CreateLinkLocalAddrFromMAC(rtTunnelLink.Attrs().HardwareAddr)
	//if err != nil {
	//	return fmt.Errorf("failed to get link-local address for router tunnel in application subnet: %w", err)
	//}
	//appTunLLAddr, err := vrfutil.CreateLinkLocalAddrFromMAC(appTunnelLink.Attrs().HardwareAddr)
	//if err != nil {
	//	return fmt.Errorf("failed to get link-local address for app tunnel in application subnet: %w", err)
	//}

	tunToRtrName = tunToRtr.Attrs().Name
	tunToAppName = tunToApp.Attrs().Name
	tunToRtrAddrs.IPv6 = tunToRtrIPv6.IP
	tunToAppAddrs.IPv6 = tunToAppIPv6.IP
	if ipv4Enabled {
		tunToRtrAddrs.IPv4 = tunToRtrIPv4.IP
		tunToAppAddrs.IPv4 = tunToAppIPv4.IP
	}
	return
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
		Gateways:  subnet.GatewayIPs,
		Bridge:    subnet.Bridge.Name,
		MTU:       DefaultMTU,
		IfaceName: ifaceName,
	}, nil
}

func (d *Software) DeleteAppInstance(_ string) error {
	// Nothing to do here for now
	return nil
}

func (d *Software) ConfigureP4TargetInstance(
	target *corev1alpha1.P4Target, _ string,
) (*dataplane.P4TargetConfig, error) {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	p4TargetConfig, exists := d.p4Targets[client.ObjectKeyFromObject(target)]
	if exists {
		return &dataplane.P4TargetConfig{
			IPv6Net:         *p4TargetConfig.TargetIPs.IPv6Net,
			ClusterIPv6CIDR: *d.cfg.ClusterCIDR.IPv6Net,
			IPv6Gateway:     d.routingSubnet.Gateway,
			Bridge:          d.routingSubnet.Bridge.Name,
			MTU:             DefaultMTU,
			IfaceName:       p4TargetConfig.ifaceName,
		}, nil
	}

	ips, err := d.routingSubnet.AllocateIPs()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IPs for application %s/%s: %w", target.Name, target.Namespace, err)
	}

	ifaceName, err := vrfutil.GenerateRandomName("nfr", 8)
	if err != nil {
		return nil, fmt.Errorf("failed to generate interface name: %w", err)
	}

	d.p4Targets[client.ObjectKeyFromObject(target)] = &P4TargetInstance{
		TargetIPs: ips,
		ifaceName: ifaceName,
	}

	return &dataplane.P4TargetConfig{
		IPv6Net:         *ips.IPv6Net,
		ClusterIPv6CIDR: *d.cfg.ClusterCIDR.IPv6Net,
		IPv6Gateway:     d.routingSubnet.Gateway,
		Bridge:          d.routingSubnet.Bridge.Name,
		MTU:             DefaultMTU,
		IfaceName:       ifaceName,
	}, nil
}

func (d *Software) DeleteP4TargetInstance(_ string) error {
	// Nothing to do here by now
	return nil
}

func (d *Software) UpdateNodeRoutes(node *infrav1alpha1.LoomNode) error {
	d.nodeMu.Lock()
	defer d.nodeMu.Unlock()
	nodeConfig, exists := d.nodeConfigs[client.ObjectKeyFromObject(node)]
	if !exists {
		return nil
	}
	if nodeConfig.LastResourceVersion >= node.ResourceVersion {
		return nil
	}
	if len(node.Spec.NodeCIDRs) == 0 {
		// Node hasn't got a CIDR yet
		return nil
	}
	tunName, err := d.tunnelManager.GetTunnelInterface(node.Name)
	if err != nil {
		return fmt.Errorf("failed to get tunnel interface for node %s: %w", node.Name, err)
	}
	tunLink, err := netlink.LinkByName(tunName)
	if err != nil {
		return fmt.Errorf("failed to get tun link for node %s: %w", node.Name, err)
	}
	err = netlink.LinkSetMaster(tunLink, d.routingSubnet.Bridge)
	if err != nil {
		return fmt.Errorf("failed to set master for tunnel interface for node %s: %w", node.Name, err)
	}
	cidrs, err := vrfutil.ParseDualStackNetworkFromStrings(node.Spec.NodeCIDRs)
	if err != nil {
		return fmt.Errorf("failed to parse CIDRs for node %s: %w", node.Name, err)
	}
	addrs, err := vrfutil.GetDualStackAddressFromLink(tunLink)
	if err != nil {
		return fmt.Errorf("failed to get addresses from tunnel interface for node %s: %w", node.Name, err)
	}
	if addrs.IPv6 == nil {
		tunAddr, err := d.routingSubnet.IP6Allocator.Allocate()
		if err != nil {
			return fmt.Errorf("failed to allocate IPv6 for node %s: %w", node.Name, err)
		}
		err = netlink.AddrAdd(tunLink, &netlink.Addr{IPNet: tunAddr})
		if err != nil {
			return fmt.Errorf("failed to add IPv6 address to tunnel interface for node %s: %w", node.Name, err)
		}
	}
	err = d.routingSubnet.AddRoute(cidrs, addrs, tunLink.Attrs().Name)
	if err != nil {
		return fmt.Errorf("failed to add route for node %s: %w", node.Name, err)
	}
	d.nodeConfigs[client.ObjectKeyFromObject(node)] = &NodeConfig{
		LastResourceVersion: node.ResourceVersion,
		Route: netutils.DualStackRoute{
			IPv4Route: netutils.Route{
				Destination: cidrs.IPv4Net,
				Gateway:     addrs.IPv4,
			},
			IPv6Route: netutils.Route{
				Destination: cidrs.IPv6Net,
				Gateway:     addrs.IPv6,
			},
		},
	}
	return nil
}

func (d *Software) RemoveNodeRoutes(node client.ObjectKey) (err error) {
	d.nodeMu.Lock()
	defer d.nodeMu.Unlock()

	nodeConfig, exists := d.nodeConfigs[node]
	if !exists {
		return nil
	}
	defer func() {
		if err != nil {
			delete(d.nodeConfigs, node)
		}
	}()
	route := &nodeConfig.Route
	if route.IsEmpty() {
		return nil
	}
	dst := netutils.DualStackNetwork{
		IPv4Net: route.IPv4Route.Destination,
		IPv6Net: route.IPv6Route.Destination,
	}
	err = d.routingSubnet.RemoveRoute(dst)
	if err != nil {
		return fmt.Errorf("failed to remove route for node %s: %w", node.Name, err)
	}
	return nil
}

func (d *Software) UpdateAppRoutes(app *corev1alpha1.Application) error {
	return nil
}

func (d *Software) RemoveAppRoutes(app client.ObjectKey) error {
	return nil
}

func (d *Software) UpdateP4TargetRoutes(target *corev1alpha1.P4Target) error {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	targetInstance, exists := d.p4Targets[client.ObjectKeyFromObject(target)]
	if !exists {
		// Either a P4Target that wasn't configured yet, or it belongs to another node.
		// TODO: This verification does not work for Hardware-based P4Targets that don't belong to any node.
		//  Refactor the code to properly handle this case.
		return nil
	}
	if len(target.Status.TargetIPs) == 0 || len(target.Status.DriverIPs) == 0 || target.Spec.NfCIDR == "" {
		// We don't have enough information about the object yet to update its routes.
		return nil
	}
	targetAddrs, err := vrfutil.ParseDualStackGatewayFromStrings(target.Status.TargetIPs)
	if err != nil {
		return fmt.Errorf("failed to parse target IPs for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	nfCIDR, err := vrfutil.ParseDualStackNetworkFromStrings([]string{target.Spec.NfCIDR})
	if err != nil {
		return fmt.Errorf("failed to parse nf CIDR for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	err = d.routingSubnet.AddRoute(nfCIDR, targetAddrs, targetInstance.ifaceName)
	if err != nil {
		return fmt.Errorf("failed to add route for P4Target %s/%s: %w", target.Name, target.Namespace, err)
	}
	return nil
}

func (d *Software) RemoveP4TargetRoutes(target client.ObjectKey) error {
	return nil
}

func (d *Software) cleanupAppSubnet(subnet *AppSubnet) {
	logger := d.log
	if subnet.VethBridgeVRF != nil {
		if err := netlink.LinkDel(subnet.VethBridgeVRF); err != nil {
			logger.Error(err, "failed to delete veth", "veth", subnet.VethBridgeVRF.Attrs().Name)
		}
	}
	if subnet.Bridge != nil {
		if err := netlink.LinkDel(subnet.Bridge); err != nil {
			logger.Error(err, "failed to delete bridge", "bridge", subnet.Bridge.Attrs().Name)
		}
	}
	if subnet.Tunnel != nil {
		if err := netlink.LinkDel(subnet.Tunnel); err != nil {
			logger.Error(err, "failed to delete tunnel", "tunnel", subnet.Tunnel.Attrs().Name)
		}
	}
	if subnet.Vrf != nil {
		if err := netlink.LinkDel(subnet.Vrf); err != nil {
			logger.Error(err, "failed to delete VRF", "vrf", subnet.Vrf.Attrs().Name)
		}
	}
}

//func (d *Software) cleanupVNFSubnet(subnet *vnfSubnet) {
//	if subnet.VethBridgeVRF != nil {
//		if err := netlink.LinkDel(subnet.VethBridgeVRF); err != nil {
//			logger.ErrorLogger().Printf("failed to delete veth %s: %v", subnet.VethBridgeVRF.Attrs().Name, err)
//		}
//	}
//	if subnet.bridge != nil {
//		if err := netlink.LinkDel(subnet.bridge); err != nil {
//			logger.ErrorLogger().Printf("failed to delete bridge %s: %v", subnet.bridge.Attrs().Name, err)
//		}
//	}
//}

//func (*Software) regenerateVNFSubnetNextHops(subnet *vnfSubnet) ([]*netlink.NexthopInfo, error) {
//	link, err := netlink.LinkByName(subnet.VethBridgeVRF.PeerName)
//	if err != nil {
//		logger.ErrorLogger().Printf("failed to get veth %s: %v", subnet.VethBridgeVRF.Attrs().Name, err)
//		return nil, err
//	}
//
//	var nextHops []*netlink.NexthopInfo
//	for _, ip := range subnet.Instances {
//		nextHops = append(nextHops, &netlink.NexthopInfo{
//			Gw:        ip,
//			LinkIndex: link.Attrs().Index,
//		})
//	}
//	return nextHops, nil
//}
//
//func (d *Software) updateVNFSubnetSIDRoute(sid *net.IPNet, nextHops []*netlink.NexthopInfo) error {
//	var route *netlink.Route
//	routes, err := netlink.RouteListFiltered(nl.FAMILY_V6, &netlink.Route{
//		Dst:   sid,
//		Table: int(d.routerVrf.Table),
//	}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
//	logger.DebugLogger().Printf("Existing routes for VNF subnet %s: %v", sid.String(), routes)
//	if err != nil {
//		return fmt.Errorf("failed to list routes for VNF subnet %s: %w", sid.String(), err)
//	}
//	if len(routes) == 0 {
//		logger.DebugLogger().Printf("No existing route for VNF subnet %s, creating a new one", sid.String())
//		// No existing route, create a new one
//		route = &netlink.Route{
//			Dst:       sid,
//			Table:     int(d.routerVrf.Table),
//			MultiPath: nextHops,
//		}
//		logger.DebugLogger().Printf("Creating new route for VNF subnet %s: %v", sid.String(), route)
//	} else {
//		logger.DebugLogger().Printf("Found existing route(s) for VNF subnet %s: %v", sid.String(), routes)
//		logger.DebugLogger().Printf("Updating existing route for VNF subnet %s: %v", sid.String(), routes[0])
//		// Update existing route
//		route = &routes[0]
//		route.MultiPath = nextHops
//		logger.DebugLogger().Printf("Updated route for VNF subnet %s: %v", sid.String(), route)
//	}
//	if err := netlink.RouteReplace(route); err != nil {
//		return fmt.Errorf("failed to update route for VNF subnet %s: %w", sid.String(), err)
//	}
//	return nil
//}

//func (d *Software) setUpRouterSubnet() error {
//	routerVrfTable, err := d.tableAllocator.Allocate()
//	if err != nil {
//		return fmt.Errorf("failed to allocate router VRF table ID: %w", err)
//	}
//	routerVrf := &netlink.Vrf{
//		LinkAttrs: netlink.LinkAttrs{
//			Name: "router-vrf",
//		},
//		Table: uint32(routerVrfTable),
//	}
//	if err := netlink.LinkAdd(routerVrf); err != nil {
//		return fmt.Errorf("failed to add router VRF: %w", err)
//	}
//	if err := netlink.LinkSetUp(routerVrf); err != nil {
//		return fmt.Errorf("failed to set up router VRF: %w", err)
//	}
//
//	decapIface := &netlink.Dummy{
//		LinkAttrs: netlink.LinkAttrs{
//			Name: "decap0",
//		},
//	}
//	if err := netlink.LinkAdd(decapIface); err != nil {
//		return fmt.Errorf("failed to add decap interface: %w", err)
//	}
//	if err := netlink.LinkSetMaster(decapIface, routerVrf); err != nil {
//		return fmt.Errorf("failed to set master for decap interface: %w", err)
//	}
//	if err := netlink.LinkSetUp(decapIface); err != nil {
//		return fmt.Errorf("failed to set up decap interface: %w", err)
//	}
//
//	decapSid, err := d.routingIP6Allocator.Allocate()
//	if err != nil {
//		return fmt.Errorf("failed to allocate decap SID: %w", err)
//	}
//	// ip -6 route add <decap sid> table <router vrf table> encap seg6local action End.DT6 table <router vrf table> dev <decap iface>
//	var flags_end_dt6_encaps [nl.SEG6_LOCAL_MAX]bool
//	flags_end_dt6_encaps[nl.SEG6_LOCAL_ACTION] = true
//	flags_end_dt6_encaps[nl.SEG6_LOCAL_TABLE] = true
//	decapRoute := &netlink.Route{
//		Dst:   decapSid,
//		Table: int(routerVrf.Table),
//		Encap: &netlink.SEG6LocalEncap{
//			Flags:  flags_end_dt6_encaps,
//			Action: nl.SEG6_LOCAL_ACTION_END_DT6,
//			Table:  int(routerVrf.Table),
//		},
//		LinkIndex: decapIface.Attrs().Index,
//	}
//	if err := netlink.RouteAdd(decapRoute); err != nil {
//		logger.ErrorLogger().Printf("Failed to execute `ip -6 route add %s table %d encap seg6local action End.DT6 vrftable %d dev %s`: %s", decapSid.String(), routerVrf.Table, routerVrf.Table, decapIface.Attrs().Name, err)
//		return fmt.Errorf("failed to add decap route: %w", err)
//	}
//	d.routerVrf = routerVrf
//
//	globalConfig, err := env.Config()
//	if err != nil {
//		return fmt.Errorf("failed to get global config: %w", err)
//	}
//	globalConfig.DecapSID = decapSid
//	return nil
//}

//func (d *Software) tearDownRouterSubnet() {
//	routerVrf, err := netlink.LinkByName("router-vrf")
//	if err == nil {
//		netlink.LinkDel(routerVrf)
//	}
//	decapIface, err := netlink.LinkByName("decap0")
//	if err == nil {
//		netlink.LinkDel(decapIface)
//	}
//}

func getFirstSubnetWithAvailableIPs(subnets []AppSubnet) (*AppSubnet, error) {
	for _, subnet := range subnets {
		if subnet.HasIPsAvailable() {
			return &subnet, nil
		}
	}
	return nil, fmt.Errorf("no available subnets found")
}
