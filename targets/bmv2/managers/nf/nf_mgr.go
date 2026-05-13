package nf

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"bmv2-driver/api"
	p4targetpkg "bmv2-driver/managers/p4target"
	"bmv2-driver/pkg/p4compile"
	p4switch "bmv2-driver/pkg/p4switch"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/vishvananda/netlink"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerConfig struct {
	Switch          *p4switch.SwitchClient
	P4TargetManager p4targetpkg.Manager
	NFInterface     string
	Log             logr.Logger
	MaxNFSlots      int
}

type Manager interface {
	GetDeployedNetworkFunctions(ctx context.Context) ([]client.ObjectKey, error)
	EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction, nfIP netip.Prefix) DeploymentHandle
	EnsureAbsent(ctx context.Context, nfKey client.ObjectKey) DeploymentHandle
}

type operationType string

const (
	opPresent operationType = "Present"
	opAbsent  operationType = "Absent"
)

type trackedOp struct {
	handle *deploymentHandle
	opType operationType
}

type RealManager struct {
	switchClient    *p4switch.SwitchClient
	p4targetManager p4targetpkg.Manager
	nfInterface     string
	maxNFSlots      int
	log             logr.Logger

	mu       sync.Mutex
	programs map[client.ObjectKey]*api.P4Program
	ops      map[client.ObjectKey]*trackedOp
}

// Compile-time assertion to ensure RealManager implements the Manager interface
var _ Manager = &RealManager{}

func NewManager(cfg ManagerConfig) (Manager, error) {
	if cfg.Switch == nil {
		return nil, fmt.Errorf("switch client is required")
	}
	if cfg.P4TargetManager == nil {
		return nil, fmt.Errorf("P4Target manager is required")
	}
	return &RealManager{
		switchClient:    cfg.Switch,
		p4targetManager: cfg.P4TargetManager,
		nfInterface:     cfg.NFInterface,
		maxNFSlots:      cfg.MaxNFSlots,
		programs:        make(map[client.ObjectKey]*api.P4Program),
		ops:             make(map[client.ObjectKey]*trackedOp),
		log:             cfg.Log,
	}, nil
}

func (m *RealManager) EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction, nfIP netip.Prefix) DeploymentHandle {
	key := client.ObjectKeyFromObject(nf)

	if existing, ok := m.ops[key]; ok {
		if existing.opType == opPresent {
			return existing.handle // already deploying
		}
		// switching from delete → deploy
		existing.handle.Cancel()
	}

	ctx, cancel := context.WithCancel(ctx)
	h := newDeploymentHandle(cancel)

	m.ops[key] = &trackedOp{
		handle: h,
		opType: opPresent,
	}

	go m.runDeployment(ctx, h, nf, nfIP)

	return h
}

func (m *RealManager) EnsureAbsent(ctx context.Context, nfKey client.ObjectKey) DeploymentHandle {
	if existing, ok := m.ops[nfKey]; ok {
		if existing.opType == opAbsent {
			return existing.handle // already deleting
		}
		// switching from deploy → delete
		existing.handle.Cancel()
	}

	ctx, cancel := context.WithCancel(ctx)
	h := newDeploymentHandle(cancel)

	m.ops[nfKey] = &trackedOp{
		handle: h,
		opType: opAbsent,
	}

	go m.runDeletion(ctx, h, nfKey)

	return h
}

