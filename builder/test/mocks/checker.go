package mocks

import (
	"context"

	corev1alpha1 "builder/api/core/v1alpha1"
	"builder/pkg/readiness"
)

type FakeReadinessChecker struct {
	Ready bool
}

func (f *FakeReadinessChecker) Check(ctx context.Context, target *corev1alpha1.P4Target) readiness.ReadyStatus {
	return readiness.ReadyStatus{
		Ready:   f.Ready,
		Reason:  "FakeReason",
		Message: "This is a fake readiness check",
	}
}
