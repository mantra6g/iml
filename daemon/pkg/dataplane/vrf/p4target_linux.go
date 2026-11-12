package vrf

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	"github.com/mantra6g/iml/daemon/pkg/dataplane"
	vrfutil "github.com/mantra6g/iml/daemon/pkg/dataplane/vrf/util"
	"github.com/mantra6g/iml/daemon/pkg/ebpf"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultP4TargetMTU is the MTU of the interfaces the CNI plugin creates for containerized P4Targets.
	DefaultP4TargetMTU = 9000

	// maxIfaceNameLen is the longest interface name the kernel accepts (IFNAMSIZ - 1).
	maxIfaceNameLen = 15
)

// p4TargetLinks names the host-side interfaces a P4Target is wired through:
//
//	routing bridge ── BridgeVeth ══ Dispatcher ──┬──► Ports[0]
//	                                             └──► ...
//
// Names are derived from the target's name so they survive a daemon restart.
type p4TargetLinks struct {
	BridgeVeth string
	Dispatcher string
	Ports      []string
}

// newP4TargetLinks returns the host-side interfaces of target, as configured by
// spec.network.hostInterfaces, behind a dispatcher:
//   - Create: Count nfX interfaces, created by the CNI plugin. This is the default, with a Count of 1.
//   - Reuse: the named interfaces, exposed by the device itself (e.g. a BlueField-3).
func newP4TargetLinks(target *corev1alpha1.P4Target) (*p4TargetLinks, error) {
	links := defaultP4TargetLinks(target.Name)
	if dataplane.ReusesHostInterfaces(target) {
		reuse := target.Spec.Network.HostInterfaces.Reuse
		if reuse == nil || len(reuse.Names) == 0 {
			return nil, fmt.Errorf("hostInterfaces.reuse.names must not be empty")
		}
		links.Ports = append([]string(nil), reuse.Names...)
		return links, nil
	} 
	if dataplane.ReusesMappedHostInterfaces(target) {
		reuse := target.Spec.Network.HostInterfaces.ReuseMapped
		if reuse == nil || len(reuse.IfacePairs) == 0 {
			return nil, fmt.Errorf("hostInterfaces.reuse.names must not be empty")
		}
		// interface pairs are flattened in the Ports slice [In0, Out0, In1, Out1, ..., InN, OutN]
		ports := []string{}
		for _, pair := range reuse.IfacePairs {
			ports = append(ports, pair.In)
			ports = append(ports, pair.Out)
		}

		links.Ports = ports
		return links, nil
	}
	// The API defaults hostInterfaces to Create with a Count of 1, so a missing field means the same.
	count := 1
	if target.Spec.Network != nil && target.Spec.Network.HostInterfaces != nil &&
		target.Spec.Network.HostInterfaces.Create != nil && target.Spec.Network.HostInterfaces.Create.Count > 0 {
		count = int(target.Spec.Network.HostInterfaces.Create.Count)
	}
	prefix := p4TargetLinkPrefix(target.Name)
	for i := range count {
		name := fmt.Sprintf("%sp%d", prefix, i)
		if len(name) > maxIfaceNameLen {
			return nil, fmt.Errorf("too many host interfaces (%d)", count)
		}
		links.Ports = append(links.Ports, name)
	}
	return links, nil
}

// defaultP4TargetLinks returns the links that can be derived from the target's name alone. It is
// what teardown falls back to when the daemon restarted and the target's spec is gone.
func defaultP4TargetLinks(targetName string) *p4TargetLinks {
	prefix := p4TargetLinkPrefix(targetName)
	return &p4TargetLinks{
		BridgeVeth: prefix + "b",
		Dispatcher: prefix + "d",
	}
}

// p4TargetLinkPrefix returns "tgt" followed by 8 hex chars of the target name's hash.
func p4TargetLinkPrefix(targetName string) string {
	sum := sha256.Sum256([]byte(targetName))
	return "tgt" + hex.EncodeToString(sum[:4])
}