func (m *RealManager) GetDeployedNetworkFunctions(ctx context.Context) ([]client.ObjectKey, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	config, err := m.switchClient.GetPipeline(reqCtx)
	if err != nil {
		return nil, fmt.Errorf("querying switch pipeline: %w", err)
	}

	if config == nil || config.Config == nil || len(config.Config.P4DeviceConfig) == 0 {
		return nil, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	deployed := make([]client.ObjectKey, 0, len(m.ops))
	for objKey, op := range m.ops {
		if op.opType == opPresent && op.handle.status.Phase == PhaseReady {
			deployed = append(deployed, objKey)
		}
	}
	return deployed, nil
}

func (m *RealManager) runDeployment(ctx context.Context, h *deploymentHandle, nf *corev1alpha1.NetworkFunction, nfIP netip.Prefix) {
	m.mu.Lock()
	defer m.mu.Unlock()

	logger := m.log
	defer func() {
		if r := recover(); r != nil {
			h.transition(PhaseFailed, "panic occurred", fmt.Errorf("%v", r))
		}
	}()

	logger.V(1).Info("Compiling network function")
	h.transition(PhaseCompiling, "compiling network function", nil)
	if err := m.compile(ctx, nf); err != nil {
		logger.V(1).Error(err, "failed to compile network function")
		h.transition(PhaseFailed, "compilation failed", err)
		return
	}

	logger.V(1).Info("Running pre-deployment checks")
	h.transition(PhasePreCheck, "running pre-deployment checks", nil)
	if err := m.preCheck(ctx, nf); err != nil {
		logger.V(1).Error(err, "pre-check failed")
		h.transition(PhaseFailed, "pre-check failed", err)
		return
	}

	logger.V(1).Info("Deploying network function")
	h.transition(PhaseDeploying, "deploying network function", nil)
	if err := m.deploy(ctx, nf); err != nil {
		logger.V(1).Error(err, "Network function deploy failed")
		h.transition(PhaseFailed, "deployment failed", err)
		return
	}

	logger.V(1).Info("Setting up networking for NF")
	h.transition(PhaseNetworkSetup, "setting up network forwarding", nil)
	if err := m.setupNetworkForwarding(nf, nfIP); err != nil {
		logger.V(1).Error(err, "failed to setup network forwarding")
		h.transition(PhaseFailed, "network setup failed", err)
		return
	}
	logger.V(1).Info("Network function setup successfully")

	h.transition(PhaseReady, "network function ready", nil)
}

// compile fetches the P4 source (URL or base64-encoded BMv2 JSON) and produces
// a compiled P4Program stored in the programs map for use by deploy.
func (m *RealManager) compile(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	key := client.ObjectKeyFromObject(nf)
	p4File := nf.Spec.P4File

	var program *api.P4Program
	var err error

	if strings.HasPrefix(p4File, "http://") || strings.HasPrefix(p4File, "https://") || strings.HasPrefix(p4File, "s3://") {
		result, compileErr := p4compile.CompileFromURL(ctx, p4File)
		if compileErr != nil {
			err = compileErr
		} else {
			program = &api.P4Program{P4DeviceConfig: result.DeviceConfig, ProgramName: nf.Name, P4Info: result.P4Info}
		}
	} else {
		// Treat as base64-encoded pre-compiled BMv2 JSON device config.
		program, err = api.LoadP4ProgramFromBase64(p4File, nf.Name)
	}
	if err != nil {
		return err
	}

	m.programs[key] = program
	return nil
}

// preCheck verifies there are available NF slots and table entries before deploying.
func (m *RealManager) preCheck(ctx context.Context, _ *corev1alpha1.NetworkFunction) error {
	deployed := 0
	for _, op := range m.ops {
		if op.opType != opPresent {
			continue
		}
		if op.handle == nil {
			continue
		}
		if op.handle.status.Phase == PhaseReady {
			deployed++
		}
	}

	availableSlots := m.maxNFSlots - deployed
	if availableSlots <= 0 {
		return fmt.Errorf("no available NF slots on target (max: %d, allocated: %d)", m.maxNFSlots, deployed)
	}

	//slots := allocatable[p4targetpkg.ResourceNFSlots]
	//if slots.Cmp(resource.MustParse("1")) < 0 {
	//	return fmt.Errorf("no NF slots available on target")
	//}

	//if tableEntries, ok := allocatable[p4targetpkg.ResourceTableEntries]; ok {
	//	if tableEntries.Cmp(resource.MustParse("1")) < 0 {
	//		return fmt.Errorf("no table entries available on target")
	//	}
	//}

	return nil
}

// deploy pushes the compiled P4 program to the BMv2 switch. If the switch
// already has an identical program loaded (e.g. after a driver restart), the
// SetForwardingPipelineConfig RPC is skipped to avoid unnecessary disruption.
func (m *RealManager) deploy(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	key := client.ObjectKeyFromObject(nf)
	program := m.programs[key]

	if program == nil {
		return fmt.Errorf("no compiled program for %s", key)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	existing, err := m.switchClient.GetPipeline(reqCtx)
	if err == nil && existing.GetConfig() != nil &&
		bytes.Equal(existing.Config.P4DeviceConfig, program.P4DeviceConfig) {
		return nil
	}

	return m.switchClient.DeployPipeline(reqCtx, program)
}

// setupNetworkForwarding is a no-op when no NF interface is configured (e.g. in tests).
func (m *RealManager) setupNetworkForwarding(nf *corev1alpha1.NetworkFunction, ip netip.Prefix) error {
	key := client.ObjectKeyFromObject(nf)
	program := m.programs[key]

	if m.nfInterface == "" || program == nil || !ip.IsValid() {
		return fmt.Errorf("invalid parameters for setting up network forwarding")
	}
	if err := m.configureTrafficForwardingToNetworkFunctionInterface(ip, m.nfInterface); err != nil {
		return fmt.Errorf("could not configure traffic forwarding for NF: %w", err)
	}
	program.IP = ip
	return nil
}

// configureTrafficForwardingToNetworkFunctionInterface sets up the necessary routes and neighbor entries to ensure
// that traffic to the NF IP gets forwarded to the NF interface via the bridge. This is needed because the NF interface
// will have an IP in the same subnet as the bridge, so we need to ensure that traffic to that IP gets forwarded to the
// correct interface.
// In order for the return traffic to work correctly, we assume that the NF will reflect the packet back to
// the MAC address of the bridge interface. When doing so, the source and destination MAC addresses will be swapped,
// which allows the traffic to be correctly forwarded back to the bridge. For the destination IP, this will
// either be determined by the NF itself, or it will be the next segment in the SRv6 path; in both cases, this MUST
// be set by the NF. Afterwards, the packet will be routed by the bridge back to the iml0 interface out of the container
// and back to the routing VRF in the host.
func (m *RealManager) configureTrafficForwardingToNetworkFunctionInterface(nfIP netip.Prefix, nfIfaceName string) error {
	nfInterface, err := netlink.LinkByName(nfIfaceName)
	if err != nil {
		return fmt.Errorf("failed to get NF interface: %w", err)
	}
	nfAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   nfIP.Addr().Unmap().AsSlice(),
			Mask: net.CIDRMask(nfIP.Bits(), nfIP.Addr().BitLen()),
		},
	}
	if err := netlink.AddrAdd(nfInterface, nfAddr); err != nil {
		return fmt.Errorf("failed to add address for NF IP %s: %w", nfIP.String(), err)
	}
	return nil
}

