package nf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"bmv2-driver/api"
	p4targetpkg "bmv2-driver/managers/p4target"

	oldproto "github.com/golang/protobuf/proto"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerConfig struct {
	P4Client        p4v1.P4RuntimeClient
	DeviceID        uint64
	ElectionIDHigh  uint64
	ElectionIDLow   uint64
	P4TargetManager p4targetpkg.Manager
}

type Manager interface {
	GetDeployedNetworkFunctions() []client.ObjectKey
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
	p4client        p4v1.P4RuntimeClient
	deviceID        uint64
	electionIDHigh  uint64
	electionIDLow   uint64
	p4targetManager p4targetpkg.Manager

	mu       sync.Mutex
	programs map[client.ObjectKey]*api.P4Program
	ops      map[client.ObjectKey]*trackedOp
}

// Compile-time assertion to ensure RealManager implements the Manager interface
var _ Manager = &RealManager{}

func NewManager(cfg ManagerConfig) (Manager, error) {
	if cfg.P4Client == nil {
		return nil, fmt.Errorf("P4Runtime client is required")
	}
	if cfg.P4TargetManager == nil {
		return nil, fmt.Errorf("P4Target manager is required")
	}
	return &RealManager{
		p4client:        cfg.P4Client,
		deviceID:        cfg.DeviceID,
		electionIDHigh:  cfg.ElectionIDHigh,
		electionIDLow:   cfg.ElectionIDLow,
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

func (m *RealManager) GetDeployedNetworkFunctions() []client.ObjectKey {
	deployed := make([]client.ObjectKey, 0, len(m.ops))
	for objKey, op := range m.ops {
		if op.opType == opPresent && op.handle.status.Phase == PhaseReady {
			deployed = append(deployed, objKey)
		}
	}
	return deployed
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
		program, err = compileFromURL(ctx, p4File, nf.Name)
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

	req := &p4v1.SetForwardingPipelineConfigRequest{
		DeviceId:   m.deviceID,
		ElectionId: &p4v1.Uint128{High: m.electionIDHigh, Low: m.electionIDLow},
		Action:     p4v1.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config: &p4v1.ForwardingPipelineConfig{
			P4DeviceConfig: program.P4DeviceConfig,
		},
	}
	if program.P4Info != nil {
		req.Config.P4Info = program.P4Info
	}

	_, err := m.p4client.SetForwardingPipelineConfig(reqCtx, req)
	return err
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

// deleteResources removes the compiled program artifact from the in-memory store.
func (m *RealManager) deleteResources(_ context.Context, nf *corev1alpha1.NetworkFunction) error {
	key := client.ObjectKeyFromObject(nf)
	m.mu.Lock()
	delete(m.programs, key)
	m.mu.Unlock()
	return nil
}

// compileFromURL downloads a P4 source file from the given URL, compiles it
// with p4c for the bmv2/v1model target, and returns the resulting P4Program.
func compileFromURL(ctx context.Context, fileURL, programName string) (*api.P4Program, error) {
	tmpDir, err := os.MkdirTemp("", "p4compile-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := tmpDir + "/input.p4"
	if err := downloadFile(fileURL, inputPath); err != nil {
		return nil, err
	}
	return compileP4File(ctx, inputPath, tmpDir, programName)
}

func compileP4File(ctx context.Context, inputPath, outDir, programName string) (*api.P4Program, error) {
	p4infoPath := outDir + "/p4info.bin"

	compileCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(compileCtx, "p4c",
		"--target", "bmv2",
		"--arch", "v1model",
		"--p4runtime-files", p4infoPath,
		"--p4runtime-format", "binary",
		"-o", outDir,
		inputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("p4c: %s", string(out))
	}

	jsonBytes, err := os.ReadFile(outDir + "/input.json")
	if err != nil {
		return nil, fmt.Errorf("reading compiled JSON: %w", err)
	}

	p4infoBytes, err := os.ReadFile(p4infoPath)
	if err != nil {
		return nil, fmt.Errorf("reading p4info: %w", err)
	}

	var p4info p4configv1.P4Info
	if err := oldproto.Unmarshal(p4infoBytes, &p4info); err != nil {
		return nil, fmt.Errorf("parsing p4info: %w", err)
	}

	return &api.P4Program{
		P4DeviceConfig: jsonBytes,
		ProgramName:    programName,
		P4Info:         &p4info,
	}, nil
}

func downloadFile(fileURL, destPath string) error {
	resp, err := http.Get(fileURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("downloading %q: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("downloading %q: HTTP %d", fileURL, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}
