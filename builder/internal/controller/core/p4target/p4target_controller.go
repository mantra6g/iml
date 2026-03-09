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
	"context"
	"time"
	
	v1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	
	corev1alpha1 "loom/api/core/v1alpha1"
	p4targetutil "loom/internal/controller/core/p4target/util"
)

const P4TargetLeaseNamespace = "p4target-leases"

// P4TargetReconciler reconciles a P4Target object
type P4TargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets/finalizers,verbs=update
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *P4TargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	p4target := &corev1alpha1.P4Target{}
	if err := r.Get(ctx, req.NamespacedName, p4target); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("P4Target resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch P4Target")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	lease, err := r.obtainLease(ctx, p4target)
	if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to obtain lease for P4Target")
		readyCondition := p4targetutil.NewReadyCondition(metav1.ConditionUnknown,
			"LeaseError", "Failed to obtain lease for P4Target")
		_ = r.ensureReadinessState(ctx, p4target, readyCondition) // best effort
		return ctrl.Result{}, err
	}

	readyCondition, err := r.calculateReadinessCondition(p4target, lease)
	if err != nil {
		logger.Error(err, "Failed to calculate readiness for P4Target")
		readyCondition = p4targetutil.NewReadyCondition(metav1.ConditionUnknown,
			"ReadinessCalculationError", "Failed to calculate readiness for P4Target")
		_ = r.ensureReadinessState(ctx, p4target, readyCondition) // best effort
		return ctrl.Result{}, err
	}

	err = r.ensureReadinessState(ctx, p4target, readyCondition)
	if err != nil {
		logger.Error(err, "Failed to ensure readiness state for P4Target")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.P4Target{}).
		// Watch Leases and enqueue reconcile calls for P4Targets with the same name
		// when they change
		Watches(&v1.Lease{},
			handler.EnqueueRequestsFromMapFunc(r.mapLeaseToRequests)).
		Named("core-p4target").
		Complete(r)
}

func (r *P4TargetReconciler) mapLeaseToRequests(ctx context.Context, obj client.Object) []ctrl.Request {
	lease, ok := obj.(*v1.Lease)
	if !ok {
		return nil
	}
	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name: lease.Name,
			},
		},
	}
}

func (r *P4TargetReconciler) obtainLease(ctx context.Context, p4target *corev1alpha1.P4Target) (*v1.Lease, error) {
	lease := &v1.Lease{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: p4target.Name, Namespace: P4TargetLeaseNamespace}, lease)
	return lease, err
}

func (r *P4TargetReconciler) calculateReadinessCondition(target *corev1alpha1.P4Target,
	lease *v1.Lease) (corev1alpha1.P4TargetCondition, error) {
	// If lease is nil, then the P4Target is not ready
	if lease == nil {
		return p4targetutil.NewReadyCondition(
			metav1.ConditionUnknown, "NoLease", "No lease found for P4Target",
		), nil
	}
	// If the lease details are unknown, then the P4Target's readiness is unknown
	if lease.Spec.RenewTime == nil {
		return p4targetutil.NewReadyCondition(
			metav1.ConditionUnknown, "MissingLeaseRenewTime", "Lease RenewTime is nil",
		), nil
	}
	if lease.Spec.LeaseDurationSeconds == nil {
		return p4targetutil.NewReadyCondition(
			metav1.ConditionUnknown, "MissingLeaseRenewTime", "LeaseDurationSeconds is nil",
		), nil
	}
	// If the lease has expired, then the P4Target's heartbeat is not healthy, and thus
	// we cannot tell if the P4Target is or isn't ready
	leaseDuration := time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	if metav1.Now().After(lease.Spec.RenewTime.Add(leaseDuration)) {
		return p4targetutil.NewReadyCondition(
			metav1.ConditionUnknown, "LeaseExpired", "Lease has expired for P4Target",
		), nil
	}

	// If the lease is valid, then we can trust the existing readiness condition on the P4Target, if it exists.
	// If it doesn't exist, then we should return an unknown readiness condition.
	readinessCondition := getCondition(target, corev1alpha1.P4_TARGET_CONDITION_READY)
	if readinessCondition == nil {
		return p4targetutil.NewReadyCondition(
			metav1.ConditionUnknown, "NoReadinessCondition", "No readiness condition found for P4Target",
		), nil
	}
	return *readinessCondition, nil
}

