package dataplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/logger"
	"net"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

type Software struct {
	appSubnets map[uuid.UUID]*appSubnet
	appMu      sync.Mutex
	vnfSubnets map[uuid.UUID]*vnfSubnet
	vnfMu      sync.Mutex
	routerVrf  *netlink.Vrf

	appNetAllocator *Subnet6Allocator
	vnfNetAllocator *Subnet6Allocator
	sidNetAllocator *Subnet6Allocator
	tunNetAllocator *Subnet6Allocator
	tableAllocator  *TableAllocator
}

type appSubnet struct {
	network       *net.IPNet
	gatewayIP     net.IP
	bridge        *netlink.Bridge
	VethBridgeVRF *netlink.Veth
	Vrf           *netlink.Vrf
	Tunnel        *netlink.Veth
	IPAllocator   *IPv6Allocator
}

func (as *appSubnet) Network() *net.IPNet {
	return as.network
}
func (as *appSubnet) GatewayIP() net.IP {
	return as.gatewayIP
}
func (as *appSubnet) Bridge() string {
	return as.bridge.Attrs().Name
}

type vnfSubnet struct {
	network       *net.IPNet
	gatewayIP     net.IP
	sids          []net.IPNet
	bridge        *netlink.Bridge
	VethBridgeVRF *netlink.Veth
	IPAllocator   *IPv6Allocator
	Instances     map[uuid.UUID]net.IP
	mu            sync.Mutex
}

func (vs *vnfSubnet) Network() *net.IPNet {
	return vs.network
}
func (vs *vnfSubnet) GatewayIP() net.IP {
	return vs.gatewayIP
}
func (vs *vnfSubnet) SIDs() []net.IPNet {
	return vs.sids
}
func (vs *vnfSubnet) Bridge() string {
	return vs.bridge.Attrs().Name
}

func NewSoftware(sidRange *net.IPNet, appRange *net.IPNet, vnfRange *net.IPNet, tunRange *net.IPNet) (Manager, error) {
	// Enable IPv6 forwarding. Required in host namespace to route packets between interfaces.
	if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/forwarding", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
	}
	// TODO: If IPv4 support is implemented sometime in the future, uncomment this.
	// if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0644); err != nil {
	// 	return nil, fmt.Errorf("failed to enable IPv4 forwarding: %w", err)
	// }
	// Enable SRv6 globally. Required in host namespace to decapsulate SRv6 packets.
	if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/seg6_enabled", []byte("1"), 0644); err != nil {
		return nil, fmt.Errorf("failed to set seg6_enabled: %w", err)
	}

	sidNetAllocator, err := NewSubnet6Allocator(sidRange, 128)
	if err != nil {
		return nil, fmt.Errorf("failed to create SID subnet allocator: %w", err)
	}
	appNetAllocator, err := NewSubnet6Allocator(appRange, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to create application subnet allocator: %w", err)
	}
	vnfNetAllocator, err := NewSubnet6Allocator(vnfRange, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to create VNF subnet allocator: %w", err)
	}
	tunNetAllocator, err := NewSubnet6Allocator(tunRange, 126)
	if err != nil {
		return nil, fmt.Errorf("failed to create tunnel subnet allocator: %w", err)
	}
	tableAllocator, err := NewTableAllocator(1000)
	if err != nil {
		return nil, fmt.Errorf("failed to create table allocator: %w", err)
	}

	dataplane := &Software{
		sidNetAllocator: sidNetAllocator,
		appNetAllocator: appNetAllocator,
		vnfNetAllocator: vnfNetAllocator,
		tunNetAllocator: tunNetAllocator,
		tableAllocator:  tableAllocator,
		appSubnets:      make(map[uuid.UUID]*appSubnet),
		vnfSubnets:      make(map[uuid.UUID]*vnfSubnet),
	}
	err = dataplane.setUpRouterSubnet()
	if err != nil {
		dataplane.tearDownRouterSubnet()
		return nil, fmt.Errorf("failed to set up router subnet: %w", err)
	}

	return dataplane, nil
}

