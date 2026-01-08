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

package core

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1alpha1 "builder/api/core/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	"builder/pkg/readiness"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	CONDITION_AVAILABLE   = "Available"
	CONDITION_INUSE       = "InUse"
	CONDITION_READINESS_PROBE_FAILED = "ReadinessProbeFailed"
	CONDITION_READINESS_PROBE_UNIMPLEMENTED = "ReadinessProbeUnimplemented"
	CONDITION_UNKNOWN     = "Unknown"
)

type CheckerRegistry map[corev1alpha1.TargetClass]readiness.Checker

// P4TargetReconciler reconciles a P4Target object
type P4TargetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Checkers CheckerRegistry
}

// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets/finalizers,verbs=update
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the P4Target object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
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

	checker, exists := r.Checkers[p4target.Spec.TargetClass]
	if !exists {
		logger.Error(nil, "Checker not found", "targetClass", p4target.Spec.TargetClass)
		p4target.Status.Ready = false
		r.setCondition(p4target, CONDITION_READINESS_PROBE_UNIMPLEMENTED)
		err := r.Client.Status().Update(ctx, p4target)
		return ctrl.Result{}, err
	}

	readyStatus := checker.Check(ctx, p4target)
	if !readyStatus.Ready {
		logger.Info("P4Target not ready", "reason", readyStatus.Reason, "message", readyStatus.Message)
		p4target.Status.Ready = false
		r.setCondition(p4target, CONDITION_READINESS_PROBE_FAILED)
		err := r.Client.Status().Update(ctx, p4target)
		return ctrl.Result{}, err
	}

	bindingList := &schedulingv1alpha1.NetworkFunctionBindingList{}
	if err := r.List(ctx, bindingList,
		client.MatchingLabels{
			schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL: p4target.Name,
		},
	); err != nil {
		logger.Error(err, "unable to list NetworkFunctionBindings")
		p4target.Status.Ready = false
		r.setCondition(p4target, CONDITION_UNKNOWN)
		err := r.Client.Status().Update(ctx, p4target)
		return ctrl.Result{}, err
	}
	if len(bindingList.Items) != 0 {
		p4target.Status.Ready = false
		r.setCondition(p4target, CONDITION_INUSE)
	} else {
		p4target.Status.Ready = true
		r.setCondition(p4target, CONDITION_AVAILABLE)
	}

	if err := r.Status().Update(ctx, p4target); err != nil {
		logger.Error(err, "Failed to update P4Target status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Checkers == nil {
		r.Checkers = make(CheckerRegistry)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.P4Target{}).
		// Watch Pods with label core.desire6g.eu/target
		// and enqueue reconcile calls when they change
		Watches(&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.mapPodToRequests),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return false
				}
				_, hasLabel := pod.Labels[corev1alpha1.TARGET_LABEL]
				return hasLabel
			}))).
		Watches(&schedulingv1alpha1.NetworkFunctionBinding{},
			handler.EnqueueRequestsFromMapFunc(r.mapBindingToRequests),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
				binding, ok := object.(*schedulingv1alpha1.NetworkFunctionBinding)
				if !ok {
					return false
				}
				_, hasLabel := binding.Labels[schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL]
				return hasLabel
			}))).
		Named("core-p4target").
		Complete(r)
}

func (r *P4TargetReconciler) mapPodToRequests(ctx context.Context, obj client.Object) []ctrl.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	targetName, exists := pod.Labels[corev1alpha1.TARGET_LABEL]
	if !exists {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name: targetName,
			},
		},
	}
}

func (r *P4TargetReconciler) mapBindingToRequests(ctx context.Context, obj client.Object) []ctrl.Request {
	binding, ok := obj.(*schedulingv1alpha1.NetworkFunctionBinding)
	if !ok {
		return nil
	}
	targetName, exists := binding.Labels[schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL]
	if !exists {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name: targetName,
			},
		},
	}
}

func (r *P4TargetReconciler) setCondition(p4target *corev1alpha1.P4Target, conditionType string) {
	switch conditionType {
	case CONDITION_AVAILABLE:
		p4target.Status.Ready = true
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Available",
			Message:            "P4Target is available for assignment",
			LastTransitionTime: metav1.Now(),
		}
		p4target.Status.Conditions = append(p4target.Status.Conditions, condition)
	case CONDITION_INUSE:
		p4target.Status.Ready = false
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "InUse",
			Message:            "P4Target is currently assigned to a NetworkFunction",
			LastTransitionTime: metav1.Now(),
		}
		p4target.Status.Conditions = append(p4target.Status.Conditions, condition)
	case CONDITION_READINESS_PROBE_FAILED:
		p4target.Status.Ready = false
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ReadinessProbeFailed",
			Message:            "P4Target readiness probe failed",
			LastTransitionTime: metav1.Now(),
		}
		p4target.Status.Conditions = append(p4target.Status.Conditions, condition)
	case CONDITION_READINESS_PROBE_UNIMPLEMENTED:
		p4target.Status.Ready = false
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ReadinessProbeUnimplemented",
			Message:            "P4Target readiness probe is unimplemented for this target class",
			LastTransitionTime: metav1.Now(),
		}
		p4target.Status.Conditions = append(p4target.Status.Conditions, condition)
	case CONDITION_UNKNOWN:
		p4target.Status.Ready = false
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Unknown",
			Message:            "P4Target status is unknown due to an error",
			LastTransitionTime: metav1.Now(),
		}
		p4target.Status.Conditions = append(p4target.Status.Conditions, condition)
	}
}