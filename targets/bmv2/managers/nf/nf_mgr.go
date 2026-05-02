package nf

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"bmv2-driver/api"
	p4targetpkg "bmv2-driver/managers/p4target"
	"bmv2-driver/pkg/p4compile"
	p4switch "bmv2-driver/pkg/p4switch"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerConfig struct {
	Switch          *p4switch.SwitchClient
	P4TargetManager p4targetpkg.Manager
}

type Manager interface {
	GetDeployedNetworkFunctions(ctx context.Context) ([]client.ObjectKey, error)
	EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle
	EnsureAbsent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle
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
		programs:        make(map[client.ObjectKey]*api.P4Program),
		ops:             make(map[client.ObjectKey]*trackedOp),
	}, nil
}

func (m *RealManager) EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle {
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

	go m.runDeployment(ctx, h, nf)

	return h
}

func (m *RealManager) EnsureAbsent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle {
	key := client.ObjectKeyFromObject(nf)

	if existing, ok := m.ops[key]; ok {
		if existing.opType == opAbsent {
			return existing.handle // already deleting
		}
		// switching from deploy → delete
		existing.handle.Cancel()
	}

	ctx, cancel := context.WithCancel(ctx)
	h := newDeploymentHandle(cancel)

	m.ops[key] = &trackedOp{
		handle: h,
		opType: opAbsent,
	}

	go m.runDeletion(ctx, h, nf)

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

func (m *RealManager) runDeployment(ctx context.Context, h *deploymentHandle, nf *corev1alpha1.NetworkFunction) {
	defer func() {
		if r := recover(); r != nil {
			h.transition(PhaseFailed, "panic occurred", fmt.Errorf("%v", r))
		}
	}()

	h.transition(PhaseCompiling, "compiling network function", nil)
	if err := m.compile(ctx, nf); err != nil {
		h.transition(PhaseFailed, "compilation failed", err)
		return
	}

	h.transition(PhasePreCheck, "running pre-deployment checks", nil)
	if err := m.preCheck(ctx, nf); err != nil {
		h.transition(PhaseFailed, "pre-check failed", err)
		return
	}

	h.transition(PhaseDeploying, "deploying network function", nil)
	if err := m.deploy(ctx, nf); err != nil {
		h.transition(PhaseFailed, "deployment failed", err)
		return
	}

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

	m.mu.Lock()
	m.programs[key] = program
	m.mu.Unlock()
	return nil
}

// preCheck verifies there are available NF slots and table entries before deploying.
func (m *RealManager) preCheck(_ context.Context, _ *corev1alpha1.NetworkFunction) error {
	allocatable := m.p4targetManager.GetAllocatable()

	slots := allocatable[p4targetpkg.ResourceNFSlots]
	if slots.Cmp(resource.MustParse("1")) < 0 {
		return fmt.Errorf("no NF slots available on target")
	}

	if tableEntries, ok := allocatable[p4targetpkg.ResourceTableEntries]; ok {
		if tableEntries.Cmp(resource.MustParse("1")) < 0 {
			return fmt.Errorf("no table entries available on target")
		}
	}

	return nil
}

// deploy pushes the compiled P4 program to the BMv2 switch.
func (m *RealManager) deploy(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	key := client.ObjectKeyFromObject(nf)

	m.mu.Lock()
	program := m.programs[key]
	m.mu.Unlock()

	if program == nil {
		return fmt.Errorf("no compiled program for %s", key)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return m.switchClient.DeployPipeline(reqCtx, program)
}

func (m *RealManager) runDeletion(ctx context.Context, h *deploymentHandle, nf *corev1alpha1.NetworkFunction) {
	defer func() {
		if r := recover(); r != nil {
			h.transition(PhaseFailed, "panic occurred", fmt.Errorf("%v", r))
		}
	}()

	h.transition(PhaseDraining, "draining traffic", nil)
	if err := m.drain(ctx, nf); err != nil {
		h.transition(PhaseFailed, "drain failed", err)
		return
	}

	h.transition(PhaseDeleting, "removing resources", nil)
	if err := m.deleteResources(ctx, nf); err != nil {
		h.transition(PhaseFailed, "deletion failed", err)
		return
	}

	h.transition(PhaseDeleted, "successfully deleted", nil)
}

// drain is a no-op for BMv2: the switch has no per-NF traffic queuing to flush.
func (m *RealManager) drain(_ context.Context, _ *corev1alpha1.NetworkFunction) error {
	return nil
}

// deleteResources resets the forwarding pipeline on the switch and removes the
// compiled program artifact from the in-memory store.
func (m *RealManager) deleteResources(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := m.switchClient.ResetPipeline(reqCtx); err != nil {
		return fmt.Errorf("resetting forwarding pipeline: %w", err)
	}

	key := client.ObjectKeyFromObject(nf)
	m.mu.Lock()
	delete(m.programs, key)
	m.mu.Unlock()
	return nil
}