func (m *RealManager) runDeletion(ctx context.Context, h *deploymentHandle, nfKey client.ObjectKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	logger := m.log
	defer func() {
		if r := recover(); r != nil {
			h.transition(PhaseFailed, "panic occurred", fmt.Errorf("%v", r))
		}
	}()

	logger.V(1).Info("Draining network function")
	h.transition(PhaseDraining, "draining traffic", nil)
	if err := m.drain(ctx, nfKey); err != nil {
		logger.V(1).Error(err, "failed to drain network function")
		h.transition(PhaseFailed, "drain failed", err)
		return
	}

	logger.V(1).Info("Deleting network function")
	h.transition(PhaseDeleting, "removing resources", nil)
	if err := m.deleteResources(ctx, nfKey); err != nil {
		logger.V(1).Error(err, "failed to delete resources")
		h.transition(PhaseFailed, "deletion failed", err)
		return
	}

	logger.V(1).Info("Network function deleted successfully")
	h.transition(PhaseDeleted, "successfully deleted", nil)
}

// drain is a no-op for BMv2: the switch has no per-NF traffic queuing to flush.
func (m *RealManager) drain(_ context.Context, _ client.ObjectKey) error {
	return nil
}

// deleteResources tears down network forwarding, resets the forwarding pipeline,
// and removes the compiled program artifact from the in-memory store.
func (m *RealManager) deleteResources(ctx context.Context, nfKey client.ObjectKey) error {
	nfOps, ok := m.ops[nfKey]
	if !ok {
		return nil // no tracked op for this NF, so nothing to delete
	}
	program, ok := m.programs[nfKey]
	if !ok {
		delete(m.ops, nfKey) // no compiled program, so just remove the op tracking
		return nil           // no compiled program, so nothing was deployed; just remove the op tracking
	}

	// If the NF was never fully deployed (e.g. failed during compilation or pre-check), then we can skip the teardown of forwarding rules
	// and just remove the compiled program, since it was never actually pushed to the switch.
	if nfOps.opType == opPresent && nfOps.handle.status.Phase != PhaseReady {
		delete(m.ops, nfKey) // NF never reached ready, so no forwarding rules were installed; just remove the compiled program
		delete(m.programs, nfKey)
		return nil
	}

	if err := m.teardownNetworkForwarding(program.IP, m.nfInterface); err != nil {
		return fmt.Errorf("tearing down network forwarding: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := m.switchClient.ResetPipeline(reqCtx); err != nil {
		return fmt.Errorf("resetting forwarding pipeline: %w", err)
	}

	return nil
}

// teardownNetworkForwarding removes the host route and neighbor entry that were
// installed by configureTrafficForwardingToNetworkFunctionInterface when the NF was deployed.
func (m *RealManager) teardownNetworkForwarding(nfIP netip.Prefix, nfIfaceName string) error {
	nfInterface, err := netlink.LinkByName(nfIfaceName)
	if err != nil {
		return fmt.Errorf("failed to get NF interface: %w", err)
	}
	nfAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   nfIP.Addr().Unmap().AsSlice(),
			Mask: net.CIDRMask(nfIP.Bits(), nfIP.Addr().BitLen()),
		},
	}
	if err := netlink.AddrDel(nfInterface, nfAddr); err != nil {
		return fmt.Errorf("failed to delete address for NF IP %s: %w", nfIP.String(), err)
	}
	// nfVeth, ok := nfInterface.(*netlink.Veth)
	// if !ok {
	// 	return nil // not a veth interface, so no neighbor entry to delete
	// }
	// nfPeer, err := netlink.LinkByName(nfVeth.PeerName)
	// if err != nil {
	// 	return fmt.Errorf("failed to get peer interface for NF veth: %w", err)
	// }
	// neighbor := &netlink.Neigh{
	// 	LinkIndex: nfPeer.Attrs().Index,
	// 	IP:        nfIP.Unmap().AsSlice(),
	// }
	// if err := netlink.NeighDel(neighbor); err != nil {
	// 	return fmt.Errorf("failed to delete neighbor entry for NF IP %s: %w", nfIP.String(), err)
	// }
	return nil
}