func (d *Software) Close() error {
	// Delete the router subnet
	d.tearDownRouterSubnet()

	// Delete all application and VNF subnets
	d.appMu.Lock()
	defer d.appMu.Unlock()
	for _, subnet := range d.appSubnets {
		d.cleanupAppSubnet(subnet)
	}

	d.vnfMu.Lock()
	defer d.vnfMu.Unlock()
	for _, subnet := range d.vnfSubnets {
		d.cleanupVNFSubnet(subnet)
	}
	return nil
}

func (d *Software) AddRoute(srcAppGroup uuid.UUID, dstNet net.IPNet, sids []net.IP) error {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	subnet, exists := d.appSubnets[srcAppGroup]
	if !exists {
		return fmt.Errorf("application subnet for group %s does not exist", srcAppGroup)
	}

	// ip route add <dstNet> vrf <subnet.Vrf> encap seg6 mode encap segs <sids> dev <subnet.tunnel>
	route := &netlink.Route{
		Dst:   &dstNet,
		Table: int(subnet.Vrf.Table),
		Encap: &netlink.SEG6Encap{
			Mode:     nl.SEG6_IPTUN_MODE_ENCAP,
			Segments: sids,
		},
		LinkIndex: subnet.Tunnel.Attrs().Index,
	}

	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add route for app group %s to %s with segs %s: %w", srcAppGroup, dstNet.String(), sids, err)
	}

	return nil
}

func (d *Software) RemoveRoute(srcAppGroup uuid.UUID, dstNet string) error {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	subnet, exists := d.appSubnets[srcAppGroup]
	if !exists {
		return fmt.Errorf("application subnet for group %s does not exist", srcAppGroup)
	}

	// Parse the destination network
	dst, err := netlink.ParseAddr(dstNet)
	if err != nil {
		return fmt.Errorf("failed to parse destination network: %w", err)
	}

	route := &netlink.Route{
		Dst:   dst.IPNet,
		Table: int(subnet.Vrf.Table),
	}
	if err := netlink.RouteDel(route); err != nil {
		return fmt.Errorf("failed to delete route for app group %s to %s: %w", srcAppGroup, dst.IPNet.String(), err)
	}
	return nil
}

