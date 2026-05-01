package nf

import (
	"context"
	"fmt"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager interface {
	GetDeployedNetworkFunctions() []client.ObjectKey
	EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle
	EnsureAbsent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle
}

type RealManager struct {
	ops map[client.ObjectKey]*trackedOp
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

func NewManager() (Manager, error) {
	return nil, fmt.Errorf("not implemented")
}

var _ Manager = &RealManager{}

func (m *RealManager) EnsurePresent(ctx context.Context, nf *corev1alpha1.NetworkFunction) DeploymentHandle {
	key := client.ObjectKeyFromObject(nf)

	if existing, ok := m.ops[key]; ok {
		if existing.opType == opPresent {
			return existing.handle // already deleting
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

	// TODO: Change the next code as you want. Remember to update the phase and message in the handle at
	//   each step, and to set the phase to PhaseFailed if any step fails. The current code is just a
	//   skeleton to show you how you might structure the deployment process.

	// --- Compile ---
	h.transition(PhaseCompiling, "compiling network function", nil)

	if err := m.compile(ctx, nf); err != nil {
		h.transition(PhaseFailed, "compilation failed", err)
		return
	}

	// --- Pre-check ---
	h.transition(PhasePreCheck, "running pre-deployment checks", nil)

	if err := m.preCheck(ctx, nf); err != nil {
		h.transition(PhaseFailed, "pre-check failed", err)
		return
	}

	// --- Deploy ---
	h.transition(PhaseDeploying, "deploying network function", nil)

	if err := m.deploy(ctx, nf); err != nil {
		h.transition(PhaseFailed, "deployment failed", err)
		return
	}

	// --- Done ---
	h.transition(PhaseReady, "network function ready", nil)
}

func (m *RealManager) compile(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	// TODO: implement or replace this function with another one
	return nil
}

func (m *RealManager) preCheck(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	// TODO: implement or replace this function with another one
	return nil
}

func (m *RealManager) deploy(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	// TODO: implement or replace this function with another one
	return nil
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

func (m *RealManager) deleteResources(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	return nil
}

func (m *RealManager) drain(ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	return nil
}
