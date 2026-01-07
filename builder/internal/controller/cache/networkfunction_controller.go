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

package cache

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "builder/api/cache/v1alpha1"
	schedulingv1alpha1 "builder/api/scheduling/v1alpha1"
	"builder/pkg/events"
)

// NetworkFunctionReconciler reconciles a NetworkFunction object
type NetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Bus    events.EventBus
}

// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.desire6g.eu,resources=networkfunctions/finalizers,verbs=update

// RBAC permissions for Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NetworkFunction object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	const finalizerName = "cache.desire6g.eu/finalizer"

	logger.Info("Reconciling NetworkFunction", "request", req)
	nf := &cachev1alpha1.NetworkFunction{}
	if err := r.Get(ctx, req.NamespacedName, nf); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunction resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NetworkFunction")
		return ctrl.Result{}, err
	}

	// Check if being deleted
	if !nf.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion
		if containsString(nf.GetFinalizers(), finalizerName) {
			r.Bus.Publish(events.Event{
				Name:    events.EventNfPreDeleted,
				Payload: nf,
			})

			// Remove finalizer
			nf.SetFinalizers(removeString(nf.GetFinalizers(), finalizerName))
			if err := r.Update(ctx, nf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !containsString(nf.GetFinalizers(), finalizerName) {
		nf.SetFinalizers(append(nf.GetFinalizers(), finalizerName))
		if err := r.Update(ctx, nf); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Search for the deployment
	found := &schedulingv1alpha1.NetworkFunctionReplicaSet{}
	err := r.Get(ctx, req.NamespacedName, found)
	if err != nil && apierrors.IsNotFound(err) {
		// Deployment not found, create it
		dep := r.replicaSetForNetworkFunction(nf)

		logger.Info("Creating a new NetworkFunctionReplicaSet", "NetworkFunctionReplicaSet.Namespace", dep.Namespace, "NetworkFunctionReplicaSet.Name", dep.Name)
		if err := r.Create(ctx, dep); err != nil {
			logger.Error(err, "Failed to create new NetworkFunctionReplicaSet", "NetworkFunctionReplicaSet.Namespace", dep.Namespace, "NetworkFunctionReplicaSet.Name", dep.Name)
			return ctrl.Result{}, err
		}
		// NetworkFunctionReplicaSet created successfully - return and requeue
		// to verify deployment exists.
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Update NetworkFunction statuses
	nf.Status.CurrentReplicas = found.Status.CurrentReplicas
	nf.Status.ReadyReplicas = found.Status.ReadyReplicas
	if err := r.Status().Update(ctx, nf); err != nil {
		logger.Error(err, "Failed to update NetworkFunction status")
		return ctrl.Result{}, err
	}

	// All is well.
	// Publish creation/update event
	r.Bus.Publish(events.Event{
		Name:    events.EventNfPreUpdated,
		Payload: nf,
	})

	return ctrl.Result{}, nil
}

func (r *NetworkFunctionReconciler) replicaSetForNetworkFunction(
	nf *cachev1alpha1.NetworkFunction) *schedulingv1alpha1.NetworkFunctionReplicaSet {

	dep := &schedulingv1alpha1.NetworkFunctionReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nf.Name,
			Namespace: nf.Namespace,
		},
		Spec: schedulingv1alpha1.NetworkFunctionReplicaSetSpec{
			Replicas:         nf.Spec.Replicas,
			SupportedTargets: nf.Spec.SupportedTargets,
			P4File:           nf.Spec.P4File,
		},
	}

	// Set the ownerRef for the ReplicaSet, ensuring that the ReplicaSet
	// will be deleted when the Busybox CR is deleted.
	controllerutil.SetControllerReference(nf, dep, r.Scheme)
	return dep
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NetworkFunction{}).
		Owns(&schedulingv1alpha1.NetworkFunctionReplicaSet{}).
		Named("cache-networkfunction").
		Complete(r)
}
