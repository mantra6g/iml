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

package lease

import (
	"bmv2-driver/managers/p4target"
	"context"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	coordv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	InitialBackoff = 1 * time.Second
	MaxBackoff     = 16 * time.Second
)

type Result struct {
	RecheckAfter time.Duration
}

func (r Result) IsZero() bool {
	return r.RecheckAfter == 0
}

type Renewer struct {
	client.Client
	Scheme          *runtime.Scheme
	RenewInterval   time.Duration
	LeaseDuration   time.Duration
	P4TargetManager p4target.Manager
}

func (r *Renewer) createLease(ctx context.Context, now metav1.MicroTime) error {
	lease := &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.P4TargetManager.GetName(),
			Namespace: corev1alpha1.P4TargetLeaseNamespace,
		},
		Spec: coordv1.LeaseSpec{
			HolderIdentity:       new(r.P4TargetManager.GetName()),
			LeaseDurationSeconds: new(int32(r.LeaseDuration.Seconds())),
			RenewTime:            &now,
			AcquireTime:          &now,
		},
	}
	err := r.Create(ctx, lease)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *Renewer) updateLease(ctx context.Context, lease *coordv1.Lease, now metav1.MicroTime) error {
	original := lease.DeepCopy()
	lease.Spec.HolderIdentity = new(r.P4TargetManager.GetName())
	lease.Spec.LeaseDurationSeconds = new(int32(r.LeaseDuration.Seconds()))
	lease.Spec.RenewTime = &now
	lease.Spec.AcquireTime = &now
	return r.Patch(ctx, lease, client.MergeFrom(original))
}

func (r *Renewer) Reconcile(ctx context.Context) (Result, error) {
	now := metav1.MicroTime{Time: time.Now()}

	lease := &coordv1.Lease{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      r.P4TargetManager.GetName(),
		Namespace: corev1alpha1.P4TargetLeaseNamespace,
	}, lease)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.createLease(ctx, now)
			if err != nil {
				return Result{}, err
			}
			return Result{RecheckAfter: 2 * time.Second}, nil
		}
		return Result{}, err
	}

	err = r.updateLease(ctx, lease, now)
	return Result{}, err
}

func (r *Renewer) Start(ctx context.Context) error {
	backoff := InitialBackoff
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
				if backoff > MaxBackoff {
					backoff = MaxBackoff
				}
				continue
			}
			// success -> reset backoff
			backoff = InitialBackoff

			// decide next schedule
			next := r.RenewInterval
			if !res.IsZero() {
				next = res.RecheckAfter
			}
			timer.Reset(next)
		}
	}
}
