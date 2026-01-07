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
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	NetworkFunctionReplicaSetPhaseUpscaling   = "Upscaling"
	NetworkFunctionReplicaSetPhaseDownscaling = "Downscaling"
)

// NetworkFunctionReplicaSetReconciler reconciles a NetworkFunctionReplicaSet object
type NetworkFunctionReplicaSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionreplicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionreplicasets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.desire6g.eu,resources=networkfunctionreplicasets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunctionReplicaSet object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReplicaSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	replicaSet := &schedulingv1alpha1.NetworkFunctionReplicaSet{}
	if err := r.Get(ctx, req.NamespacedName, replicaSet); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionReplicaSet resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch NetworkFunctionReplicaSet")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Reconciling NetworkFunctionReplicaSet", "name", replicaSet.Name, "namespace", replicaSet.Namespace)

	var bindingList schedulingv1alpha1.NetworkFunctionBindingList
	if err := r.List(ctx, &bindingList); err != nil {
		logger.Error(err, "unable to list NetworkFunctionBindings")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if len(bindingList.Items) < int(*replicaSet.Spec.Replicas) {
		// Create missing bindings

		missingBindings := int(*replicaSet.Spec.Replicas) - len(bindingList.Items)
		logger.Info("Creating missing NetworkFunctionBindings", "count", missingBindings)

		for range missingBindings {
			binding := r.createBindingFromReplicaSet(replicaSet)
			if err := r.Create(ctx, binding); err != nil {
				logger.Error(err, "unable to create NetworkFunctionBinding", "binding", binding)
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
			logger.Info("Created NetworkFunctionBinding", "binding", binding)
		}
		// Set in the upscaling phase
		replicaSet.Status.Phase = NetworkFunctionReplicaSetPhaseUpscaling
		// Add upscaling condition
		replicaSet.Status.Conditions = append(replicaSet.Status.Conditions,
			metav1.Condition{
				Type:               "Upscaling",
				Status:             metav1.ConditionTrue,
				Reason:             "CreatingBindings",
				Message:            "Creating missing NetworkFunctionBindings",
				LastTransitionTime: metav1.Now(),
			},
		)

		// Set last deployment time
		replicaSet.Status.CurrentReplicas = int32(len(bindingList.Items) + missingBindings)
		replicaSet.Status.LastDeploymentTime = metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, replicaSet); err != nil {
			logger.Error(err, "unable to update NetworkFunctionReplicaSet status")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Requeue to verify bindings creation
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	} else if len(bindingList.Items) > int(*replicaSet.Spec.Replicas) {
		// Remove extra bindings
		extraBindings := len(bindingList.Items) - int(*replicaSet.Spec.Replicas)
		logger.Info("Removing extra NetworkFunctionBindings", "count", extraBindings)

		for i := range extraBindings {
			binding := &bindingList.Items[i]
			if err := r.Delete(ctx, binding); err != nil {
				logger.Error(err, "unable to delete NetworkFunctionBinding", "binding", binding)
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
			logger.Info("Deleted NetworkFunctionBinding", "binding", binding)
		}

		// Set in the downscaling phase
		replicaSet.Status.Phase = NetworkFunctionReplicaSetPhaseDownscaling
		// Add downscaling condition
		replicaSet.Status.Conditions = append(replicaSet.Status.Conditions,
			metav1.Condition{
				Type:               "Downscaling",
				Status:             metav1.ConditionTrue,
				Reason:             "DeletingBindings",
				Message:            "Deleting extra NetworkFunctionBindings",
				LastTransitionTime: metav1.Now(),
			},
		)

		// Set last deployment time
		replicaSet.Status.CurrentReplicas = int32(len(bindingList.Items) - extraBindings)
		replicaSet.Status.LastDeploymentTime = metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, replicaSet); err != nil {
			logger.Error(err, "unable to update NetworkFunctionReplicaSet status")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		// Requeue to verify bindings deletion
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update status
	// Set ready replicas
	// deployment.Status.ReadyReplicas = ...
	//

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReplicaSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionReplicaSet{}).
		Owns(&schedulingv1alpha1.NetworkFunctionBinding{}).
		Named("scheduling-networkfunctionreplicaset").
		Complete(r)
}

func (r *NetworkFunctionReplicaSetReconciler) createBindingFromReplicaSet(replicaSet *schedulingv1alpha1.NetworkFunctionReplicaSet) *schedulingv1alpha1.NetworkFunctionBinding {
	binding := &schedulingv1alpha1.NetworkFunctionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      replicaSet.Name + "-" + randSeq(5),
			Namespace: replicaSet.Namespace,
		},
		Spec: schedulingv1alpha1.NetworkFunctionBindingSpec{
			SupportedTargets: replicaSet.Spec.SupportedTargets,
			P4File:           replicaSet.Spec.P4File,
		},
	}

	// Set the ownerRef for the Binding, ensuring that the Binding
	// will be deleted when the NetworkFunctionReplicaSet CR is deleted.
	controllerutil.SetControllerReference(replicaSet, binding, r.Scheme)
	return binding
}

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
