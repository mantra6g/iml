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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// NetworkFunctionDeploymentReconciler reconciles a NetworkFunctionDeployment object
type NetworkFunctionDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctiondeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctiondeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctiondeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunctionDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	deployment := &schedulingv1alpha1.NetworkFunctionDeployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunctionDeployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling NetworkFunctionDeployment", "deployment", deployment.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionDeployment{}).
		Named("scheduling-networkfunctiondeployment").
		Complete(r)
}