func usesSRv6Proxy(target *corev1alpha1.P4Target) bool {
	return target.Spec.Network != nil &&
		target.Spec.Network.SRv6 != nil &&
		target.Spec.Network.SRv6.Mode == corev1alpha1.SRv6ModeProxy
}

func (d *Software) EnsureP4TargetHostWiring(target *corev1alpha1.P4Target) error {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()

	key := client.ObjectKeyFromObject(target)
	instance, exists := d.p4Targets[key]
	if !exists {
		instance = &P4TargetInstance{}
	}
	if instance.TargetIPs.IsEmpty() {
		// The daemon restarted since the IP was assigned. It's recorded in the spec.
		addrs, err := vrfutil.ParseDualStackGatewayFromStrings(target.Spec.TargetDataplaneIPs)
		if err != nil || addrs.IPv6 == nil {
			return fmt.Errorf("P4Target %s has no dataplane IPv6 address yet", target.Name)
		}
		instance.TargetIPs.IPv6Net = &net.IPNet{IP: addrs.IPv6, Mask: net.CIDRMask(128, 128)}
	}
	if instance.links == nil {
		links, err := newP4TargetLinks(target)
		if err != nil {
			return fmt.Errorf("failed to name host interfaces for P4Target %s: %w", target.Name, err)
		}
		instance.links = links
	}
	d.p4Targets[key] = instance

	proxyConfig := &target.Spec.Network.SRv6.ProxyConfig

	return d.wireP4Target(target.Name, instance.links, instance.TargetIPs.IPv6Net.IP, usesSRv6Proxy(target), dataplane.ReusesMappedHostInterfaces(target), proxyConfig)
}

// wireP4Target connects the target's links to the routing bridge:
//   - a bridge veth ↔ dispatcher pair, with srv6_dispatch spreading the traffic for targetIP across
//     the ports, and a static neighbor entry resolving targetIP to the dispatcher, since nothing
//     answers neighbor solicitations for it.
//   - with srv6Proxy, srv6_end applies the SRv6 End behavior to the traffic coming back from the target.
func (d *Software) wireP4Target(targetName string, links *p4TargetLinks, targetIP net.IP, srv6Proxy bool, mappedInterfaces bool, proxyConfig *corev1alpha1.ProxyConfig) error {
	bridge, err := netlink.LinkByName(d.routingSubnet.Bridge.Name)
	if err != nil {
		return fmt.Errorf("failed to get routing bridge: %w", err)
	}

	ports := make([]netlink.Link, 0, len(links.Ports))
	for _, name := range links.Ports {
		port, err := linkByName(name)
		if err != nil {
			return err
		}
		if err = netlink.LinkSetUp(port); err != nil {
			return fmt.Errorf("failed to set %s up: %w", name, err)
		}
		ports = append(ports, port)
	}
	dispatcher, err := ensureDispatcherPair(links, minMTU(ports))
	if err != nil {
		return err
	}

	err = netlink.NeighSet(&netlink.Neigh{
		LinkIndex:    bridge.Attrs().Index,
		HardwareAddr: dispatcher.Attrs().HardwareAddr,
		IP:           targetIP,
		State:        netlink.NUD_PERMANENT,
	})
	if err != nil {
		return fmt.Errorf("failed to set static neighbor entry for %s: %w", targetIP, err)
	}

	bridgeVeth, err := linkByName(links.BridgeVeth)
	if err != nil {
		return err
	}
	if bridgeVeth.Attrs().MasterIndex != bridge.Attrs().Index {
		if err = netlink.LinkSetMaster(bridgeVeth, bridge); err != nil {
			return fmt.Errorf("failed to attach %s to the routing bridge: %w", links.BridgeVeth, err)
		}
	}
	if err = netlink.LinkSetUp(bridgeVeth); err != nil {
		return fmt.Errorf("failed to set %s up: %w", links.BridgeVeth, err)
	}

	if srv6Proxy {
		switch proxyConfig.Strategy {
		case corev1alpha1.DynamicProxy:
			return configureDynamicProxy(targetName, links, mappedInterfaces, dispatcher, bridgeVeth)
		case corev1alpha1.LaneBasedProxy:
			return configureLaneBasedProxy(targetName, links, mappedInterfaces, dispatcher, bridgeVeth, proxyConfig)
		case corev1alpha1.FieldBasedProxy:
			return configureFieldBasedProxy(targetName, links, dispatcher, bridgeVeth, proxyConfig)
		default:
			return fmt.Errorf("Unsupported ProxyStrategy: %s", string(proxyConfig.Strategy))
		}
	} else {
		if err = ebpf.DetachTC(links.BridgeVeth, ebpf.Ingress); err != nil {
			return fmt.Errorf("failed to detach SRv6 End from %s: %w", links.BridgeVeth, err)
		}
		return nil
	}
}

