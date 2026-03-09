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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "loom/api/cache/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	nfutil "loom/internal/controller/cache/networkfunction/util"
	stringutils "loom/pkg/util/string"
)

// NetworkFunctionReconciler reconciles a NetworkFunction object
type NetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.loom.io,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.loom.io,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.loom.io,resources=networkfunctions/finalizers,verbs=update
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionreplicasets,verbs=get;list;watch;create;update;patch;delete

// RBAC permissions for Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.NetworkFunction{}).
		Owns(&schedulingv1alpha1.NetworkFunctionReplicaSet{}).
		Named("cache-networkfunction").
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

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
	if !nf.DeletionTimestamp.IsZero() {
		// Handle deletion
		if stringutils.ContainsElement(nf.GetFinalizers(), cachev1alpha1.NETWORK_FUNCTION_FINALIZER_LABEL) {
			// Remove finalizer
			nf.SetFinalizers(stringutils.RemoveElement(nf.GetFinalizers(), cachev1alpha1.NETWORK_FUNCTION_FINALIZER_LABEL))
			if err := r.Update(ctx, nf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !stringutils.ContainsElement(nf.GetFinalizers(), cachev1alpha1.NETWORK_FUNCTION_FINALIZER_LABEL) {
		nf.SetFinalizers(append(nf.GetFinalizers(), cachev1alpha1.NETWORK_FUNCTION_FINALIZER_LABEL))
		if err := r.Update(ctx, nf); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Search for all NetworkFunctionReplicaSets associated with this NetworkFunction
	allRSs, err := r.listReplicaSets(ctx, nf)
	if err != nil {
		logger.Error(err, "Failed to list NetworkFunctionReplicaSets for NetworkFunction",
			"NetworkFunction.Namespace", nf.Namespace, "NetworkFunction.Name", nf.Name)
		return ctrl.Result{}, err
	}

	// Sorts the replicaSets by asc creation timestamp and returns the oldest RS that matches the current hash
	// and then a slice of all the outdated RSs
	currentRS, oldRSs := r.sortAndSplitReplicaSets(ctx, nf, allRSs)

	// Make sure the current RS exists and is up to date in its metadata fields as well
	// as its replica count (if needed).
	// This is important to ensure that we have a stable current RS to work with when
	// applying scaling operations and updating the NetworkFunction status.
	updated, err := r.ensureUpdatedReplicaSet(ctx, nf, currentRS, allRSs)
	if err != nil {
		logger.Error(err, "Failed to create/update the current NetworkFunctionReplicaSet for NetworkFunction",
			"NetworkFunction.Namespace", nf.Namespace, "NetworkFunction.Name", nf.Name)
		return ctrl.Result{}, err
	}
	if updated {
		// If we had to create or update the current ReplicaSet, we should requeue to verify the state of the
		// ReplicaSets and update the NetworkFunction status accordingly
		//setProgressingCondition(&nf.Status, "UpdatingReplicaSet", metav1.ConditionTrue, "CurrentReplicaSetUpdated")
		_ = r.updateNFStatus(ctx, allRSs, currentRS, nf) // best-effort status update
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if scaling is needed
	needsScaling, err := nfutil.NeedsScaling(ctx, nf, currentRS, oldRSs)
	if err != nil {
		logger.Error(err, "Failed to determine if scaling is needed for NetworkFunction",
			"NetworkFunction.Namespace", nf.Namespace, "NetworkFunction.Name", nf.Name)
		return ctrl.Result{}, err
	}
	if !needsScaling {
		_ = r.updateNFStatus(ctx, allRSs, currentRS, nf) // best-effort status update
		return ctrl.Result{}, nil
	}

	switch nf.Spec.Strategy.Type {
	case cachev1alpha1.DeploymentStrategyTypeRollingUpdate:
		err = r.applyRollingUpdate(ctx, nf, allRSs, oldRSs, currentRS)
	case cachev1alpha1.DeploymentStrategyTypeRecreate:
		err = r.applyRecreate(ctx, nf, allRSs, oldRSs, currentRS)
	default:
		err = fmt.Errorf("unknown deployment strategy type: %s", nf.Spec.Strategy.Type)
	}
	if err != nil {
		logger.Error(err, "Failed to apply deployment strategy for NetworkFunction",
			"NetworkFunction.Namespace", nf.Namespace, "NetworkFunction.Name", nf.Name)
		_ = r.updateNFStatus(ctx, allRSs, currentRS, nf) // best-effort status update
		return ctrl.Result{}, err
	}

	// Update NetworkFunction statuses
	err = r.updateNFStatus(ctx, allRSs, currentRS, nf)
	if err != nil {
		logger.Error(err, "Failed to update NetworkFunction status",
			"NetworkFunction.Namespace", nf.Namespace, "NetworkFunction.Name", nf.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