// Creates a subnet into the dataplane and returns the configured bridge name.
func (d *Software) AddApplicationSubnet(appGroupID uuid.UUID) (AppSubnet, error) {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	_, exists := d.appSubnets[appGroupID]
	if exists {
		return nil, fmt.Errorf("subnet for app group %s already exists", appGroupID)
	}
	subnet := &appSubnet{}

	AppNetwork, err := d.appNetAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate application subnet: %w", err)
	}
	subnet.network = AppNetwork

	ipAllocator, err := NewIPv6Allocator(AppNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 allocator for application subnet: %w", err)
	}
	subnet.IPAllocator = ipAllocator

	gatewayIPNet, err := ipAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate gateway IP for application subnet: %w", err)
	}
	subnet.gatewayIP = gatewayIPNet.IP

	tableID, err := d.tableAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate table ID for application subnet: %w", err)
	}

	// Generate a /126 subnet from the tunnel range for the tunnel between router and app subnet
	tunNet, err := d.tunNetAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate tunnel subnet: %w", err)
	}
	tunIPAllocator, err := NewIPv6Allocator(tunNet)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 allocator for tunnel subnet: %w", err)
	}

	appVrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("vrf-%d", tableID),
		},
		Table: uint32(tableID),
	}
	if err := netlink.LinkAdd(appVrf); err != nil {
		return nil, fmt.Errorf("failed to add VRF for application subnet: %w", err)
	}
	subnet.Vrf = appVrf
	if err := netlink.LinkSetUp(appVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up VRF for application subnet: %w", err)
	}

	routerTunnel := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: fmt.Sprintf("rt-tun-%d", tableID),
		},
		PeerName: fmt.Sprintf("app-tun-%d", tableID),
	}
	if err := netlink.LinkAdd(routerTunnel); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add veth pair for application subnet: %w", err)
	}
	subnet.Tunnel = routerTunnel
	if err := netlink.LinkSetMaster(routerTunnel, appVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set master for router tunnel in application subnet: %w", err)
	}
	rtTunAddr, err := tunIPAllocator.Allocate()
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to generate address for router tunnel in application subnet: %w", err)
	}
	if err := netlink.AddrAdd(routerTunnel, &netlink.Addr{IPNet: rtTunAddr}); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add address to router tunnel in application subnet: %w", err)
	}
	if err := netlink.LinkSetUp(routerTunnel); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up router tunnel in application subnet: %w", err)
	}

	appTunnel, err := netlink.LinkByName(routerTunnel.PeerName)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get app tunnel in application subnet: %w", err)
	}
	if err := netlink.LinkSetMaster(appTunnel, d.routerVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set master for app tunnel in router subnet: %w", err)
	}
	appTunAddr, err := tunIPAllocator.Allocate()
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to generate address for app tunnel in application subnet: %w", err)
	}
	if err := netlink.AddrAdd(appTunnel, &netlink.Addr{IPNet: appTunAddr}); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add address to app tunnel in application subnet: %w", err)
	}
	if err := netlink.LinkSetUp(appTunnel); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up app tunnel in application subnet: %w", err)
	}

	// Get the mac addresses of the tunnels to create link-local addresses
	appTunnelLink, err := netlink.LinkByName(appTunnel.Attrs().Name)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get app tunnel link in application subnet: %w", err)
	}
	rtTunnelLink, err := netlink.LinkByName(routerTunnel.Attrs().Name)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get router tunnel link in application subnet: %w", err)
	}
	rtTunLLAddr, err := d.createLinkLocalAddrFromMAC(rtTunnelLink.Attrs().HardwareAddr)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get link-local address for router tunnel in application subnet: %w", err)
	}
	appTunLLAddr, err := d.createLinkLocalAddrFromMAC(appTunnelLink.Attrs().HardwareAddr)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get link-local address for app tunnel in application subnet: %w", err)
	}

	// Create a route in the router VRF to reach the application subnet using
	// the app tunnel as the outgoing interface.
	routeToApp := &netlink.Route{
		Dst:       AppNetwork,
		Gw:        rtTunLLAddr.IP,
		Table:     int(d.routerVrf.Table),
		LinkIndex: appTunnel.Attrs().Index,
	}
	if err := netlink.RouteAdd(routeToApp); err != nil {
		d.cleanupAppSubnet(subnet)
		logger.DebugLogger().Printf("failed to execute: `ip -6 route add %s table %d dev %s`", AppNetwork.String(), d.routerVrf.Table, appTunnel.Attrs().Name)
		return nil, fmt.Errorf("failed to add route to application subnet in router VRF: %w", err)
	}

	// Create a route in the application VRF to reach the router subnet using
	// the router tunnel as the outgoing interface.
	routeToRouter := &netlink.Route{
		Dst: &net.IPNet{
			IP:   net.ParseIP("::"),
			Mask: net.CIDRMask(0, 128),
		},
		Gw:        appTunLLAddr.IP,
		Table:     int(appVrf.Table),
		LinkIndex: routerTunnel.Attrs().Index,
	}
	if err := netlink.RouteAdd(routeToRouter); err != nil {
		d.cleanupAppSubnet(subnet)
		logger.DebugLogger().Printf("failed to execute: `ip -6 route add ::/0 table %d dev %s`", appVrf.Table, routerTunnel.Attrs().Name)
		return nil, fmt.Errorf("failed to add route to router subnet in application VRF: %w", err)
	}

	// Create a bridge for this subnet
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to generate random bridge name: %w", err)
	}
	hexStr := hex.EncodeToString(b)
	bridgeName := fmt.Sprintf("br-%s", hexStr)

	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:        bridgeName,
			MasterIndex: appVrf.Attrs().Index,
		},
	}
	if err := netlink.LinkAdd(bridge); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add bridge %s: %w", bridgeName, err)
	}
	subnet.bridge = bridge
	if err := netlink.LinkSetUp(bridge); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up bridge %s: %w", bridgeName, err)
	}

	vethFromBridgeToVrf := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:        fmt.Sprintf("%s-%d", hexStr, appVrf.Table),
			MasterIndex: bridge.Attrs().Index,
		},
		PeerName: fmt.Sprintf("%d-%s", appVrf.Table, hexStr),
	}
	if err := netlink.LinkAdd(vethFromBridgeToVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add veth from bridge to vnf %s: %w", vethFromBridgeToVrf.Attrs().Name, err)
	}
	subnet.VethBridgeVRF = vethFromBridgeToVrf
	if err := netlink.LinkSetUp(vethFromBridgeToVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up veth from bridge to vnf %s: %w", vethFromBridgeToVrf.Attrs().Name, err)
	}

	vethFromVrfToBridge, err := netlink.LinkByName(vethFromBridgeToVrf.PeerName)
	if err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to get veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.LinkSetMaster(vethFromVrfToBridge, appVrf); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set master for veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.AddrAdd(vethFromVrfToBridge, &netlink.Addr{IPNet: gatewayIPNet}); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to add IP address to veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.LinkSetUp(vethFromVrfToBridge); err != nil {
		d.cleanupAppSubnet(subnet)
		return nil, fmt.Errorf("failed to set up veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}

	d.appSubnets[appGroupID] = subnet
	return subnet, nil
}

func (d *Software) RemoveApplicationSubnet(appGroupID uuid.UUID) {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	subnet, exists := d.appSubnets[appGroupID]
	if !exists {
		return
	}
	d.cleanupAppSubnet(subnet)
	delete(d.appSubnets, appGroupID)
}

func (d *Software) AddApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) (*net.IPNet, string, error) {
	d.appMu.Lock()
	defer d.appMu.Unlock()

	subnet, exists := d.appSubnets[appGroupID]
	if !exists {
		return nil, "", fmt.Errorf("application subnet for group %s does not exist", appGroupID)
	}
	instanceIPNet, err := subnet.IPAllocator.Allocate()
	if err != nil {
		return nil, "", fmt.Errorf("failed to allocate IP for application instance %s: %w", appInstanceID, err)
	}

	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random bytes for VNF interface name: %w", err)
	}
	ifaceName := fmt.Sprintf("nfr-%s", hex.EncodeToString(randomBytes))

	return instanceIPNet, ifaceName, nil
}