func configureDynamicProxy(targetName string, links *p4TargetLinks, mappedInterfaces bool, dispatcher netlink.Link, bridge_port netlink.Link) error {
	if len(links.Ports) < 2 {
		return fmt.Errorf("Dynamic proxying requires at least two interfaces.")
	}
	if !mappedInterfaces {
		fmt.Printf("Enough interfaces available for dynamic proxying, but no mapping specified. Selecting %s as In (toVNF) and %s as Out (fromVNF).\n",
			links.Ports[0], links.Ports[1])
	}
	objs, err := ebpf.LoadSRv6Dynamic(ebpf.TargetPinPaths(targetName), nil); if err != nil {
		return fmt.Errorf("Loading SRv6DynamicProxy: %w", err)
	}
	
	initSrhValue := make([]byte, 2104) // the size of cached_outer 
	if err := objs.SrhTable.Put(uint32(0), initSrhValue); err != nil {
		return fmt.Errorf("initializing SRH table: %w", err)
	}

	// maintaining ordering [In, Out, Dispatcher, BridgeVeth] is crucial here
	ifaces := []netlink.Link{}
	for _, portName := range links.Ports {
		l, err := netlink.LinkByName(portName); if err != nil {
			return fmt.Errorf("Couldnt get link: %s", portName)
		}
		ifaces = append(ifaces, l)
	}
	ifaces = append(ifaces, dispatcher)
	ifaces = append(ifaces, bridge_port)
	
	for i, iface := range ifaces {
        // inIfaces..., OutIfaces..., DispatchVeth, RoutingBridge
        if err := objs.LinkIndexTable.Put(uint32(i), uint32(iface.Attrs().Index)); err != nil {
            return fmt.Errorf("populate lane LinkIndex: %w", err)
        }
		mac := vrfutil.PaddedMAC(iface.Attrs().HardwareAddr)
        if err := objs.MacAddrTable.Put(uint32(i), mac); err != nil {
            return fmt.Errorf("populate lane MAC: %w", err)
        }
    }

	defer objs.Close()
	if err = ebpf.AttachXDP(objs.Decap, links.Dispatcher); err != nil {
		return fmt.Errorf("failed to attach dynamic decap to %s XDP: %w", links.Dispatcher, err)
	}
	if err = ebpf.AttachXDP(objs.Encap, links.Ports[1]); err != nil {
		return fmt.Errorf("failed to attach dynamic encap to %s XDP: %w", links.Ports[1], err)
	}
	return nil
}

