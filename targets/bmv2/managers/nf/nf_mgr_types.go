package nf

import (
	"context"
	"sync"
)

type Phase string

// TODO: make sure to add or remove any phases that are relevant for your deployment process.
//
//	The ones listed here are just examples.
const (
	// deploy phases
	PhaseCompiling Phase = "Compiling"
	PhasePreCheck  Phase = "PreCheck"
	PhaseDeploying Phase = "Deploying"
	PhaseReady     Phase = "Ready"

	// delete phases
	PhaseDraining Phase = "Draining"
	PhaseDeleting Phase = "Deleting"
	PhaseDeleted  Phase = "Deleted"

	// shared
	PhasePending  Phase = "Pending"
	PhaseFailed   Phase = "Failed"
	PhaseCanceled Phase = "Canceled"
)

type DeploymentHandle interface {
	// Events returns a stream of state transitions (non-blocking for producer)
	Events() <-chan DeploymentEvent

	// Done returns a channel which is closed exactly once when deployment reaches terminal state
	Done() <-chan struct{}

	// Status returns a snapshot of the current deployment state (useful for reconciler polling fallback)
	Status() DeploymentStatus

	// Err returns a terminal error or nil if succeeded
	Err() error

	// Cancel ongoing work
	Cancel() error
}

type DeploymentStatus struct {
	Phase   Phase
	Message string
	Err     error
}

type DeploymentEvent struct {
	Phase   Phase
	Message string
	Err     error
}

type deploymentHandle struct {
	mu sync.RWMutex

	status DeploymentStatus

	events chan DeploymentEvent
	done   chan struct{}

	cancel context.CancelFunc

	once sync.Once // ensures terminal state closes exactly once
}

func newDeploymentHandle(cancel context.CancelFunc) *deploymentHandle {
	return &deploymentHandle{
		status: DeploymentStatus{
			Phase: PhasePending,
		},
		events: make(chan DeploymentEvent, 10), // buffered to avoid blocking
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

func (h *deploymentHandle) Status() DeploymentStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *deploymentHandle) Err() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status.Err
}

func (h *deploymentHandle) Events() <-chan DeploymentEvent {
	return h.events
}

func (h *deploymentHandle) Done() <-chan struct{} {
	return h.done
}

func (h *deploymentHandle) Cancel() error {
	h.cancel()
	h.transition(PhaseCanceled, "deployment canceled", context.Canceled)
	return nil
}

func (h *deploymentHandle) transition(phase Phase, msg string, err error) {
	h.mu.Lock()
	h.status = DeploymentStatus{
		Phase:   phase,
		Message: msg,
		Err:     err,
	}
	h.mu.Unlock()

	h.emitEvent(phase, msg, err)

	if isTerminal(phase) {
		h.finish()
	}
}

func (h *deploymentHandle) emitEvent(phase Phase, msg string, err error) {
	evt := DeploymentEvent{
		Phase:   phase,
		Message: msg,
		Err:     err,
	}

	select {
	case h.events <- evt:
	default:
		// Drop if buffer full (critical: never block manager)
	}
}

func isTerminal(p Phase) bool {
	return p == PhaseReady || p == PhaseFailed || p == PhaseCanceled || p == PhaseDeleted
}

func (h *deploymentHandle) finish() {
	h.once.Do(func() {
		close(h.done)
		close(h.events)
	})
}