func (d *Software) RemoveApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) error {
	// Nothing to do here for now
	return nil
}

func (d *Software) AddVNFSubnet(vnfGroupID uuid.UUID, sidAmount int) (VNFSubnet, error) {
	d.vnfMu.Lock()
	defer d.vnfMu.Unlock()

	_, exists := d.vnfSubnets[vnfGroupID]
	if exists {
		return nil, fmt.Errorf("subnet for VNF group %s already exists", vnfGroupID)
	}
	subnet := &vnfSubnet{}

	vnfNetwork, err := d.vnfNetAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate VNF subnet: %w", err)
	}
	subnet.network = vnfNetwork

	ipAllocator, err := NewIPv6Allocator(vnfNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPv6 allocator for VNF subnet: %w", err)
	}
	subnet.IPAllocator = ipAllocator

	gatewayIPNet, err := ipAllocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate gateway IP for VNF subnet: %w", err)
	}
	subnet.gatewayIP = gatewayIPNet.IP

	sids := make([]net.IPNet, sidAmount)
	for i := 0; i < sidAmount; i++ {
		sidNet, err := d.sidNetAllocator.Allocate()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate SID for VNF subnet: %w", err)
		}
		sids[i] = *sidNet
	}
	subnet.sids = sids

	// Create a bridge in the router vrf
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bridge name: %w", err)
	}
	hexStr := hex.EncodeToString(b)
	bridgeName := fmt.Sprintf("br-%s", hexStr)

	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: bridgeName,
		},
	}
	if err := netlink.LinkAdd(bridge); err != nil {
		return nil, fmt.Errorf("failed to add bridge %s: %w", bridgeName, err)
	}
	subnet.bridge = bridge
	if err := netlink.LinkSetUp(bridge); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to set up bridge %s: %w", bridgeName, err)
	}

	vethFromBridgeToVrf := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name:        fmt.Sprintf("%s-%d", hexStr, d.routerVrf.Table),
			MasterIndex: bridge.Attrs().Index,
		},
		PeerName: fmt.Sprintf("%d-%s", d.routerVrf.Table, hexStr),
	}
	if err := netlink.LinkAdd(vethFromBridgeToVrf); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to add veth from bridge to vnf %s: %w", vethFromBridgeToVrf.Attrs().Name, err)
	}
	subnet.VethBridgeVRF = vethFromBridgeToVrf
	if err := netlink.LinkSetUp(vethFromBridgeToVrf); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to set up veth from bridge to vnf %s: %w", vethFromBridgeToVrf.Attrs().Name, err)
	}

	vethFromVrfToBridge, err := netlink.LinkByName(vethFromBridgeToVrf.PeerName)
	if err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to get veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.LinkSetMaster(vethFromVrfToBridge, d.routerVrf); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to set master for veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.AddrAdd(vethFromVrfToBridge, &netlink.Addr{IPNet: gatewayIPNet}); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to add IP address to veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}
	if err := netlink.LinkSetUp(vethFromVrfToBridge); err != nil {
		d.cleanupVNFSubnet(subnet)
		return nil, fmt.Errorf("failed to set up veth from vrf to bridge %s: %w", vethFromBridgeToVrf.PeerName, err)
	}

	subnet.Instances = make(map[uuid.UUID]net.IP)
	d.vnfSubnets[vnfGroupID] = subnet
	return subnet, nil
}