func configureLaneBasedProxy(targetName string, links *p4TargetLinks, mappedInterfaces bool, dispatcher netlink.Link, bridge_port netlink.Link, proxyConfig *corev1alpha1.ProxyConfig) error {
	if !mappedInterfaces || len(links.Ports) < 2 {
		return fmt.Errorf("Lane-based Proxying requires at least two mapped interfaces. Got: ifaces mapped? %b, Interfaces: %d",
			mappedInterfaces, len(links.Ports))
	}
	if(len(links.Ports) % 2 != 0){
		return fmt.Errorf("Interface peer missing, proxying requires a multiple of two interfaces.")
	}
	if(proxyConfig.LaneCount > len(links.Ports)/2){
		return fmt.Errorf("Target ports do not suffice to satisfy the LaneCount (one Lane requires two interfaces) requested. LaneCount: %d, Ports:%d.",
			proxyConfig.LaneCount, len(links.Ports))
	}
	n := proxyConfig.LaneCount
	// LoadSrv6LaneBased
	objs, err := ebpf.LoadSRv6LaneBased(ebpf.TargetPinPaths(targetName), nil, n); if err != nil {
		return fmt.Errorf("reading SRv6 lane-based proxy to spec: %w", err)
	}

	initSrhValue := make([]byte, 2104) // the size of cached_outer 
	for i := range n {
		if err := objs.SrhTable.Put(uint32(i), initSrhValue); err != nil {
			return fmt.Errorf("initializing SRH table: %w", err)
		}
	}
	// maintaining ordering [In, Out, Dispatcher, BridgeVeth] is crucial here
	ifaces := []netlink.Link{}
	for _, portName := range links.Ports {
		l, err := netlink.LinkByName(portName); if err != nil {
			return fmt.Errorf("Couldnt get link: %s", portName)
		}
		ifaces = append(ifaces, l)
	}
	ifaces = append(ifaces, dispatcher)
	ifaces = append(ifaces, bridge_port)
	
	for i, iface := range ifaces {
        // inIfaces..., OutIfaces..., DispatchVeth, RoutingBridge
        if err := objs.LinkIndexTable.Put(uint32(i), uint32(iface.Attrs().Index)); err != nil {
            return fmt.Errorf("populate lane LinkIndex: %w", err)
        }
		mac := vrfutil.PaddedMAC(iface.Attrs().HardwareAddr)
        if err := objs.MacAddrTable.Put(uint32(i), mac); err != nil {
            return fmt.Errorf("populate lane MAC: %w", err)
        }
		if(i >= n && i < 2*n){ // indices n -> 2n-1 are outgoing interfaces
            err := objs.LaneTable.Put(uint32(iface.Attrs().Index), uint32(i - n)); if err != nil {
                return fmt.Errorf("populating lane map (link_index of return iface -> lane): %w", err)
            }
        }
    }
	    
    zeroValue := make([]byte, 2072) // sizeof(sfc_t)
    for i := range n {
        if err := objs.SfcLaneMap.Put(uint32(i), zeroValue); err != nil {
            return fmt.Errorf("populating sfc lane map: %w", err)
        }
    }
	
	decapProg := objs.Decap
    err = objs.Progs.Put(uint32(0), decapProg); if err != nil {
        return fmt.Errorf("populating tailcall map: %w", err)
    }

	defer objs.Close()
	if err = ebpf.AttachXDP(objs.ClassifySfc, links.Dispatcher); err != nil {
		return fmt.Errorf("failed to attach dynamic decap to %s XDP: %w", links.Dispatcher, err)
	}
	// links.Ports = [in1, in2, ..., out1, out2..., outN] -> attach encap to all return ifaces
	for i := n; i < 2*n; i++ {
		if err = ebpf.AttachXDP(objs.Encap, links.Ports[i]); err != nil {
			return fmt.Errorf("failed to attach dynamic encap to %s XDP: %w", links.Ports[1], err)
		}
	}
	return nil
}

