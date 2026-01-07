package readiness

import (
	"context"
	"fmt"

	corev1alpha1 "builder/api/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Ready reason
	ReasonAvailable = "Available"
	// Not ready reasons
	ReasonPodNotFound           = "PodNotFound"
	ReasonPodNotReady           = "PodNotReady"
	ReasonControllerUnreachable = "ControllerUnreachable"
	ReasonHardwareUnreachable   = "HardwareUnreachable"
	ReasonProbeFailed           = "ProbeFailed"
	ReasonTargetInUse           = "TargetInUse"
	ReasonUnknown               = "Unknown"
)

type Checker interface {
	Check(ctx context.Context, target *corev1alpha1.P4Target) ReadyStatus
}

type ReadyStatus struct {
	Ready   bool
	Message string
	Reason  string
}

type PodBasedTargetChecker struct {
	Client client.Client
}

func (p *PodBasedTargetChecker) Check(ctx context.Context, target *corev1alpha1.P4Target) ReadyStatus {
	var pods corev1.PodList
	if err := p.Client.List(ctx, &pods,
		client.InNamespace(target.Namespace),
		client.MatchingLabels{
			"infra.desire6g.eu/target": target.Name,
		},
	); err != nil {
		if apierrors.IsNotFound(err) {
			return ReadyStatus{
				Ready:   false,
				Reason:  ReasonPodNotFound,
				Message: "Target's pod not found",
			}
		}
		return ReadyStatus{
			Ready:   false,
			Reason:  ReasonUnknown,
			Message: "Error listing target's pod: " + err.Error(),
		}
	}
	if len(pods.Items) == 0 {
		return ReadyStatus{
			Ready:   false,
			Reason:  ReasonPodNotFound,
			Message: "Target's pod not found",
		}
	}
	pod := pods.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return ReadyStatus{
			Ready:   false,
			Reason:  ReasonPodNotReady,
			Message: fmt.Sprintf("Target's pod is not ready (current phase: %s)", pod.Status.Phase),
		}
	}

	return ReadyStatus{
		Ready:   true,
		Message: "Pod is ready",
		Reason:  ReasonAvailable,
	}
}

type ExternalTargetChecker struct {
}

func (h *ExternalTargetChecker) Check(ctx context.Context, target *corev1alpha1.P4Target) ReadyStatus {
	// Implement hardware-based readiness check logic here
	return ReadyStatus{
		Ready:   false,
		Reason:  ReasonUnknown,
		Message: "Target class not implemented",
	}
}