func (d *Software) RemoveVNFSubnet(vnfGroupID uuid.UUID) {
	d.vnfMu.Lock()
	defer d.vnfMu.Unlock()

	subnet, exists := d.vnfSubnets[vnfGroupID]
	if !exists {
		return
	}
	d.cleanupVNFSubnet(subnet)
	delete(d.vnfSubnets, vnfGroupID)
}

func (d *Software) AddVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) (*net.IPNet, string, error) {
	d.vnfMu.Lock()
	defer d.vnfMu.Unlock()

	subnet, exists := d.vnfSubnets[vnfGroupID]
	if !exists {
		return nil, "", fmt.Errorf("VNF subnet for group %s does not exist", vnfGroupID)
	}
	subnet.mu.Lock()
	defer subnet.mu.Unlock()

	vnfIPNet, err := subnet.IPAllocator.Allocate()
	if err != nil {
		return nil, "", fmt.Errorf("failed to allocate IP for VNF instance %s: %w", vnfInstanceID, err)
	}

	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate random bytes for VNF interface name: %w", err)
	}
	ifaceName := fmt.Sprintf("nfr-%s", hex.EncodeToString(randomBytes))

	subnet.Instances[vnfInstanceID] = vnfIPNet.IP
	nextHops, err := d.regenerateVNFSubnetNextHops(subnet)
	if err != nil {
		return nil, "", fmt.Errorf("failed to regenerate vnf subnet next hops: %w", err)
	}
	logger.DebugLogger().Printf("VNF subnet %s next hops after adding instance %s: %v", subnet.network.String(), vnfInstanceID, nextHops)

	for _, sid := range subnet.sids {
		err = d.updateVNFSubnetSIDRoute(&sid, nextHops)
		if err != nil {
			return nil, "", fmt.Errorf("failed to update vnf subnet routes: %w", err)
		}
	}
	return vnfIPNet, ifaceName, nil
}

func (d *Software) RemoveVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) error {
	d.vnfMu.Lock()
	defer d.vnfMu.Unlock()

	subnet, exists := d.vnfSubnets[vnfGroupID]
	if !exists {
		return fmt.Errorf("VNF subnet for group %s does not exist", vnfGroupID)
	}
	subnet.mu.Lock()
	defer subnet.mu.Unlock()
	delete(subnet.Instances, vnfInstanceID)
	nextHops, err := d.regenerateVNFSubnetNextHops(subnet)
	if err != nil {
		return fmt.Errorf("failed to regenerate vnf subnet next hops: %w", err)
	}
	for _, sid := range subnet.sids {
		err = d.updateVNFSubnetSIDRoute(&sid, nextHops)
		if err != nil {
			return fmt.Errorf("failed to update vnf subnet routes: %w", err)
		}
	}
	return nil
}

