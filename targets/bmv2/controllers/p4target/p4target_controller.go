/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package p4target

import (
	"bmv2-driver/controllers/lease"
	"bmv2-driver/controllers/p4target/utils"
	"bmv2-driver/managers/p4target"
	"context"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Reconciler reconciles a P4Target object
type Reconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	P4TargetManager p4target.Manager
	PullInterval    time.Duration
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	target := &corev1alpha1.P4Target{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.P4TargetManager.GetName()}, target); err != nil {
		if errors.IsNotFound(err) {
			err = r.createP4Target(ctx)
			if err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("P4Target created")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		logger.Error(err, "Failed to get P4Target")
		return ctrl.Result{}, err
	}

	if len(target.Spec.NfCIDR) == 0 {
		logger.V(1).Info("P4Target does not have any IP address. Requeueing after 10 seconds.")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, r.updateP4TargetStatus(ctx, target)
}

func (r *Reconciler) createP4Target(ctx context.Context) error {
	target := &corev1alpha1.P4Target{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.P4TargetManager.GetName(),
			Labels: map[string]string{
				corev1alpha1.P4TargetArchitectureLabel: "v1model",
			},
		},
		Spec: corev1alpha1.P4TargetSpec{},
	}
	return r.Create(ctx, target)
}

func (r *Reconciler) updateP4TargetStatus(ctx context.Context, target *corev1alpha1.P4Target) error {
	original := target.DeepCopy()
	target.Status = r.calculateP4TargetStatus()
	if utils.StatusChanged(original, target) {
		return r.Status().Patch(ctx, target, client.MergeFrom(original))
	}
	return nil
}

func (r *Reconciler) calculateP4TargetStatus() corev1alpha1.P4TargetStatus {
	targetStatus := corev1alpha1.P4TargetStatus{}
	targetStatus.Capacity = r.P4TargetManager.GetCapacity()
	targetStatus.Allocatable = r.P4TargetManager.GetAllocatable()
	targetStatus.Conditions = []corev1alpha1.P4TargetCondition{}
	targetStatus.Conditions = append(targetStatus.Conditions, r.P4TargetManager.GetReadyCondition())
	targetStatus.Conditions = append(targetStatus.Conditions, r.P4TargetManager.GetHealthyCondition())
	targetStatus.Conditions = append(targetStatus.Conditions, r.P4TargetManager.GetNetworkConfiguredCondition())
	targetStatus.Conditions = append(targetStatus.Conditions, r.P4TargetManager.GetOccupiedCondition())
	return targetStatus
}

func (r *Reconciler) Start(ctx context.Context) error {
	backoff := lease.InitialBackoff
	timer := time.NewTimer(0) // run immediately
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			res, err := r.Reconcile(ctx)
			if err != nil {
				// failure -> apply backoff and retry
				timer.Reset(backoff)

				backoff *= 2
				if backoff > lease.MaxBackoff {
					backoff = lease.MaxBackoff
				}
				continue
			}
			// success -> reset backoff
			backoff = lease.InitialBackoff

			// decide next schedule
			next := r.PullInterval
			if !res.IsZero() {
				next = res.RequeueAfter
			}
			timer.Reset(next)
		}
	}
}
