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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "builder/api/core/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// P4TargetReconciler reconciles a P4Target object
type P4TargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.desire6g.eu,resources=p4targets/finalizers,verbs=update

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

	var p4target *corev1alpha1.P4Target
	if err := r.Get(ctx, req.NamespacedName, p4target); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("P4Target resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch P4Target")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *P4TargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.P4Target{}).
		Named("core-p4target").
		Complete(r)
}