func (d *Software) createLinkLocalAddrFromMAC(mac net.HardwareAddr) (*net.IPNet, error) {
	var eui64 []byte
	switch len(mac) {
	case 6:
		// Convert 48-bit MAC to EUI-64 by inserting ff:fe in the middle
		eui64 = make([]byte, 8)
		copy(eui64[0:3], mac[0:3])
		eui64[3] = 0xff
		eui64[4] = 0xfe
		copy(eui64[5:], mac[3:6])
	case 8:
		// Already EUI-64
		eui64 = make([]byte, 8)
		copy(eui64, mac)
	default:
		return nil, fmt.Errorf("expected MAC size is 6 bytes (48-bit) or 8 bytes (EUI-64), instead got %d bytes", len(mac))
	}

	// Flip the universal/local bit (the 7th bit of the first byte)
	eui64[0] ^= 0x02

	// Build IPv6 address: fe80::/64 + interface ID (EUI-64)
	ip := make(net.IP, net.IPv6len) // 16 bytes
	ip[0] = 0xfe
	ip[1] = 0x80
	// bytes 2..7 are zero
	ip[2] = 0x00
	ip[3] = 0x00
	ip[4] = 0x00
	ip[5] = 0x00
	ip[6] = 0x00
	ip[7] = 0x00
	copy(ip[8:], eui64) // interface ID placed in lower 64 bits

	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(64, 128),
	}, nil
}

func (d *Software) cleanupAppSubnet(subnet *appSubnet) {
	if subnet.VethBridgeVRF != nil {
		if err := netlink.LinkDel(subnet.VethBridgeVRF); err != nil {
			logger.ErrorLogger().Printf("failed to delete veth %s: %v", subnet.VethBridgeVRF.Attrs().Name, err)
		}
	}
	if subnet.bridge != nil {
		if err := netlink.LinkDel(subnet.bridge); err != nil {
			logger.ErrorLogger().Printf("failed to delete bridge %s: %v", subnet.bridge.Attrs().Name, err)
		}
	}
	if subnet.Tunnel != nil {
		if err := netlink.LinkDel(subnet.Tunnel); err != nil {
			logger.ErrorLogger().Printf("failed to delete tunnel %s: %v", subnet.Tunnel.Attrs().Name, err)
		}
	}
	if subnet.Vrf != nil {
		if err := netlink.LinkDel(subnet.Vrf); err != nil {
			logger.ErrorLogger().Printf("failed to delete VRF %s: %v", subnet.Vrf.Attrs().Name, err)
		}
	}
}

func (d *Software) cleanupVNFSubnet(subnet *vnfSubnet) {
	if subnet.VethBridgeVRF != nil {
		if err := netlink.LinkDel(subnet.VethBridgeVRF); err != nil {
			logger.ErrorLogger().Printf("failed to delete veth %s: %v", subnet.VethBridgeVRF.Attrs().Name, err)
		}
	}
	if subnet.bridge != nil {
		if err := netlink.LinkDel(subnet.bridge); err != nil {
			logger.ErrorLogger().Printf("failed to delete bridge %s: %v", subnet.bridge.Attrs().Name, err)
		}
	}
}

func (*Software) regenerateVNFSubnetNextHops(subnet *vnfSubnet) ([]*netlink.NexthopInfo, error) {
	link, err := netlink.LinkByName(subnet.VethBridgeVRF.PeerName)
	if err != nil {
		logger.ErrorLogger().Printf("failed to get veth %s: %v", subnet.VethBridgeVRF.Attrs().Name, err)
		return nil, err
	}

	var nextHops []*netlink.NexthopInfo
	for _, ip := range subnet.Instances {
		nextHops = append(nextHops, &netlink.NexthopInfo{
			Gw:        ip,
			LinkIndex: link.Attrs().Index,
		})
	}
	return nextHops, nil
}