func configureFieldBasedProxy(targetName string, links *p4TargetLinks, dispatcher netlink.Link, bridge_port netlink.Link, proxyConfig *corev1alpha1.ProxyConfig) error {
	if(len(links.Ports) < 2){
		return fmt.Errorf("Field-based proxying requires at least two interfaces, got: %d", len(links.Ports))
	}

	objs, err := ebpf.LoadSRv6FieldBased(ebpf.TargetPinPaths(targetName), nil); if err != nil {
		return fmt.Errorf("loading srv6 field-based proxy: %w", err)
	}

	/*
	* init maps: scratch map, mac/link_index tables, key masks
	*/
	// scratch table
	zeroValue := make([]byte, 2104)
    if err := objs.ScratchTable.Put(uint32(0), zeroValue); err != nil {
       return fmt.Errorf("initialize SRH scratch table entry: %w", err)
    }

	// maintaining ordering [In, Out, Dispatcher, BridgeVeth] is crucial here
	ifaces := []netlink.Link{}
	for _, portName := range links.Ports {
		l, err := netlink.LinkByName(portName); if err != nil {
			return fmt.Errorf("Couldnt get link: %s", portName)
		}
		ifaces = append(ifaces, l)
	}
	ifaces = append(ifaces, dispatcher)
	ifaces = append(ifaces, bridge_port)
	
	// mac_addr_table + link_index_table
	for i, iface := range ifaces {
        // inIface, OutIface, DispatchVeth, RoutingBridge
        if err := objs.LinkIndexTable.Put(uint32(i), uint32(iface.Attrs().Index)); err != nil {
            return fmt.Errorf("populate lane LinkIndex: %w", err)
        }
		mac := vrfutil.PaddedMAC(iface.Attrs().HardwareAddr)
        if err := objs.MacAddrTable.Put(uint32(i), mac); err != nil {
            return fmt.Errorf("populate lane MAC: %w", err)
        }
    }
	// key_mask_tables: get immutable fields from proxyConfig and calculate masks
	// IPv4
	ipv4_fields := vrfutil.FilterFieldsByProto(proxyConfig.Fields, vrfutil.PROTO_IPv4)
	mask, err := vrfutil.GetKeyMaskForProto(ipv4_fields); if err != nil {
		fmt.Printf("Could not get key mask for IPv4: %s", err)
	}
	err = objs.MaskTableL3.Put(uint32(vrfutil.PROTO_NUM_IPv4), 
		ebpf.Srv6FieldBasedL3MaskT{ Bytes: [vrfutil.MAX_L3_HDR_LEN]byte(mask) }); 
	if err != nil {
		return fmt.Errorf("Could not insert key mask for proto IPv4.\n: %w", err)
	}
	// IPv6
	ipv6_fields := vrfutil.FilterFieldsByProto(proxyConfig.Fields, vrfutil.PROTO_IPv6)
	mask, err = vrfutil.GetKeyMaskForProto(ipv6_fields); if err != nil {
		fmt.Printf("Could not get key mask for IPv6: %s", err)
	}
	err = objs.MaskTableL3.Put(uint32(vrfutil.PROTO_NUM_IPv6), 
		ebpf.Srv6FieldBasedL3MaskT{ Bytes: [vrfutil.MAX_L3_HDR_LEN]byte(mask) }); 
	if err != nil {
		return fmt.Errorf("Could not insert key mask for proto IPv6.\n: %w", err)
	}
	// TCP
	tcp_fields := vrfutil.FilterFieldsByProto(proxyConfig.Fields, vrfutil.PROTO_TCP)
	mask, err = vrfutil.GetKeyMaskForProto(tcp_fields); if err != nil {
		fmt.Printf("Could not get key mask for TCP: %s", err)
	}
	err = objs.MaskTableL4.Put(uint32(vrfutil.PROTO_NUM_TCP), 
		ebpf.Srv6FieldBasedL4MaskT{ Bytes: [vrfutil.MAX_L4_HDR_LEN]byte(mask) }); 
	if err != nil {
		return fmt.Errorf("Could not insert key mask for proto TCP.\n: %w", err)
	}
	// UDP
	udp_fields := vrfutil.FilterFieldsByProto(proxyConfig.Fields, vrfutil.PROTO_UDP)
	mask, err = vrfutil.GetKeyMaskForProto(udp_fields); if err != nil {
		fmt.Printf("Could not get key mask for UDP: %s", err)
	}
	err = objs.MaskTableL4.Put(uint32(vrfutil.PROTO_NUM_UDP), 
		ebpf.Srv6FieldBasedL4MaskT{ Bytes: [vrfutil.MAX_L4_HDR_LEN]byte(mask) }); 
	if err != nil {
		return fmt.Errorf("Could not insert key mask for proto UDP.\n: %w", err)
	}

	/*
	* ATTACH programs to dispatcher and Out
	*/
	defer objs.Close()
	if err = ebpf.AttachXDP(objs.ProxyIngress, links.Dispatcher); err != nil {
		return fmt.Errorf("failed to attach field-based decap to %s XDP: %w", links.Dispatcher, err)
	}
	if err = ebpf.AttachXDP(objs.ProxyEgress, links.Ports[1]); err != nil {
		return fmt.Errorf("failed to attach field-based decap to %s XDP: %w", links.Ports[1], err)
	}
	return nil
}

