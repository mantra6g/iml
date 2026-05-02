package p4target

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/vishvananda/netlink"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"golang.org/x/sys/unix"

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
	DriverIP   net.IP
	MaxNFSlots int
	P4Client   p4v1.P4RuntimeClient
	NFIface    string // kernel interface to assign NF IPs to (e.g. "nf0")
}

type Manager interface {
	GetName() string
	GetCapacity() corev1.ResourceList
	GetAllocatable() corev1.ResourceList
	GetHealthyCondition() corev1alpha1.P4TargetCondition
	GetReadyCondition() corev1alpha1.P4TargetCondition
	GetNetworkConfiguredCondition() corev1alpha1.P4TargetCondition
	GetOccupiedCondition() corev1alpha1.P4TargetCondition
	GetTargetIPs() []net.IP
	GetDriverIP() net.IP
	EnsureNetworkConfiguration(NetConfig) error
	AllocateNetworkFunctionIP() (net.IP, error)
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
		driverIP:     cfg.DriverIP,
		maxNFSlots:   cfg.MaxNFSlots,
		p4client:     cfg.P4Client,
		nfIface:      cfg.NFIface,
		allocatedIPs: make(map[netip.Addr]struct{}),
	}, nil
}

// Compile-time assertion to ensure RealManager implements the Manager interface
var _ Manager = &RealManager{}

type RealManager struct {
	name       string
	targetIPs  []net.IP
	driverIP   net.IP
	maxNFSlots int
	p4client   p4v1.P4RuntimeClient
	nfIface    string

	mu           sync.RWMutex
	cidr         netip.Prefix
	ipAllocator  *ipam.AddrAllocator
	allocatedIPs map[netip.Addr]struct{}
}

func (r *RealManager) GetName() string { return r.name }

func (r *RealManager) GetTargetIPs() []net.IP { return r.targetIPs }

func (r *RealManager) GetDriverIP() net.IP { return r.driverIP }

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
	return nil
}

func (r *RealManager) AllocateNetworkFunctionIP() (net.IP, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ipAllocator == nil {
		return nil, fmt.Errorf("network not configured: call EnsureNetworkConfiguration first")
	}
	addr, err := r.ipAllocator.Next()
	if err != nil {
		return nil, err
	}
	r.allocatedIPs[addr] = struct{}{}
	ip := net.IP(addr.Unmap().AsSlice())

	if r.nfIface != "" {
		if err := assignIPToIface(ip, r.nfIface); err != nil {
			return nil, fmt.Errorf("assigning IP %s to %s: %w", ip, r.nfIface, err)
		}
	}
	return ip, nil
}

func assignIPToIface(ip net.IP, ifaceName string) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("interface %q not found: %w", ifaceName, err)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	addr := &netlink.Addr{
		IPNet: &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)},
	}
	if err := netlink.AddrAdd(link, addr); err != nil && !errors.Is(err, unix.EEXIST) {
		return err
	}
	return nil
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
	_, err := r.p4client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{})
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
	}
}

func (r *RealManager) GetReadyCondition() corev1alpha1.P4TargetCondition {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := metav1.NewTime(time.Now())
	resp, err := r.p4client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
		ResponseType: p4v1.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	})
	if err != nil {
		return corev1alpha1.P4TargetCondition{
			Type:              corev1alpha1.P4TargetConditionReady,
			Status:            metav1.ConditionFalse,
			LastHeartbeatTime: now,
			Reason:            "SwitchUnreachable",
			Message:           err.Error(),
		}
	}
	if resp.Config == nil || resp.Config.P4Info == nil || len(resp.Config.P4Info.Tables) == 0 {
		return corev1alpha1.P4TargetCondition{
			Type:              corev1alpha1.P4TargetConditionReady,
			Status:            metav1.ConditionFalse,
			LastHeartbeatTime: now,
			Reason:            "NoProgramLoaded",
		}
	}
	return corev1alpha1.P4TargetCondition{
		Type:              corev1alpha1.P4TargetConditionReady,
		Status:            metav1.ConditionTrue,
		LastHeartbeatTime: now,
		Reason:            "ProgramLoaded",
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