func (d *Software) updateVNFSubnetSIDRoute(sid *net.IPNet, nextHops []*netlink.NexthopInfo) error {
	var route *netlink.Route
	routes, err := netlink.RouteListFiltered(nl.FAMILY_V6, &netlink.Route{
		Dst:   sid,
		Table: int(d.routerVrf.Table),
	}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
	logger.DebugLogger().Printf("Existing routes for VNF subnet %s: %v", sid.String(), routes)
	if err != nil {
		return fmt.Errorf("failed to list routes for VNF subnet %s: %w", sid.String(), err)
	}
	if len(routes) == 0 {
		logger.DebugLogger().Printf("No existing route for VNF subnet %s, creating a new one", sid.String())
		// No existing route, create a new one
		route = &netlink.Route{
			Dst:       sid,
			Table:     int(d.routerVrf.Table),
			MultiPath: nextHops,
		}
		logger.DebugLogger().Printf("Creating new route for VNF subnet %s: %v", sid.String(), route)
	} else {
		logger.DebugLogger().Printf("Found existing route(s) for VNF subnet %s: %v", sid.String(), routes)
		logger.DebugLogger().Printf("Updating existing route for VNF subnet %s: %v", sid.String(), routes[0])
		// Update existing route
		route = &routes[0]
		route.MultiPath = nextHops
		logger.DebugLogger().Printf("Updated route for VNF subnet %s: %v", sid.String(), route)
	}
	if err := netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("failed to update route for VNF subnet %s: %w", sid.String(), err)
	}
	return nil
}

func (d *Software) setUpRouterSubnet() error {
	routerVrfTable, err := d.tableAllocator.Allocate()
	if err != nil {
		return fmt.Errorf("failed to allocate router VRF table ID: %w", err)
	}
	routerVrf := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{
			Name: "router-vrf",
		},
		Table: uint32(routerVrfTable),
	}
	if err := netlink.LinkAdd(routerVrf); err != nil {
		return fmt.Errorf("failed to add router VRF: %w", err)
	}
	if err := netlink.LinkSetUp(routerVrf); err != nil {
		return fmt.Errorf("failed to set up router VRF: %w", err)
	}

	// Enable VRF strict mode. Required because of SRv6 and VRF interaction.
	if err := os.WriteFile("/proc/sys/net/vrf/strict_mode", []byte("1"), 0644); err != nil {
		logger.ErrorLogger().Printf("Failed to enable VRF strict mode: %s", err)
	}

	decapIface := &netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{
			Name: "decap0",
		},
	}
	if err := netlink.LinkAdd(decapIface); err != nil {
		return fmt.Errorf("failed to add decap interface: %w", err)
	}
	if err := netlink.LinkSetMaster(decapIface, routerVrf); err != nil {
		return fmt.Errorf("failed to set master for decap interface: %w", err)
	}
	if err := netlink.LinkSetUp(decapIface); err != nil {
		return fmt.Errorf("failed to set up decap interface: %w", err)
	}

	decapSid, err := d.sidNetAllocator.Allocate()
	if err != nil {
		return fmt.Errorf("failed to allocate decap SID: %w", err)
	}
	// ip -6 route add <decap sid> table <router vrf table> encap seg6local action End.DT6 table <router vrf table> dev <decap iface>
	var flags_end_dt6_encaps [nl.SEG6_LOCAL_MAX]bool
	flags_end_dt6_encaps[nl.SEG6_LOCAL_ACTION] = true
	flags_end_dt6_encaps[nl.SEG6_LOCAL_TABLE] = true
	decapRoute := &netlink.Route{
		Dst:   decapSid,
		Table: int(routerVrf.Table),
		Encap: &netlink.SEG6LocalEncap{
			Flags:  flags_end_dt6_encaps,
			Action: nl.SEG6_LOCAL_ACTION_END_DT6,
			Table:  int(routerVrf.Table),
		},
		LinkIndex: decapIface.Attrs().Index,
	}
	if err := netlink.RouteAdd(decapRoute); err != nil {
		logger.ErrorLogger().Printf("Failed to execute `ip -6 route add %s table %d encap seg6local action End.DT6 vrftable %d dev %s`: %s", decapSid.String(), routerVrf.Table, routerVrf.Table, decapIface.Attrs().Name, err)
		return fmt.Errorf("failed to add decap route: %w", err)
	}
	d.routerVrf = routerVrf

	globalConfig, err := env.Config()
	if err != nil {
		return fmt.Errorf("failed to get global config: %w", err)
	}
	globalConfig.DecapSID = decapSid
	return nil
}

func (d *Software) tearDownRouterSubnet() {
	routerVrf, err := netlink.LinkByName("router-vrf")
	if err == nil {
		netlink.LinkDel(routerVrf)
	}
	decapIface, err := netlink.LinkByName("decap0")
	if err == nil {
		netlink.LinkDel(decapIface)
	}
}