// ensureDispatcherPair creates the bridge veth ↔ dispatcher pair if it doesn't exist yet, and
// returns the dispatcher with both ends up and set to mtu.
func ensureDispatcherPair(links *p4TargetLinks, mtu int) (netlink.Link, error) {
	dispatcher, err := netlink.LinkByName(links.Dispatcher)
	if errors.As(err, &netlink.LinkNotFoundError{}) {
		veth := &netlink.Veth{
			LinkAttrs: netlink.LinkAttrs{Name: links.Dispatcher, MTU: mtu},
			PeerName:  links.BridgeVeth,
			PeerMTU:   uint32(mtu),
		}
		if err = netlink.LinkAdd(veth); err != nil {
			return nil, fmt.Errorf("failed to create veth pair %s/%s: %w", links.Dispatcher, links.BridgeVeth, err)
		}
		dispatcher, err = netlink.LinkByName(links.Dispatcher)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get dispatcher %s: %w", links.Dispatcher, err)
	}
	for _, name := range []string{links.Dispatcher, links.BridgeVeth} {
		link, err := netlink.LinkByName(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get %s: %w", name, err)
		}
		if link.Attrs().MTU != mtu {
			if err = netlink.LinkSetMTU(link, mtu); err != nil {
				return nil, fmt.Errorf("failed to set MTU of %s to %d: %w", name, mtu, err)
			}
		}
		if err = netlink.LinkSetUp(link); err != nil {
			return nil, fmt.Errorf("failed to set %s up: %w", name, err)
		}
	}
	return dispatcher, nil
}

// attachDispatcher loads the target's srv6_dispatch instance, points it at the dispatcher and the
// ports, and attaches it to all of them.
func attachDispatcher(targetName string, dispatcher netlink.Link, ports []netlink.Link, mappedIfaces bool) error {
	objs, err := ebpf.LoadSRv6Dispatch(ebpf.TargetPinPaths(targetName), nil)
	if err != nil {
		return fmt.Errorf("failed to load SRv6 Dispatch eBPF program: %w", err)
	}
	defer objs.Close()

	vmac, err := macArray(dispatcher)
	if err != nil {
		return err
	}
	err = objs.Cfg.Put(uint32(0), ebpf.Srv6DispatchConfig{
		DispatchIfindex: uint32(dispatcher.Attrs().Index),
		Vmac:            vmac,
	})
	if err != nil {
		return fmt.Errorf("failed to configure dispatcher %s: %w", dispatcher.Attrs().Name, err)
	}
	instances := make([]ebpf.Srv6DispatchInstance, 0, len(ports))

	if mappedIfaces {
		// mappedIfaces indicates that the ports belong together in pairs:
		// assemble bucket entries to match the correct destination address to the correct out iface
		for i := 0; i < len(ports) - 1; i =+ 2 {
			var mac [6]byte
			toVNF := ports[i]
			fromVNF := ports[i+1]
			mac, err = macArray(fromVNF)
			if err != nil {
				return err
			}
			instances = append(instances, ebpf.Srv6DispatchInstance{
				Ifindex: uint32(toVNF.Attrs().Index),
				Mac:     mac,
			})
		}
	} else {
		// use a load-balancing strategy to distribute traffic between multiple interfaces instaed of a fixed mapping
		for _, port := range ports {
			mac, err := macArray(port)
			if err != nil {
				return err
			}
			instances = append(instances, ebpf.Srv6DispatchInstance{
				Ifindex: uint32(port.Attrs().Index),
				Mac:     mac,
			})
		}
	}
	if err = ebpf.FillBuckets(objs.Buckets, instances); err != nil {
		return fmt.Errorf("failed to fill dispatch buckets: %w", err)
	}

	for _, link := range append([]netlink.Link{dispatcher}, ports...) {
		if err = ebpf.AttachTC(objs.Srv6Dispatch, link.Attrs().Name, ebpf.Ingress); err != nil {
			return fmt.Errorf("failed to attach SRv6 Dispatch to %s ingress: %w", link.Attrs().Name, err)
		}
	}
	return nil
}

