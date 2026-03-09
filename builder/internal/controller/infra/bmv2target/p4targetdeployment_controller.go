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

package bmv2target

import (
	"context"
	"fmt"
	
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	
	p4targetutil "loom/internal/controller/core/p4target/util"
	corev1alpha1 "loom/api/core/v1alpha1"
	infrav1alpha1 "loom/api/infra/v1alpha1"
	bmv2utils "loom/internal/controller/infra/bmv2target/util"
)

// P4TargetDeploymentReconciler reconciles a P4TargetDeployment object
type P4TargetDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.loom.io,resources=p4targetdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.loom.io,resources=p4targetdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.loom.io,resources=p4targetdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.loom.io,resources=p4targets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *P4TargetDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	bmv2Target := &infrav1alpha1.P4TargetDeployment{}
	err := r.Get(ctx, req.NamespacedName, bmv2Target)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("P4TargetDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch P4TargetDeployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	dep, err := r.ensureDeployment(ctx, bmv2Target)
	if err != nil {
		logger.Error(err, "failed to ensure bmv2Target has correct defaults and finalizers")
		return ctrl.Result{}, err
	}

	p4target, err := r.ensureP4Target(ctx, bmv2Target)
	if err != nil {
		logger.Error(err, "failed to ensure p4target has correct defaults and finalizers")
		return ctrl.Result{}, err
	}

	err = r.updateStatus(ctx, bmv2Target, p4target, dep)
	if err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.P4TargetDeployment{}).
		Owns(&corev1alpha1.P4Target{}).
		Owns(&appsv1.Deployment{}).
		Named("infra-p4targetdeployment").
		Complete(r)
}

func (r *P4TargetDeploymentReconciler) ensureP4Target(
	ctx context.Context, bmv2tgt *infrav1alpha1.P4TargetDeployment) (*corev1alpha1.P4Target, error) {
	p4target := &corev1alpha1.P4Target{
		ObjectMeta: metav1.ObjectMeta{
			Name: bmv2tgt.Name,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, p4target, func() error {
		p4target.Labels = bmv2utils.EnsureP4TargetLabels(bmv2tgt, p4target.Labels)
		p4target.Annotations = bmv2utils.EnsureP4TargetAnnotations(bmv2tgt, p4target.Annotations)
		p4target.Finalizers = bmv2utils.EnsureP4TargetFinalizers(bmv2tgt, p4target.Finalizers)
		p4target.Spec = *bmv2utils.EnsureP4TargetSpec(bmv2tgt, &p4target.Spec)
		return controllerutil.SetControllerReference(bmv2tgt, p4target, r.Scheme) // P4TargetDeployment owns P4Target
	})
	return p4target, err
}

func (r *P4TargetDeploymentReconciler) ensureDeployment(ctx context.Context,
	targetDeployment *infrav1alpha1.P4TargetDeployment) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetDeployment.Name,
			Namespace: targetDeployment.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, dep, func() error {
		dep.Labels = bmv2utils.EnsureBMv2DeploymentLabels(targetDeployment, dep.Labels)
		dep.Annotations = bmv2utils.EnsureBMv2DeploymentAnnotations(targetDeployment, dep.Annotations)
		dep.Finalizers = bmv2utils.EnsureBMv2DeploymentFinalizers(targetDeployment, dep.Finalizers)
		dep.Spec = *bmv2utils.EnsureBMv2DeploymentSpec(targetDeployment, &dep.Spec)
		return controllerutil.SetControllerReference(targetDeployment, dep, r.Scheme) // P4TargetDeployment owns Deployment
	})
	return dep, err
}

func (r *P4TargetDeploymentReconciler) updateStatus(
	ctx context.Context, bmv2Target *infrav1alpha1.P4TargetDeployment,
	p4target *corev1alpha1.P4Target, dep *appsv1.Deployment) error {
	newStatus := calculateStatus(bmv2Target, p4target, dep)

	original := bmv2Target.DeepCopy()
	bmv2Target.Status = *newStatus

	return r.Status().Patch(ctx, bmv2Target, client.MergeFrom(original))
}

func calculateStatus(bmv2Target *infrav1alpha1.P4TargetDeployment,
	p4target *corev1alpha1.P4Target, dep *appsv1.Deployment) *infrav1alpha1.P4TargetDeploymentStatus {
	status := &infrav1alpha1.P4TargetDeploymentStatus{
		ObservedGeneration: bmv2Target.Generation,
		Conditions:         make([]infrav1alpha1.BMv2TargetCondition, 0, len(bmv2Target.Status.Conditions)),
	}
	// Copy conditions from old status
	for i := range bmv2Target.Status.Conditions {
		status.Conditions = append(status.Conditions, bmv2Target.Status.Conditions[i])
	}
	if dep == nil {
		newReadyCondition := bmv2utils.NewReadyCondition(metav1.ConditionFalse,
			"DeploymentNotFound", "The Deployment for this P4TargetDeployment was not found.")
		status.Conditions = bmv2utils.UpdateBMv2TargetCondition(bmv2Target, newReadyCondition)
		return status
	}
	if p4target == nil {
		newReadyCondition := bmv2utils.NewReadyCondition(metav1.ConditionFalse,
			"P4TargetNotFound", "The P4Target for this P4TargetDeployment was not found.")
		status.Conditions = bmv2utils.UpdateBMv2TargetCondition(bmv2Target, newReadyCondition)
		return status
	}
	if dep.Status.AvailableReplicas < *dep.Spec.Replicas {
		newReadyCondition := bmv2utils.NewReadyCondition(metav1.ConditionFalse,
			"DeploymentNotReady",
			fmt.Sprintf("The Deployment for this P4TargetDeployment is not ready. Available replicas: %d/%d.",
				dep.Status.AvailableReplicas, *dep.Spec.Replicas))
		status.Conditions = bmv2utils.UpdateBMv2TargetCondition(bmv2Target, newReadyCondition)
		return status
	}
	p4targetReadyCondition := p4targetutil.GetReadyCondition(p4target)
	if p4targetReadyCondition == nil || p4targetReadyCondition.Status != metav1.ConditionTrue {
		newReadyCondition := bmv2utils.NewReadyCondition(metav1.ConditionFalse,
			"P4TargetNotReady",
			fmt.Sprintf("The P4Target for this P4TargetDeployment is not ready. P4Target condition: %v.",
				p4targetReadyCondition))
		status.Conditions = bmv2utils.UpdateBMv2TargetCondition(bmv2Target, newReadyCondition)
		return status
	}
	// If we reach this point, it means the Deployment is ready and the P4Target is ready
	newReadyCondition := bmv2utils.NewReadyCondition(metav1.ConditionTrue,
		"Ready", "The P4TargetDeployment is ready.")
	status.Conditions = bmv2utils.UpdateBMv2TargetCondition(bmv2Target, newReadyCondition)
	return status
}
