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

package networkfunction

import (
	"context"
	"fmt"
	nfutils "loom/internal/controller/core/networkfunction/util"
	"time"

	corev1alpha1 "loom/api/core/v1alpha1"
	p4targetutil "loom/internal/controller/core/p4target/util"
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

// NetworkFunctionReconciler reconciles a NetworkFunction object
type NetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctions/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	nf := &corev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, nf); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunction")
		return ctrl.Result{}, err
	}
	logger.Info("Reconciling NetworkFunction", "name", nf.Name, "namespace", nf.Namespace)

	// Check if being deleted
	if !nf.DeletionTimestamp.IsZero() {
		// Handle deletion
		if stringutils.ContainsElement(nf.GetFinalizers(), corev1alpha1.NetworkFunctionFinalizer) {
			// Remove finalizer
			nf.SetFinalizers(stringutils.RemoveElement(nf.GetFinalizers(), corev1alpha1.NetworkFunctionFinalizer))
			if err := r.Update(ctx, nf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !stringutils.ContainsElement(nf.Finalizers, corev1alpha1.NetworkFunctionFinalizer) {
		nf.Finalizers = append(nf.Finalizers, corev1alpha1.NetworkFunctionFinalizer)
		if err := r.Update(ctx, nf); err != nil {
			logger.Error(err, "failed to add finalizer to NetworkFunction")
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to NetworkFunction", "name", nf.Name)
	}

	// Schedule the nf to a P4Target if not already bound
	// TODO: This logic should be handled by a scheduler component in the control plane,
	//   but for simplicity we do it here for now. We can refactor later to move the scheduling
	//   logic to a separate component if needed.
	if nf.Spec.TargetName == "" {
		err := r.scheduleNetworkFunction(ctx, nf)
		if err != nil {
			logger.Error(err, "failed to schedule NetworkFunction")
			return ctrl.Result{}, err
		}
		logger.Info("Scheduled NetworkFunction to target",
			"targetName", nf.Spec.TargetName)
		// No need for requeue here because the scheduling logic issues an update operation to the nf's spec
		// with the chosen target, which will trigger another reconciliation loop
		return ctrl.Result{}, nil
	}

	// Once assigned, make sure the control-plane deployment
	// associated with this nf exists.
	operation, err := r.ensureControlPlaneDeployment(ctx, nf)
	if err != nil {
		logger.Error(err, "failed to ensure control plane deployment")
		_ = r.updateStatus(ctx, nf)
		return ctrl.Result{}, err
	}
	if operation != controllerutil.OperationResultNone {
		logger.Info("Control plane deployment updated")
		_ = r.updateStatus(ctx, nf)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Target is scheduled and control plane deployment is up to date,
	// we can update the status to reflect the current state
	return ctrl.Result{}, r.updateStatus(ctx, nf)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.NetworkFunction{}).
		Named("core-networkfunction").
		Complete(r)
}

func (r *NetworkFunctionReconciler) scheduleNetworkFunction(
	ctx context.Context, nf *corev1alpha1.NetworkFunction) error {
	logger := logf.FromContext(ctx)

	// List available P4Targets
	allTargets, err := r.listTargets(ctx, nf)
	if err != nil {
		logger.Error(err, "Failed to list P4Targets for NetworkFunction", "nf", nf)
		return err
	}

	feasible := filterFeasible(nf, allTargets)
	if len(feasible) == 0 {
		logger.Info("No feasible P4Targets found for NetworkFunction", "nf", nf)
		return fmt.Errorf("no feasible P4Targets found for NetworkFunction")
	}

	chosen := r.pickNode(nf, feasible)

	return r.updateNFWithChosenTarget(ctx, nf, chosen)
}

func (r *NetworkFunctionReconciler) updateStatus(ctx context.Context,
	nf *corev1alpha1.NetworkFunction) error {
	original := nf.DeepCopy()
	newStatus := calculateStatus(nf)
	nf.Status = newStatus
	return r.Status().Patch(ctx, nf, client.MergeFrom(original))
}

func calculateStatus(nf *corev1alpha1.NetworkFunction,
) corev1alpha1.NetworkFunctionStatus {
	status := corev1alpha1.NetworkFunctionStatus{
		ObservedGeneration: nf.Generation,
	}
	// Copy conditions to the new status
	status.Conditions = make([]corev1alpha1.NetworkFunctionCondition, len(nf.Status.Conditions))
	for i := range nf.Status.Conditions {
		status.Conditions = append(status.Conditions, nf.Status.Conditions[i])
	}
	if nf.Spec.TargetName == "" {
		newCondition := nfutils.NewScheduledCondition(metav1.ConditionFalse,
			"NotScheduled", "The NetworkFunction has not been scheduled to a target yet.")
		status.Conditions = nfutils.UpdateNFCondition(nf, newCondition)
		return status
	}
	newCondition := nfutils.NewScheduledCondition(metav1.ConditionTrue, "Scheduled",
		fmt.Sprintf("The NetworkFunction is scheduled to target %s.", nf.Spec.TargetName))
	status.Conditions = nfutils.UpdateNFCondition(nf, newCondition)
	return status
}

func (r *NetworkFunctionReconciler) listTargets(ctx context.Context,
	nf *corev1alpha1.NetworkFunction) ([]*corev1alpha1.P4Target, error) {
	targetLabelsSel, err := labels.ValidatedSelectorFromSet(nf.Spec.TargetSelector)
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
	nf *corev1alpha1.NetworkFunction, allTargets []*corev1alpha1.P4Target) []*corev1alpha1.P4Target {
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

func (r *NetworkFunctionReconciler) pickNode(
	nf *corev1alpha1.NetworkFunction, feasible []*corev1alpha1.P4Target) *corev1alpha1.P4Target {
	// Example of how we could use a ML model to get recommendations
	// rec := nf.Status.Recommendations
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

func (r *NetworkFunctionReconciler) ensureControlPlaneDeployment(
	ctx context.Context, nf *corev1alpha1.NetworkFunction) (controllerutil.OperationResult, error) {
	logger := logf.FromContext(ctx)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nf.Name + "-ctrl",
			Namespace: nf.Namespace,
		},
	}
	return controllerutil.CreateOrPatch(ctx, r.Client, dep, func() error {
		replicas := int32(1)
		dep.Spec.Replicas = &replicas

		// Ensure selector and template labels match for proper ownership
		dep.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				corev1alpha1.CONTROL_PLANE_POD_LABEL: nf.Name,
			},
		}
		dep.Spec.Template.Labels = map[string]string{
			corev1alpha1.CONTROL_PLANE_POD_LABEL: nf.Name,
		}

		// Ensure scheduling hints
		dep.Spec.Template.Spec.NodeSelector = nf.Spec.ControlPlane.NodeSelector
		dep.Spec.Template.Spec.Tolerations = nf.Spec.ControlPlane.Tolerations
		dep.Spec.Template.Spec.Affinity = nf.Spec.ControlPlane.Affinity

		// Ensure containers
		if dep.Spec.Template.Spec.Containers == nil || len(dep.Spec.Template.Spec.Containers) != 1 {
			dep.Spec.Template.Spec.Containers = make([]corev1.Container, 1)
		}
		dep.Spec.Template.Spec.Containers[0].Image = nf.Spec.ControlPlane.Image
		dep.Spec.Template.Spec.Containers[0].ImagePullPolicy = nf.Spec.ControlPlane.ImagePullPolicy
		dep.Spec.Template.Spec.Containers[0].Resources = nf.Spec.ControlPlane.Resources
		dep.Spec.Template.Spec.Containers[0].Args = nf.Spec.ControlPlane.Args

		// Validate and ensure environment variables
		validEnvs := make([]corev1.EnvVar, 0)
		for _, env := range nf.Spec.ControlPlane.ExtraEnv {
			if env.Name == "" {
				return fmt.Errorf("invalid environment variable with empty name in ControlPlaneSpec.ExtraEnv")
			}
			if env.Name == corev1alpha1.CONTROL_PLANE_POD_NF_NAMESPACE_ENV_VAR_KEY ||
				env.Name == corev1alpha1.CONTROL_PLANE_POD_NF_NAME_ENV_VAR_KEY {
				logger.Info("Skipping environment variable with reserved name", "name", env.Name)
			}
			validEnvs = append(validEnvs, env)
		}
		// Add mandatory environment variables for the control plane to know its nf context
		validEnvs = append(validEnvs, corev1.EnvVar{
			Name:  corev1alpha1.CONTROL_PLANE_POD_NF_NAME_ENV_VAR_KEY,
			Value: nf.Name,
		})
		validEnvs = append(validEnvs, corev1.EnvVar{
			Name:  corev1alpha1.CONTROL_PLANE_POD_NF_NAMESPACE_ENV_VAR_KEY,
			Value: nf.Namespace,
		})
		dep.Spec.Template.Spec.Containers[0].Env = validEnvs

		return controllerutil.SetControllerReference(nf, dep, r.Scheme)
	})
}

func (r *NetworkFunctionReconciler) updateNFWithChosenTarget(ctx context.Context,
	nf *corev1alpha1.NetworkFunction, chosen *corev1alpha1.P4Target) error {
	original := nf.DeepCopy()
	nf.Spec.TargetName = chosen.Name
	return r.Patch(ctx, nf, client.MergeFrom(original))
}
