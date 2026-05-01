// Most of this code comes from the Kubernetes repo at https://github.com/kubernetes/kubernetes,
// with some minor reworks to be able to work with network functions.
/*
Copyright 2025 The Kubernetes Authors

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

package nfgc

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/mantra6g/iml/api/core/v1alpha1"
	nfutil "github.com/mantra6g/iml/operator/pkg/util/nf"
	p4targetutil "github.com/mantra6g/iml/operator/pkg/util/p4target"
	"github.com/mantra6g/iml/operator/pkg/util/taints"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterWideReconciler reconciles network functions that have failed or are stuck in an invalid state.
type ClusterWideReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	Log                   logr.Logger
	TerminatedNFThreshold int
	QuarantineTime        time.Duration
	targetQueue           workqueue.TypedDelayingInterface[string]
}

const (
	ReconcileInterval = 20 * time.Second
)

// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.loom.io,resources=networkfunctions/finalizers,verbs=update

func (r *ClusterWideReconciler) listAllNetworkFunctions(ctx context.Context) ([]*v1alpha1.NetworkFunction, error) {
	var nfList = &v1alpha1.NetworkFunctionList{}
	if err := r.List(ctx, nfList); err != nil {
		return nil, err
	}
	var allNfs = make([]*v1alpha1.NetworkFunction, len(nfList.Items))
	for i := range nfList.Items {
		allNfs[i] = &nfList.Items[i]
	}
	return allNfs, nil
}

func (r *ClusterWideReconciler) listAllP4Targets(ctx context.Context) ([]*v1alpha1.P4Target, error) {
	var p4TargetList = &v1alpha1.P4TargetList{}
	if err := r.List(ctx, p4TargetList); err != nil {
		return nil, err
	}
	var allP4Targets = make([]*v1alpha1.P4Target, len(p4TargetList.Items))
	for i := range p4TargetList.Items {
		allP4Targets[i] = &p4TargetList.Items[i]
	}
	return allP4Targets, nil
}

func isNFTerminated(nf *v1alpha1.NetworkFunction) bool {
	if phase := nf.Status.Phase; phase != v1alpha1.NetworkFunctionPending &&
		phase != v1alpha1.NetworkFunctionRunning {
		return true
	}
	return false
}

// isNFTerminating returns true if the nf is terminating.
func isNFTerminating(nf *v1alpha1.NetworkFunction) bool {
	return nf.ObjectMeta.DeletionTimestamp != nil
}

func (r *ClusterWideReconciler) markFailedAndDeleteNF(ctx context.Context, nf *v1alpha1.NetworkFunction) error {
	return r.markFailedAndDeleteNFWithCondition(ctx, nf, nil)
}

func (r *ClusterWideReconciler) markFailedAndDeleteNFWithCondition(ctx context.Context, nf *v1alpha1.NetworkFunction, condition *v1alpha1.NetworkFunctionCondition) error {
	logger := klog.FromContext(ctx)
	logger.Info("NetworkFunctionGC is force deleting NF", "nf", klog.KObj(nf))
	// Patch the nf to make sure it is transitioned to the Failed phase before deletion.
	//
	// Mark the nf as failed - this is especially important in case the nf
	// is orphaned, in which case the nf would remain in the Running phase
	// forever as there is no kubelet running to change the phase.
	if nf.Status.Phase != v1alpha1.NetworkFunctionFailed {
		original := nf.DeepCopy()
		nf.Status.Phase = v1alpha1.NetworkFunctionFailed
		nf.Status.ObservedGeneration = nf.Generation
		if condition != nil {
			nfutil.UpdateNFCondition(&nf.Status, condition)
		}
		if err := r.Status().Patch(ctx, nf, client.MergeFrom(original)); err != nil {
			return err
		}
	}
	return r.Delete(ctx, nf)
}

func (r *ClusterWideReconciler) checkIfTargetExists(ctx context.Context, name string) (bool, error) {
	fetchErr := r.Get(ctx, types.NamespacedName{Name: name}, &v1alpha1.P4Target{})
	if errors.IsNotFound(fetchErr) {
		return false, nil
	}
	return fetchErr == nil, fetchErr
}

func (r *ClusterWideReconciler) discoverDeletedTargets(ctx context.Context, existingTargetNames sets.Set[string]) (sets.Set[string], bool) {
	deletedTargetNames := sets.New[string]()
	for r.targetQueue.Len() > 0 {
		item, quit := r.targetQueue.Get()
		if quit {
			return nil, true
		}
		nodeName := item
		if !existingTargetNames.Has(nodeName) {
			exists, err := r.checkIfTargetExists(ctx, nodeName)
			switch {
			case err != nil:
				klog.FromContext(ctx).Error(err, "Error while getting target", "target", nodeName)
				// Node will be added back to the queue in the subsequent loop if still needed
			case !exists:
				deletedTargetNames.Insert(nodeName)
			}
		}
		r.targetQueue.Done(item)
	}
	return deletedTargetNames, false
}

func (r *ClusterWideReconciler) gcTerminated(ctx context.Context, nfs []*v1alpha1.NetworkFunction) {
	terminatedNfs := make([]*v1alpha1.NetworkFunction, 0)
	for _, nf := range nfs {
		if isNFTerminated(nf) {
			terminatedNfs = append(terminatedNfs, nf)
		}
	}

	terminatedNFCount := len(terminatedNfs)
	deleteCount := terminatedNFCount - r.TerminatedNFThreshold

	if deleteCount <= 0 {
		return
	}

	logger := klog.FromContext(ctx)
	logger.Info("Garbage collecting nfs", "numNFs", deleteCount)
	// sort only when necessary
	sort.Sort(byEvictionAndCreationTimestamp(terminatedNfs))
	var waitGrp sync.WaitGroup
	for i := 0; i < deleteCount; i++ {
		waitGrp.Add(1)
		go func(nf *v1alpha1.NetworkFunction) {
			defer waitGrp.Done()
			if err := r.markFailedAndDeleteNF(ctx, nf); err != nil {
				// ignore not founds
				defer utilruntime.HandleErrorWithContext(ctx, err, "Failed to delete terminated nf", "nf", klog.KObj(nf))
			}
		}(terminatedNfs[i])
	}
	waitGrp.Wait()
}

func (r *ClusterWideReconciler) gcTerminating(ctx context.Context, nfs []*v1alpha1.NetworkFunction) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("GC'ing terminating nfs that are on out-of-service nodes")
	terminatingNFs := make([]*v1alpha1.NetworkFunction, 0)
	for _, nf := range nfs {
		if isNFTerminating(nf) {
			target := &v1alpha1.P4Target{}
			err := r.Get(ctx, types.NamespacedName{Name: nf.Spec.TargetName}, target)
			if err != nil {
				logger.Error(err, "Failed to get target", "target", nf.Spec.TargetName)
				continue
			}
			// Add this nf to terminatingNFs list only if the following conditions are met:
			// 1. Target is not ready.
			// 2. Target has `target.loom.io/out-of-service` taint.
			if !p4targetutil.IsTargetReady(target) && taints.TaintKeyExists(target.Spec.Taints, v1alpha1.TaintP4TargetOutOfService) {
				logger.V(4).Info("Garbage collecting nf that is terminating", "nf", klog.KObj(nf), "phase", nf.Status.Phase)
				terminatingNFs = append(terminatingNFs, nf)
			}
		}
	}

	deleteCount := len(terminatingNFs)
	if deleteCount == 0 {
		return
	}

	logger.V(4).Info("Garbage collecting nfs that are terminating on node tainted with node.kubernetes.io/out-of-service", "numNFs", deleteCount)
	// sort only when necessary
	sort.Sort(byEvictionAndCreationTimestamp(terminatingNFs))
	var waitGrp sync.WaitGroup
	for i := 0; i < deleteCount; i++ {
		waitGrp.Add(1)
		go func(nf *v1alpha1.NetworkFunction) {
			defer waitGrp.Done()
			if err := r.markFailedAndDeleteNF(ctx, nf); err != nil {
				// ignore not founds
				utilruntime.HandleErrorWithContext(ctx, err, "Failed to delete terminating nf on out-of-service node", "nf", klog.KObj(nf))
			}
		}(terminatingNFs[i])
	}
	waitGrp.Wait()
}

// gcOrphaned deletes nfs that are bound to targets that don't exis t.
func (r *ClusterWideReconciler) gcOrphaned(ctx context.Context, nfs []*v1alpha1.NetworkFunction, targets []*v1alpha1.P4Target) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("GC'ing orphaned")
	existingNodeNames := sets.New[string]()
	for _, node := range targets {
		existingNodeNames.Insert(node.Name)
	}
	// Add newly found unknown targets to quarantine
	for _, nf := range nfs {
		if nf.Spec.TargetName != "" && !existingNodeNames.Has(nf.Spec.TargetName) {
			r.targetQueue.AddAfter(nf.Spec.TargetName, r.QuarantineTime)
		}
	}
	// Check if targets are still missing after quarantine period
	deletedTargetNames, quit := r.discoverDeletedTargets(ctx, existingNodeNames)
	if quit {
		return
	}
	// Delete orphaned nfs
	for _, nf := range nfs {
		if !deletedTargetNames.Has(nf.Spec.TargetName) {
			continue
		}
		logger.V(2).Info("Found orphaned nf assigned to the target, deleting", "nf", klog.KObj(nf), "target", nf.Spec.TargetName)
		condition := &v1alpha1.NetworkFunctionCondition{
			Type:    v1alpha1.DisruptionTarget,
			Status:  metav1.ConditionTrue,
			Reason:  "DeletionByNetworkFunctionGC",
			Message: "NFGC: target no longer exists",
		}
		if err := r.markFailedAndDeleteNFWithCondition(ctx, nf, condition); err != nil {
			utilruntime.HandleErrorWithContext(ctx, err, "Failed to delete orphaned nf", "nf", klog.KObj(nf))
		} else {
			logger.Info("Forced deletion of orphaned nf succeeded", "nf", klog.KObj(nf))
		}
	}
}

// gcUnscheduledTerminating deletes nfs that are terminating and haven't been scheduled to a particular target.
func (r *ClusterWideReconciler) gcUnscheduledTerminating(ctx context.Context, nfs []*v1alpha1.NetworkFunction) {
	logger := klog.FromContext(ctx)
	logger.V(4).Info("GC'ing unscheduled nfs which are terminating")

	for _, nf := range nfs {
		if nf.DeletionTimestamp == nil || len(nf.Spec.TargetName) > 0 {
			continue
		}

		logger.V(2).Info("Found unscheduled terminating nf not assigned to any target, deleting", "nf", klog.KObj(nf))
		if err := r.markFailedAndDeleteNF(ctx, nf); err != nil {
			utilruntime.HandleErrorWithContext(ctx, err, "Failed to delete unscheduled terminating nf", "nf", klog.KObj(nf))
		} else {
			logger.Info("Forced deletion of unscheduled terminating nf succeeded", "nf", klog.KObj(nf))
		}
	}
}

// Start initializes the controller and starts the periodic reconciliation loop.
func (r *ClusterWideReconciler) Start(ctx context.Context) error {
	r.targetQueue = workqueue.NewTypedDelayingQueueWithConfig(
		workqueue.TypedDelayingQueueConfig[string]{Name: "orphaned_nf_targets"})
	wait.UntilWithContext(ctx, r.Reconcile, ReconcileInterval)
	return nil
}

// Reconcile lists all NetworkFunctions in the cluster and takes care of those that are in a failed state or
// have been stuck in an invalid state for too long.
func (r *ClusterWideReconciler) Reconcile(ctx context.Context) {
	logger := r.Log
	logger.V(1).Info("Starting network function garbage collection reconciliation")

	allNfs, err := r.listAllNetworkFunctions(ctx)
	if err != nil {
		logger.Error(err, "Failed to list NetworkFunctions")
		return
	}
	allTargets, err := r.listAllP4Targets(ctx)
	if err != nil {
		logger.Error(err, "Failed to list P4Targets")
		return
	}
	if r.TerminatedNFThreshold > 0 {
		r.gcTerminated(ctx, allNfs)
	}
	r.gcTerminating(ctx, allNfs)
	r.gcOrphaned(ctx, allNfs, allTargets)
	r.gcUnscheduledTerminating(ctx, allNfs)
}
