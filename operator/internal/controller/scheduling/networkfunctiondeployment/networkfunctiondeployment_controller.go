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

package networkfunctiondeployment

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "github.com/mantra6g/iml/api/scheduling/v1alpha1"
	nfutil "github.com/mantra6g/iml/operator/internal/controller/scheduling/networkfunctiondeployment/util"
	stringutils "github.com/mantra6g/iml/operator/pkg/util/string"
)

// NetworkFunctionDeploymentReconciler reconciles a NetworkFunctionDeployment object
type NetworkFunctionDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctiondeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctiondeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctiondeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=scheduling.loom.io,resources=networkfunctionreplicasets,verbs=get;list;watch;create;update;patch;delete

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkFunctionDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.NetworkFunctionDeployment{}).
		Owns(&schedulingv1alpha1.NetworkFunctionReplicaSet{}).
		Named("scheduling-networkfunctiondeployment").
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *NetworkFunctionDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	logger.Info("Reconciling NetworkFunctionDeployment", "request", req)
	nfDeployment := &schedulingv1alpha1.NetworkFunctionDeployment{}
	if err := r.Get(ctx, req.NamespacedName, nfDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("NetworkFunctionDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get NetworkFunctionDeployment")
		return ctrl.Result{}, err
	}

	// Check if being deleted
	if !nfDeployment.DeletionTimestamp.IsZero() {
		// Handle deletion
		if stringutils.ContainsElement(nfDeployment.GetFinalizers(), schedulingv1alpha1.NFDeploymentFinalizer) {
			// Remove finalizer
			nfDeployment.SetFinalizers(stringutils.RemoveElement(nfDeployment.GetFinalizers(), schedulingv1alpha1.NFDeploymentFinalizer))
			if err := r.Update(ctx, nfDeployment); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing
	if !stringutils.ContainsElement(nfDeployment.GetFinalizers(), schedulingv1alpha1.NFDeploymentFinalizer) {
		nfDeployment.SetFinalizers(append(nfDeployment.GetFinalizers(), schedulingv1alpha1.NFDeploymentFinalizer))
		if err := r.Update(ctx, nfDeployment); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Search for all NetworkFunctionReplicaSets associated with this NetworkFunctionDeployment
	allRSs, err := r.listReplicaSets(ctx, nfDeployment)
	if err != nil {
		logger.Error(err, "Failed to list NetworkFunctionReplicaSets for NetworkFunctionDeployment",
			"NetworkFunctionDeployment.Namespace", nfDeployment.Namespace, "NetworkFunctionDeployment.Name", nfDeployment.Name)
		return ctrl.Result{}, err
	}

	// Sorts the replicaSets by asc creation timestamp and returns the oldest RS that matches the current hash
	// and then a slice of all the outdated RSs
	currentRS, oldRSs := r.sortAndSplitReplicaSets(ctx, nfDeployment, allRSs)

	// Make sure the current RS exists and is up to date in its metadata fields as well
	// as its replica count (if needed).
	// This is important to ensure that we have a stable current RS to work with when
	// applying scaling operations and updating the NetworkFunctionDeployment status.
	updated, err := r.ensureUpdatedReplicaSet(ctx, nfDeployment, currentRS, allRSs)
	if err != nil {
		logger.Error(err, "Failed to create/update the current NetworkFunctionReplicaSet for NetworkFunctionDeployment",
			"NetworkFunctionDeployment.Namespace", nfDeployment.Namespace, "NetworkFunctionDeployment.Name", nfDeployment.Name)
		return ctrl.Result{}, err
	}
	if updated {
		// If we had to create or update the current ReplicaSet, we should requeue to verify the state of the
		// ReplicaSets and update the NetworkFunctionDeployment status accordingly
		//setProgressingCondition(&nfDeployment.Status, "UpdatingReplicaSet", metav1.ConditionTrue, "CurrentReplicaSetUpdated")
		_ = r.updateNFDeploymentStatus(ctx, allRSs, currentRS, nfDeployment) // best-effort status update
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if scaling is needed
	needsScaling, err := nfutil.NeedsScaling(ctx, nfDeployment, currentRS, oldRSs)
	if err != nil {
		logger.Error(err, "Failed to determine if scaling is needed for NetworkFunctionDeployment",
			"NetworkFunctionDeployment.Namespace", nfDeployment.Namespace, "NetworkFunctionDeployment.Name", nfDeployment.Name)
		return ctrl.Result{}, err
	}
	if !needsScaling {
		_ = r.updateNFDeploymentStatus(ctx, allRSs, currentRS, nfDeployment) // best-effort status update
		return ctrl.Result{}, nil
	}

	switch nfutil.GetDeploymentStrategyType(nfDeployment) {
	case schedulingv1alpha1.DeploymentStrategyTypeRollingUpdate:
		err = r.applyRollingUpdate(ctx, nfDeployment, allRSs, oldRSs, currentRS)
	case schedulingv1alpha1.DeploymentStrategyTypeRecreate:
		err = r.applyRecreate(ctx, nfDeployment, allRSs, oldRSs, currentRS)
	default:
		err = fmt.Errorf("unknown deployment strategy type: %s", nfDeployment.Spec.Strategy.Type)
	}
	if err != nil {
		logger.Error(err, "Failed to apply deployment strategy for NetworkFunctionDeployment",
			"NetworkFunctionDeployment.Namespace", nfDeployment.Namespace, "NetworkFunctionDeployment.Name", nfDeployment.Name)
		_ = r.updateNFDeploymentStatus(ctx, allRSs, currentRS, nfDeployment) // best-effort status update
		return ctrl.Result{}, err
	}

	// Update NetworkFunctionDeployment statuses
	err = r.updateNFDeploymentStatus(ctx, allRSs, currentRS, nfDeployment)
	if err != nil {
		logger.Error(err, "Failed to update NetworkFunctionDeployment status",
			"NetworkFunctionDeployment.Namespace", nfDeployment.Namespace, "NetworkFunctionDeployment.Name", nfDeployment.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
