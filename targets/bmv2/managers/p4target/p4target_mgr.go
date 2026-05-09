package p4target

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"bmv2-driver/pkg/ipam"
)

type Condition struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

type NetConfig struct {
	TargetCIDR string
}

type ManagerConfig struct {
	Name       string
	TargetIPs  []net.IP
	DriverIPs  []net.IP
	MaxNFSlots int
	P4Client   p4v1.P4RuntimeClient
	BridgeName string
	Log        logr.Logger
}

type Manager interface {
	GetName() string
	GetCapacity() corev1.ResourceList
	GetAllocatable() corev1.ResourceList
	GetHealthyCondition() corev1alpha1.P4TargetCondition
	GetReadyCondition(corev1alpha1.P4TargetCondition, corev1alpha1.P4TargetCondition) corev1alpha1.P4TargetCondition
	GetNetworkConfiguredCondition() corev1alpha1.P4TargetCondition
	GetOccupiedCondition() corev1alpha1.P4TargetCondition
	GetTargetIPs() []net.IP
	GetDriverIPs() []net.IP
	EnsureNetworkConfiguration(NetConfig) error
	AllocateNetworkFunctionIP() (netip.Addr, error)
	GetNfCIDR() netip.Prefix
}

func NewManager(cfg ManagerConfig) (Manager, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("manager name is required")
	}
	if cfg.MaxNFSlots <= 0 {
		return nil, fmt.Errorf("max NF slots must be > 0")
	}
	if cfg.P4Client == nil {
		return nil, fmt.Errorf("P4Runtime client is required")
	}
	return &RealManager{
		name:         cfg.Name,
		targetIPs:    cfg.TargetIPs,
		driverIPs:    cfg.DriverIPs,
		maxNFSlots:   cfg.MaxNFSlots,
		p4client:     cfg.P4Client,
		BridgeName:   cfg.BridgeName,
		allocatedIPs: make(map[netip.Addr]struct{}),
		log:          cfg.Log,
	}, nil
}

// Compile-time assertion to ensure RealManager implements the Manager interface
var _ Manager = &RealManager{}

type RealManager struct {
	name       string
	targetIPs  []net.IP
	driverIPs  []net.IP
	maxNFSlots int
	p4client   p4v1.P4RuntimeClient
	BridgeName string
	log        logr.Logger

	mu           sync.RWMutex
	cidr         netip.Prefix
	ipAllocator  *ipam.AddrAllocator
	allocatedIPs map[netip.Addr]struct{}
}

func (r *RealManager) GetNfCIDR() netip.Prefix {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cidr
}

func (r *RealManager) GetName() string { return r.name }

func (r *RealManager) GetTargetIPs() []net.IP { return r.targetIPs }

func (r *RealManager) GetDriverIPs() []net.IP { return r.driverIPs }

func (r *RealManager) EnsureNetworkConfiguration(cfg NetConfig) error {
	cidr, err := netip.ParsePrefix(cfg.TargetCIDR)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cfg.TargetCIDR, err)
	}
	cidr = cidr.Masked()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cidr == cidr {
		return nil
	}

	alloc, err := ipam.NewAddrAllocator(cidr)
	if err != nil {
		return fmt.Errorf("failed to create IP allocator: %w", err)
	}
	r.cidr = cidr
	r.ipAllocator = alloc
	r.allocatedIPs = make(map[netip.Addr]struct{})
	if r.BridgeName != "" {
		bridgeIP, err := alloc.Next()
		if err != nil {
			return fmt.Errorf("failed to allocate bridge IP from %s: %w", cidr, err)
		}
		if err := r.configureNFBridge(bridgeIP, cidr); err != nil {
			return fmt.Errorf("failed to configure NF bridge: %w", err)
		}
	}
	return nil
}

func (r *RealManager) AllocateNetworkFunctionIP() (netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ipAllocator == nil {
		return netip.Addr{}, fmt.Errorf("network not configured: call EnsureNetworkConfiguration first")
	}
	addr, err := r.ipAllocator.Next()
	if err != nil {
		return netip.Addr{}, err
	}
	r.allocatedIPs[addr] = struct{}{}
	return addr, nil
}

func (r *RealManager) GetCapacity() corev1.ResourceList {
	capacity := corev1.ResourceList{
		ResourceNFSlots: *resource.NewQuantity(int64(r.maxNFSlots), resource.DecimalSI),
	}
	if tableCapacity := r.queryTableCapacity(); tableCapacity > 0 {
		capacity[ResourceTableEntries] = *resource.NewQuantity(tableCapacity, resource.DecimalSI)
	}
	return capacity
}

func (r *RealManager) GetAllocatable() corev1.ResourceList {
	r.mu.RLock()
	usedSlots := int64(len(r.allocatedIPs))
	r.mu.RUnlock()

	freeSlots := max(int64(r.maxNFSlots)-usedSlots, 0)
	allocatable := corev1.ResourceList{
		ResourceNFSlots: *resource.NewQuantity(freeSlots, resource.DecimalSI),
	}
	tableCapacity := r.queryTableCapacity()
	if tableCapacity > 0 {
		allocatable[ResourceTableEntries] = *resource.NewQuantity(max(tableCapacity-r.queryTableUsed(), 0), resource.DecimalSI)
	}
	return allocatable
}

func (r *RealManager) queryTableCapacity() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := r.p4client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
		ResponseType: p4v1.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	})
	if err != nil || resp.Config == nil || resp.Config.P4Info == nil {
		return 0
	}
	var total int64
	for _, t := range resp.Config.P4Info.Tables {
		total += t.Size
	}
	return total
}