func (d *Software) TeardownP4TargetHostWiring(target client.ObjectKey) error {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()
	return d.unwireP4Target(target)
}

func (d *Software) DeleteP4TargetInstance(target client.ObjectKey) error {
	d.p4Mu.Lock()
	defer d.p4Mu.Unlock()
	if err := d.unwireP4Target(target); err != nil {
		return err
	}
	delete(d.p4Targets, target)
	return nil
}

// unwireP4Target undoes wireP4Target. Ports are left in place: they are either the device's own
// interfaces or go away with the target's network namespace. Without a cached instance (the daemon
// restarted) only what can be derived from the target's name is removed.
func (d *Software) unwireP4Target(target client.ObjectKey) error {
	links := defaultP4TargetLinks(target.Name)
	var targetIP net.IP
	if instance, exists := d.p4Targets[target]; exists {
		if instance.links != nil {
			links = instance.links
		}
		if instance.TargetIPs.IPv6Net != nil {
			targetIP = instance.TargetIPs.IPv6Net.IP
		}
	}
	for _, port := range links.Ports {
		if err := ebpf.DetachXDP(port); err != nil {
			return fmt.Errorf("failed to detach SRv6 Dispatch from %s: %w", port, err)
		}
	}
	if targetIP != nil {
		bridge, err := netlink.LinkByName(d.routingSubnet.Bridge.Name)
		if err != nil {
			return fmt.Errorf("failed to get routing bridge: %w", err)
		}
		err = netlink.NeighDel(&netlink.Neigh{LinkIndex: bridge.Attrs().Index, IP: targetIP})
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("failed to delete static neighbor entry for %s: %w", targetIP, err)
		}
	}
	// Deleting the dispatcher also deletes its bridge veth peer.
	dispatcher, err := netlink.LinkByName(links.Dispatcher)
	if err == nil {
		err = netlink.LinkDel(dispatcher)
	}
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) {
		return fmt.Errorf("failed to delete dispatcher %s: %w", links.Dispatcher, err)
	}
	return ebpf.UnpinTarget(target.Name)
}

// linkByName is netlink.LinkByName, reporting a missing link as dataplane.ErrP4TargetLinksMissing.
func linkByName(name string) (netlink.Link, error) {
	link, err := netlink.LinkByName(name)
	if errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil, fmt.Errorf("%w: %s", dataplane.ErrP4TargetLinksMissing, name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", name, err)
	}
	return link, nil
}

func minMTU(links []netlink.Link) int {
	mtu := DefaultP4TargetMTU
	for _, link := range links {
		mtu = min(mtu, link.Attrs().MTU)
	}
	return mtu
}

func macArray(link netlink.Link) ([6]byte, error) {
	var mac [6]byte
	if len(link.Attrs().HardwareAddr) != len(mac) {
		return mac, fmt.Errorf("%s has no Ethernet address", link.Attrs().Name)
	}
	copy(mac[:], link.Attrs().HardwareAddr)
	return mac, nil
}
