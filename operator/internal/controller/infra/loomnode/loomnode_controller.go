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

package loomnode

import (
	"context"
	"fmt"
	infrav1alpha1 "loom/api/infra/v1alpha1"
	"loom/pkg/ipam"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// LoomNodeReconciler reconciles a LoomNode object
type LoomNodeReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	NodeCIDRv4Allocator *ipam.PrefixAllocator
	NodeCIDRv6Allocator *ipam.PrefixAllocator
}

// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LoomNode object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *LoomNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var loomNode = &infrav1alpha1.LoomNode{}
	if err := r.Get(ctx, req.NamespacedName, loomNode); apierrors.IsNotFound(err) {
		// Resource was deleted
		logger.V(1).Info("loomnode resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	original := loomNode.DeepCopy()
	var updatedAssignments = false
	if len(loomNode.Spec.NodeCIDRs) == 0 {
		if err := r.assignNodeCIDRs(loomNode); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to assign node CIDRs: %w", err)
		}
		updatedAssignments = true
	}

	if updatedAssignments {
		err := r.Patch(ctx, loomNode, client.MergeFrom(original))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update loomnode with assigned CIDRs: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoomNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.LoomNode{}).
		Named("infra-loomnode").
		Complete(r)
}

func (r *LoomNodeReconciler) assignNodeCIDRs(loomNode *infrav1alpha1.LoomNode) error {
	cidrs := make([]string, 0)
	if r.NodeCIDRv4Allocator != nil {
		ipv4Prefix, err := r.NodeCIDRv4Allocator.Next()
		if err != nil {
			return err
		}
		cidrs = append(cidrs, ipv4Prefix.String())
	}
	ipv6Prefix, err := r.NodeCIDRv6Allocator.Next()
	if err != nil {
		return err
	}
	cidrs = append(cidrs, ipv6Prefix.String())
	loomNode.Spec.NodeCIDRs = cidrs
	return nil
}
