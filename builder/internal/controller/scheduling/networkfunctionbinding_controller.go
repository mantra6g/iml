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

package scheduling

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "builder/api/core/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const finalizerName = "scheduling.desire6g.eu/finalizer"

// NetworkFunctionBindingReconciler reconciles a NetworkFunctionBinding object
type NetworkFunctionBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunctionBinding object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	binding := &schedulingv1alpha1.NetworkFunctionBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionBinding resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunctionBinding")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling NetworkFunctionBinding", "name", binding.Name, "namespace", binding.Namespace)

	// Check if being deleted
	if !binding.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(binding.GetFinalizers(), finalizerName) {
			// Perform any cleanup logic here before removing finalizer

			// r.Bus.Publish(events.Event{
			// 	Name:    events.EventNfBindingDeleted,
			// 	Payload: binding,
			// })
			// ... any additional cleanup logic ...

			// Remove finalizer
			binding.SetFinalizers(removeString(binding.GetFinalizers(), finalizerName))
			if err := r.Update(ctx, binding); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	const finalizerName = "scheduling.desire6g.eu/finalizer"
	if !containsString(binding.Finalizers, finalizerName) {
		binding.Finalizers = append(binding.Finalizers, finalizerName)
		if err := r.Update(ctx, binding); err != nil {
			logger.Error(err, "failed to add finalizer to NetworkFunctionBinding")
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to NetworkFunctionBinding", "name", binding.Name)
	}

	// Schedule the binding to a P4Target if not already bound
	if binding.Status.AssignedTarget == "" {
		return r.scheduleBinding(ctx, binding)
	}

	// All is well
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionBinding{}).
		Named("scheduling-networkfunctionbinding").
		Complete(r)
}

func (r *NetworkFunctionBindingReconciler) scheduleBinding(ctx context.Context, binding *schedulingv1alpha1.NetworkFunctionBinding) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// List available P4Targets
	targetList := &corev1alpha1.P4TargetList{}
	if err := r.List(ctx, targetList); err != nil {
		logger.Error(err, "unable to list P4Targets")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	feasible := filterFeasible(binding, targetList.Items)
	if len(feasible) == 0 {
		logger.Info("No feasible P4Targets found for NetworkFunctionBinding", "binding", binding)
		binding.Status.Phase = "Pending"
		_ = r.Status().Update(ctx, binding)
		// Requeue after some time to try again
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	chosen := r.pickNode(binding, feasible)

	binding.Labels[schedulingv1alpha1.TARGET_ASSIGNMENT_LABEL] = chosen.Name
	binding.Status.AssignedTarget = chosen.Name
	binding.Status.Phase = "Scheduled"

	if err := r.Status().Update(ctx, binding); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func filterFeasible(binding *schedulingv1alpha1.NetworkFunctionBinding, targetList []corev1alpha1.P4Target) []corev1alpha1.P4Target {
	feasible := []corev1alpha1.P4Target{}
	for _, t := range targetList {
		// Match supported architecture
		supportedArchitectures := binding.Spec.SupportedTargets
		targetArch := t.Spec.TargetClass

		if !matchesSupportedArchitectures(targetArch, supportedArchitectures) {
			continue
		}

		// Is target not ready/occupied? => skip
		if t.Status.Phase != "Ready" {
			continue
		}

		// TODO: Add resource availability checks here

		feasible = append(feasible, t)
	}
	return feasible
}

func matchesSupportedArchitectures(targetArch string, architectures []string) bool {
	for _, arch := range architectures {
		if arch == targetArch {
			return true
		}
	}
	return false
}

func (r *NetworkFunctionBindingReconciler) pickNode(binding *schedulingv1alpha1.NetworkFunctionBinding, feasible []corev1alpha1.P4Target) *corev1alpha1.P4Target {
	// Example of how we could use a ML model to get recommendations
	// rec := binding.Status.Recommendations
	// if rec.TargetNode != "" && rec.Confidence > 0.7 {
	// 		for _, n := range nodes {
	// 				if n.Name == rec.TargetNode {
	// 						return n
	// 				}
	// 		}
	// }

	// Example of how we could sort based on available resources
	// and return the best one
	// sort.Slice(feasible, func(i, j int) bool {
	// 	return feasible[i].Status.Allocatable.CPU >
	// 					feasible[j].Status.Allocatable.CPU
	// })

	// For now, just return the first feasible target
	chosen := feasible[0]
	return &chosen
}