func getCondition(
	p4target *corev1alpha1.P4Target, conditionType corev1alpha1.P4TargetConditionType,
) *corev1alpha1.P4TargetCondition {
	for i := range p4target.Status.Conditions {
		if p4target.Status.Conditions[i].Type == conditionType {
			return &p4target.Status.Conditions[i]
		}
	}
	return nil
}

func (r *P4TargetReconciler) ensureReadinessTaints(p4target *corev1alpha1.P4Target,
	readinessCondition corev1alpha1.P4TargetCondition) (updated bool) {
	switch readinessCondition.Status {
	case metav1.ConditionTrue:
		// If the P4Target is ready, then we should remove the "unreachable" or "not-ready" taints
		return p4targetutil.RemoveTaints(p4target, []string{
			corev1alpha1.TaintP4TargetUnreachable,
			corev1alpha1.TaintP4TargetNotReady,
		})
	case metav1.ConditionFalse:
		// If the P4Target is not ready, then we should remove the "unreachable" taint (if any)
		// and add the "not-ready" taint to indicate that it's not ready
		removedTaints := p4targetutil.RemoveTaints(p4target, []string{
			corev1alpha1.TaintP4TargetUnreachable,
		})
		addedTaints := p4targetutil.AddTaints(p4target, []corev1alpha1.Taint{
			{
				Key:       corev1alpha1.TaintP4TargetNotReady,
				Effect:    corev1alpha1.TaintEffectNoSchedule,
				TimeAdded: metav1.Now(),
			},
		})
		return removedTaints || addedTaints
	default:
		// If the P4Target's readiness is unknown, then remove the "not-ready" taint (if any)
		// and add a "unreachable" taint to indicate that it's unreachable
		removedTaints := p4targetutil.RemoveTaints(p4target, []string{
			corev1alpha1.TaintP4TargetNotReady,
		})
		addedTaints := p4targetutil.AddTaints(p4target, []corev1alpha1.Taint{
			{
				Key:       corev1alpha1.TaintP4TargetUnreachable,
				Effect:    corev1alpha1.TaintEffectNoSchedule,
				TimeAdded: metav1.Now(),
			},
		})
		return removedTaints || addedTaints
	}
}

func (r *P4TargetReconciler) ensureReadinessState(ctx context.Context, p4target *corev1alpha1.P4Target,
	readyCondition corev1alpha1.P4TargetCondition) error {
	original := p4target.DeepCopy()
	updatedTaints := r.ensureReadinessTaints(p4target, readyCondition)
	updatedConditions := r.ensureReadinessConditions(p4target, readyCondition)

	if updatedTaints {
		// This also updates conditions if they've changed
		return r.Client.Patch(ctx, p4target, client.MergeFrom(original))
	}
	if updatedConditions {
		return r.Client.Status().Patch(ctx, p4target, client.MergeFrom(original))
	}
	return nil
}

func (r *P4TargetReconciler) ensureReadinessConditions(target *corev1alpha1.P4Target,
	condition corev1alpha1.P4TargetCondition) (updated bool) {
	existingReadyCondition := getCondition(target, corev1alpha1.P4_TARGET_CONDITION_READY)
	if existingReadyCondition == nil {
		// If there is no existing readiness condition, then we should add the new condition to the target's status
		target.Status.Conditions = append(target.Status.Conditions, condition)
		return true
	}
	// If there is an existing readiness condition, then we should update it with the new values
	// First check if the status has actually changed, and if it hasn't,
	// then we shouldn't update the condition to avoid unnecessary updates to the target's status
	if p4targetutil.ConditionsAreEqual(*existingReadyCondition, condition) {
		return false
	}
	existingReadyCondition.Status = condition.Status
	existingReadyCondition.Reason = condition.Reason
	existingReadyCondition.Message = condition.Message
	existingReadyCondition.LastTransitionTime = condition.LastTransitionTime
	return true
}
