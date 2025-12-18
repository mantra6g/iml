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

package infra

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "builder/api/core/v1alpha1"
	infrav1alpha1 "builder/api/infra/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

const TargetDeploymentNamespace = "iml-infra"

type CNIArgs struct {
	AppType  string `json:"app_type"`
	AppID    string `json:"app_id,omitempty"`
	TargetID string `json:"target_id,omitempty"`
}

type CNIConfig struct {
	Name    string  `json:"name"`
	CNIArgs CNIArgs `json:"cni-args"`
}

func (c CNIConfig) String() string {
	return `{
		"name": "` + c.Name + `",
		"cni-args": {
			"app_type": "` + c.CNIArgs.AppType + `",
			"app_id": "` + c.CNIArgs.AppID + `",
			"target_id": "` + c.CNIArgs.TargetID + `"
		}
	}`
}

// P4TargetSetReconciler reconciles a P4TargetSet object
type P4TargetSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.desire6g.eu,resources=p4targetsets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the P4TargetSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *P4TargetSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	targetSet := &infrav1alpha1.P4TargetSet{}
	err := r.Get(ctx, req.NamespacedName, targetSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("P4TargetSet resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch P4TargetSet")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var targetDeployment appsv1.Deployment
	err = r.Get(ctx, client.ObjectKey{Name: targetSet.ObjectMeta.Name, Namespace: TargetDeploymentNamespace}, &targetDeployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create switch deployment
			newDeployment := r.createSwitchDeploymentFromSet(targetSet)
			if err := r.Create(ctx, newDeployment); err != nil {
				logger.Error(err, "Failed to create P4Target Deployment", "name", targetSet.ObjectMeta.Name)
				return ctrl.Result{}, err
			}
		}
		logger.Error(err, "Failed to get P4Target Deployment", "name", targetSet.ObjectMeta.Name)
		return ctrl.Result{}, err
	}

	var target corev1alpha1.P4Target
	if err := r.Get(ctx, client.ObjectKey{Name: targetSet.ObjectMeta.Name}, &target); err != nil {
		if apierrors.IsNotFound(err) {
			// Create P4Target
			newTarget := r.createP4TargetFromSet(targetSet)
			if err := r.Create(ctx, newTarget); err != nil {
				logger.Error(err, "Failed to create P4Target", "name", targetSet.ObjectMeta.Name)
				return ctrl.Result{}, err
			}
		}
		logger.Error(err, "Failed to get P4Target", "name", targetSet.ObjectMeta.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.P4TargetSet{}).
		Owns(&corev1alpha1.P4Target{}).
		Named("infra-p4targetset").
		Complete(r)
}

func (r *P4TargetSetReconciler) createSwitchDeploymentFromSet(targetSet *infrav1alpha1.P4TargetSet) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetSet.ObjectMeta.Name,
			Namespace: TargetDeploymentNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: targetSet.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": targetSet.ObjectMeta.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": targetSet.ObjectMeta.Name,
					},
					Annotations: map[string]string{
						"k8s.v1.cni.cncf.io/networks": "[" + CNIConfig{
							Name: "iml-cni",
							CNIArgs: CNIArgs{
								AppType:  "p4-target",
								TargetID: string(targetSet.UID),
							},
						}.String() + "]",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "control-plane",
							Image: "tomasagata/p4target-control-plane:latest",
						},
						{
							Name: "data-plane",
							Image: "tomasagata/p4target-data-plane:latest",
						},
					},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment, ensuring that the Deployment
	// will be deleted when the P4TargetSet CR is deleted.
	controllerutil.SetControllerReference(targetSet, dep, r.Scheme)
	return dep
}

func (r *P4TargetSetReconciler) createP4TargetFromSet(targetSet *infrav1alpha1.P4TargetSet) *corev1alpha1.P4Target {
	target := &corev1alpha1.P4Target{
		ObjectMeta: metav1.ObjectMeta{
			Name: targetSet.ObjectMeta.Name,
		},
		Spec: corev1alpha1.P4TargetSpec{
			TargetClass: "bmv2",
		},
	}

	// Set the ownerRef for the P4Target, ensuring that the P4Target
	// will be deleted when the P4TargetSet CR is deleted.
	controllerutil.SetControllerReference(targetSet, target, r.Scheme)
	return target
}