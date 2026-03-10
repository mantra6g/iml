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

package nf_binding

import (
	"context"
	"fmt"
	"time"

	corev1alpha1 "loom/api/core/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	p4targetutil "loom/internal/controller/core/p4target/util"
	bindingutils "loom/internal/controller/scheduling/nf_binding/util"
	stringutils "loom/pkg/util/string"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// NetworkFunctionBindingReconciler reconciles a NetworkFunctionBinding object
type NetworkFunctionBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
		return ctrl.Result{}, err
	}
	logger.Info("Reconciling NetworkFunctionBinding", "name", binding.Name, "namespace", binding.Namespace)

	// Check if being deleted
	if !binding.DeletionTimestamp.IsZero() {
		// Handle deletion
		if stringutils.ContainsElement(binding.GetFinalizers(), schedulingv1alpha1.BINDING_FINALIZER_LABEL) {
			// Remove finalizer
			binding.SetFinalizers(stringutils.RemoveElement(binding.GetFinalizers(), schedulingv1alpha1.BINDING_FINALIZER_LABEL))
			if err := r.Update(ctx, binding); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !stringutils.ContainsElement(binding.Finalizers, schedulingv1alpha1.BINDING_FINALIZER_LABEL) {
		binding.Finalizers = append(binding.Finalizers, schedulingv1alpha1.BINDING_FINALIZER_LABEL)
		if err := r.Update(ctx, binding); err != nil {
			logger.Error(err, "failed to add finalizer to NetworkFunctionBinding")
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to NetworkFunctionBinding", "name", binding.Name)
	}

	// Schedule the binding to a P4Target if not already bound
	// TODO: This logic should be handled by a scheduler component in the control plane,
	//   but for simplicity we do it here for now. We can refactor later to move the scheduling
	//   logic to a separate component if needed.
	if binding.Spec.TargetName == "" {
		err := r.scheduleBinding(ctx, binding)
		if err != nil {
			logger.Error(err, "failed to schedule NetworkFunctionBinding")
			return ctrl.Result{}, err
		}
		logger.Info("Scheduled NetworkFunctionBinding to target",
			"targetName", binding.Spec.TargetName)
		// No need for requeue here because the scheduling logic issues an update operation to the binding's spec
		// with the chosen target, which will trigger another reconciliation loop
		return ctrl.Result{}, nil
	}

	// Once assigned, make sure the control-plane deployment
	// associated with this binding exists.
	operation, err := r.ensureControlPlaneDeployment(ctx, binding)
	if err != nil {
		logger.Error(err, "failed to ensure control plane deployment")
		_ = r.updateStatus(ctx, binding)
		return ctrl.Result{}, err
	}
	if operation != controllerutil.OperationResultNone {
		logger.Info("Control plane deployment updated")
		_ = r.updateStatus(ctx, binding)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Target is scheduled and control plane deployment is up to date,
	// we can update the status to reflect the current state
	return ctrl.Result{}, r.updateStatus(ctx, binding)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionBinding{}).
		Named("scheduling-networkfunctionbinding").
		Complete(r)
}

func (r *NetworkFunctionBindingReconciler) scheduleBinding(
	ctx context.Context, binding *schedulingv1alpha1.NetworkFunctionBinding) error {
	logger := logf.FromContext(ctx)

	// List available P4Targets
	allTargets, err := r.listTargets(ctx, binding)
	if err != nil {
		logger.Error(err, "Failed to list P4Targets for NetworkFunctionBinding", "binding", binding)
		return err
	}

	feasible := filterFeasible(binding, allTargets)
	if len(feasible) == 0 {
		logger.Info("No feasible P4Targets found for NetworkFunctionBinding", "binding", binding)
		return fmt.Errorf("no feasible P4Targets found for NetworkFunctionBinding")
	}

	chosen := r.pickNode(binding, feasible)

	return r.updateBindingWithChosenTarget(ctx, binding, chosen)
}

func (r *NetworkFunctionBindingReconciler) updateStatus(ctx context.Context,
	binding *schedulingv1alpha1.NetworkFunctionBinding) error {
	original := binding.DeepCopy()
	newStatus := calculateStatus(binding)
	binding.Status = newStatus
	return r.Status().Patch(ctx, binding, client.MergeFrom(original))
}

func calculateStatus(binding *schedulingv1alpha1.NetworkFunctionBinding,
) schedulingv1alpha1.NetworkFunctionBindingStatus {
	status := schedulingv1alpha1.NetworkFunctionBindingStatus{
		ObservedGeneration: binding.Generation,
	}
	// Copy conditions to the new status
	status.Conditions = make([]schedulingv1alpha1.BindingCondition, len(binding.Status.Conditions))
	for i := range binding.Status.Conditions {
		status.Conditions = append(status.Conditions, binding.Status.Conditions[i])
	}
	if binding.Spec.TargetName == "" {
		newCondition := bindingutils.NewScheduledCondition(metav1.ConditionFalse,
			"NotScheduled", "The NetworkFunctionBinding has not been scheduled to a target yet.")
		status.Conditions = bindingutils.UpdateBindingCondition(binding, newCondition)
		return status
	}
	newCondition := bindingutils.NewScheduledCondition(metav1.ConditionTrue, "Scheduled",
		fmt.Sprintf("The NetworkFunctionBinding is scheduled to target %s.", binding.Spec.TargetName))
	status.Conditions = bindingutils.UpdateBindingCondition(binding, newCondition)
	return status
}

func (r *NetworkFunctionBindingReconciler) listTargets(ctx context.Context,
	binding *schedulingv1alpha1.NetworkFunctionBinding) ([]*corev1alpha1.P4Target, error) {
	targetLabelsSel, err := labels.ValidatedSelectorFromSet(binding.Spec.TargetSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to create label selector: %w", err)
	}
	targetList := &corev1alpha1.P4TargetList{}
	if err := r.List(ctx, targetList, client.MatchingLabelsSelector{Selector: targetLabelsSel}); err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}
	targets := make([]*corev1alpha1.P4Target, 0, len(targetList.Items))
	for i := range targetList.Items {
		targets = append(targets, &targetList.Items[i])
	}
	return targets, nil
}

func filterFeasible(
	binding *schedulingv1alpha1.NetworkFunctionBinding, allTargets []*corev1alpha1.P4Target) []*corev1alpha1.P4Target {
	feasible := make([]*corev1alpha1.P4Target, 0)
	for _, t := range allTargets {
		// Skip targets with taints
		hasUnschedulableTaint := p4targetutil.HasTaintWithEffects(
			t, corev1alpha1.TaintEffectNoSchedule, corev1alpha1.TaintEffectNoExecute)

		if hasUnschedulableTaint {
			continue
		}

		// TODO: Add resource availability checks here

		feasible = append(feasible, t)
	}
	return feasible
}

func (r *NetworkFunctionBindingReconciler) pickNode(
	binding *schedulingv1alpha1.NetworkFunctionBinding, feasible []*corev1alpha1.P4Target) *corev1alpha1.P4Target {
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
	return chosen
}

func (r *NetworkFunctionBindingReconciler) ensureControlPlaneDeployment(
	ctx context.Context, binding *schedulingv1alpha1.NetworkFunctionBinding) (controllerutil.OperationResult, error) {
	logger := logf.FromContext(ctx)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.Name + "-ctrl",
			Namespace: binding.Namespace,
		},
	}
	return controllerutil.CreateOrPatch(ctx, r.Client, dep, func() error {
		replicas := int32(1)
		dep.Spec.Replicas = &replicas

		// Ensure selector and template labels match for proper ownership
		dep.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				schedulingv1alpha1.CONTROL_PLANE_POD_LABEL: binding.Name,
			},
		}
		dep.Spec.Template.Labels = map[string]string{
			schedulingv1alpha1.CONTROL_PLANE_POD_LABEL: binding.Name,
		}

		// Ensure scheduling hints
		dep.Spec.Template.Spec.NodeSelector = binding.Spec.ControlPlane.NodeSelector
		dep.Spec.Template.Spec.Tolerations = binding.Spec.ControlPlane.Tolerations
		dep.Spec.Template.Spec.Affinity = binding.Spec.ControlPlane.Affinity

		// Ensure containers
		if dep.Spec.Template.Spec.Containers == nil || len(dep.Spec.Template.Spec.Containers) != 1 {
			dep.Spec.Template.Spec.Containers = make([]corev1.Container, 1)
		}
		dep.Spec.Template.Spec.Containers[0].Image = binding.Spec.ControlPlane.Image
		dep.Spec.Template.Spec.Containers[0].ImagePullPolicy = binding.Spec.ControlPlane.ImagePullPolicy
		dep.Spec.Template.Spec.Containers[0].Resources = binding.Spec.ControlPlane.Resources
		dep.Spec.Template.Spec.Containers[0].Args = binding.Spec.ControlPlane.Args

		// Validate and ensure environment variables
		validEnvs := make([]corev1.EnvVar, 0)
		for _, env := range binding.Spec.ControlPlane.ExtraEnv {
			if env.Name == "" {
				return fmt.Errorf("invalid environment variable with empty name in ControlPlaneSpec.ExtraEnv")
			}
			if env.Name == schedulingv1alpha1.CONTROL_PLANE_POD_BINDING_NAMESPACE_ENV_VAR_KEY ||
				env.Name == schedulingv1alpha1.CONTROL_PLANE_POD_BINDING_NAME_ENV_VAR_KEY {
				logger.Info("Skipping environment variable with reserved name", "name", env.Name)
			}
			validEnvs = append(validEnvs, env)
		}
		// Add mandatory environment variables for the control plane to know its binding context
		validEnvs = append(validEnvs, corev1.EnvVar{
			Name:  schedulingv1alpha1.CONTROL_PLANE_POD_BINDING_NAME_ENV_VAR_KEY,
			Value: binding.Name,
		})
		validEnvs = append(validEnvs, corev1.EnvVar{
			Name:  schedulingv1alpha1.CONTROL_PLANE_POD_BINDING_NAMESPACE_ENV_VAR_KEY,
			Value: binding.Namespace,
		})
		dep.Spec.Template.Spec.Containers[0].Env = validEnvs

		return controllerutil.SetControllerReference(binding, dep, r.Scheme)
	})
}

func (r *NetworkFunctionBindingReconciler) updateBindingWithChosenTarget(ctx context.Context,
	binding *schedulingv1alpha1.NetworkFunctionBinding, chosen *corev1alpha1.P4Target) error {
	original := binding.DeepCopy()
	binding.Spec.TargetName = chosen.Name
	return r.Patch(ctx, binding, client.MergeFrom(original))
}