func (r *RealManager) queryTableUsed() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := r.p4client.Read(ctx, &p4v1.ReadRequest{
		Entities: []*p4v1.Entity{{Entity: &p4v1.Entity_TableEntry{TableEntry: &p4v1.TableEntry{}}}},
	})
	if err != nil {
		return 0
	}
	var count int64
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		count += int64(len(resp.Entities))
	}
	return count
}

func (r *RealManager) GetHealthyCondition() corev1alpha1.P4TargetCondition {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := metav1.NewTime(time.Now())
	_, err := r.p4client.Capabilities(ctx, &p4v1.CapabilitiesRequest{})
	if err != nil {
		return corev1alpha1.P4TargetCondition{
			Type:              ConditionHealthy,
			Status:            metav1.ConditionFalse,
			LastHeartbeatTime: now,
			Reason:            "SwitchUnreachable",
			Message:           err.Error(),
		}
	}
	return corev1alpha1.P4TargetCondition{
		Type:              ConditionHealthy,
		Status:            metav1.ConditionTrue,
		LastHeartbeatTime: now,
		Reason:            "SwitchReachable",
		Message:           "The BMv2 switch is reachable",
	}
}

func (r *RealManager) GetReadyCondition(healthyCond corev1alpha1.P4TargetCondition,
	netconfCond corev1alpha1.P4TargetCondition) corev1alpha1.P4TargetCondition {
	if healthyCond.Status == metav1.ConditionTrue && netconfCond.Status == metav1.ConditionTrue {
		return corev1alpha1.P4TargetCondition{
			Type:              corev1alpha1.P4TargetConditionReady,
			Status:            metav1.ConditionTrue,
			LastHeartbeatTime: metav1.NewTime(time.Now()),
			Reason:            "TargetReady",
			Message:           "The target is ready.",
		}
	}
	return corev1alpha1.P4TargetCondition{
		Type:              corev1alpha1.P4TargetConditionReady,
		Status:            metav1.ConditionFalse,
		LastHeartbeatTime: metav1.NewTime(time.Now()),
		Reason:            "TargetNotReady",
		Message:           "The target is not ready.",
	}
}

func (r *RealManager) GetNetworkConfiguredCondition() corev1alpha1.P4TargetCondition {
	r.mu.RLock()
	cidr := r.cidr
	r.mu.RUnlock()
	now := metav1.NewTime(time.Now())
	if !cidr.IsValid() {
		return corev1alpha1.P4TargetCondition{
			Type:              ConditionNetworkConfigured,
			Status:            metav1.ConditionFalse,
			LastHeartbeatTime: now,
			Reason:            "NoCIDR",
			Message:           "No CIDR was assigned to the P4Target yet.",
		}
	}
	return corev1alpha1.P4TargetCondition{
		Type:              ConditionNetworkConfigured,
		Status:            metav1.ConditionTrue,
		LastHeartbeatTime: now,
		Reason:            "CIDRConfigured",
		Message:           cidr.String(),
	}
}

func (r *RealManager) GetOccupiedCondition() corev1alpha1.P4TargetCondition {
	r.mu.RLock()
	used := len(r.allocatedIPs)
	r.mu.RUnlock()
	now := metav1.NewTime(time.Now())
	if used >= r.maxNFSlots {
		return corev1alpha1.P4TargetCondition{
			Type:              ConditionOccupied,
			Status:            metav1.ConditionTrue,
			LastHeartbeatTime: now,
			Reason:            "NoSlotsAvailable",
			Message:           fmt.Sprintf("%d/%d slots used", used, r.maxNFSlots),
		}
	}
	return corev1alpha1.P4TargetCondition{
		Type:              ConditionOccupied,
		Status:            metav1.ConditionFalse,
		LastHeartbeatTime: now,
		Reason:            "SlotsAvailable",
		Message:           fmt.Sprintf("%d/%d slots used", used, r.maxNFSlots),
	}
}

// configureNFBridge configures the bridge interface with the given IP and subnet mask based
// on the allocated CIDR for the target. This is needed to ensure that the bridge has an IP
// in the same subnet as the NF interfaces, and also to ensure that it can act as a kind of
// gateway for the NFs to forward traffic back to the host.
func (r *RealManager) configureNFBridge(bridgeIP netip.Addr, nfCIDR netip.Prefix) error {
	nfBridge, err := netlink.LinkByName(r.BridgeName)
	if err != nil {
		return err
	}
	// List and remove old addresses, except for link-local and loopback addresses
	previousAddresses, err := netlink.AddrList(nfBridge, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list existing addresses on bridge: %w", err)
	}
	for _, addr := range previousAddresses {
		if addr.IP.IsLinkLocalUnicast() || addr.IP.IsLoopback() || addr.IP.IsLinkLocalMulticast() {
			continue
		}
		if err := netlink.AddrDel(nfBridge, &addr); err != nil {
			return fmt.Errorf("failed to remove old address %s from bridge: %w", addr.IP.String(), err)
		}
	}
	// Add the new address for the bridge based on the allocated CIDR
	newBridgeAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   bridgeIP.Unmap().AsSlice(),
			Mask: net.CIDRMask(nfCIDR.Bits(), bridgeIP.BitLen()),
		},
	}
	err = netlink.AddrAdd(nfBridge, newBridgeAddr)
	if err != nil {
		return fmt.Errorf("failed to add new address %s to bridge: %w", newBridgeAddr.IPNet.String(), err)
	}
	return nil
}
