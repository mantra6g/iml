package nf_replicaset

import (
	"context"
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	rsutil "loom/internal/controller/scheduling/nf_replicaset/util"
)

const (
	// BurstReplicas defines the maximum number of nf replicas that can be created or deleted
	// in a single reconciliation loop.
	BurstReplicas = 500

	// SlowStartInitialBatchSize is the size of the initial batch when batching pod creates.
	// The size of each successive batch is twice the size of
	// the previous batch.  For example, for a value of 1, batch sizes would be
	// 1, 2, 4, 8, ...  and for a value of 10, batch sizes would be
	// 10, 20, 40, 80, ...  Setting the value higher means that quota denials
	// will result in more doomed API calls and associated event spam.  Setting
	// the value lower will result in more API call round trip periods for
	// large batches.
	//
	// Given a number of pods to start "N":
	// The number of doomed calls per sync once quota is exceeded is given by:
	//      min(N,SlowStartInitialBatchSize)
	// The number of batches is given by:
	//      1+floor(log_2(ceil(N/SlowStartInitialBatchSize)))
	SlowStartInitialBatchSize = 1
)

// controllerUIDIndex is the index of ReplicaSets by their controller's UID.
// It is used to quickly find all ReplicaSets that are owned by the same controller,
// which is necessary when managing expectations for deletions.
var controllerUIDIndex = "controller-uid-index"

// Reasons for binding events
const (
	// FailedCreateBindingReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be created.
	FailedCreateBindingReason = "FailedCreate"
	// SuccessfulCreateBindingReason is added in an event when a pod for a replica set
	// is successfully created.
	SuccessfulCreateBindingReason = "SuccessfulCreate"
	// FailedDeleteBindingReason is added in an event and in a replica set condition
	// when a pod for a replica set is failed to be deleted.
	FailedDeleteBindingReason = "FailedDelete"
	// SuccessfulDeleteBindingReason is added in an event when a pod for a replica set
	// is successfully deleted.
	SuccessfulDeleteBindingReason = "SuccessfulDelete"
)

func (r *NetworkFunctionReplicaSetReconciler) listActiveBindings(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
) ([]*schedulingv1alpha1.NetworkFunctionBinding, error) {
	// Convert the NetworkFunctionReplicaSet's label selector to a selector that can be used to list
	// NetworkFunctionBindings
	bindingSelector, err := metav1.LabelSelectorAsSelector(rs.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("nf replicaset %s/%s has invalid label selector: %v",
			rs.Namespace, rs.Name, err)
	}

	var bindingList schedulingv1alpha1.NetworkFunctionBindingList
	if err = r.List(ctx, &bindingList,
		client.MatchingLabelsSelector{Selector: bindingSelector}); err != nil {
		return nil, err
	}

	allBindings := make([]*schedulingv1alpha1.NetworkFunctionBinding, 0, len(bindingList.Items))
	for i := range bindingList.Items {
		allBindings = append(allBindings, &bindingList.Items[i])
	}

	return rsutil.FilterActiveBindings(allBindings), nil
}

// manageReplicas checks and updates replicas for the given ReplicaSet.
// Does NOT modify <activeBindings>.
// It will requeue the replica set in case of an error while creating/deleting bindings.
func (r *NetworkFunctionReplicaSetReconciler) manageReplicas(
	ctx context.Context,
	activeBindings []*schedulingv1alpha1.NetworkFunctionBinding,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
) error {
	diff := len(activeBindings) - int(*(rs.Spec.Replicas))
	rsKey, err := rsutil.KeyFunc(rs)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for nf replicaset %#v: %v", rs, err))
		return nil
	}
	logger := logf.FromContext(ctx)
	if diff < 0 {
		diff *= -1
		if diff > BurstReplicas {
			diff = BurstReplicas
		}
		// TODO: Track UIDs of creates just like deletes. The problem currently
		//   is we'd need to wait on the result of a create to record the pod's
		//   UID, which would require locking *across* the create, which will turn
		//   into a performance bottleneck. We should generate a UID for the pod
		//   beforehand and store it via ExpectCreations.
		r.expectations.ExpectCreations(logger, rsKey, diff)
		logger.V(2).Info("Too few replicas",
			"replicaSet", rs, "need", *(rs.Spec.Replicas), "creating", diff)
		// Batch the pod creates. Batch sizes start at SlowStartInitialBatchSize
		// and double with each successful iteration in a kind of "slow start".
		// This handles attempts to start large numbers of pods that would
		// likely all fail with the same error. For example a project with a
		// low quota that attempts to create a large number of pods will be
		// prevented from spamming the API service with the pod create requests
		// after one of its pods fails.  Conveniently, this also prevents the
		// event spam that those failures would generate.
		successfulCreations, err := slowStartBatch(diff, SlowStartInitialBatchSize, func() error {
			err := r.CreateBindings(ctx, rs)
			if err != nil {
				if apierrors.HasStatusCause(err, v1.NamespaceTerminatingCause) {
					// if the namespace is being terminated, we don't have to do
					// anything because any creation will fail
					return nil
				}
			}
			return err
		})

		// Any skipped pods that we never attempted to start shouldn't be expected.
		// The skipped pods will be retried later. The next controller resync will
		// retry the slow start process.
		if skippedPods := diff - successfulCreations; skippedPods > 0 {
			logger.V(2).Info("Slow-start failure. Skipping creation of pods, decrementing expectations",
				"podsSkipped", skippedPods, "replicaSet", rs)
			for i := 0; i < skippedPods; i++ {
				// Decrement the expected number of creates because the informer won't observe this pod
				r.expectations.CreationObserved(logger, rsKey)
			}
		}
		return err
	} else if diff > 0 {
		if diff > BurstReplicas {
			diff = BurstReplicas
		}
		logger.V(2).Info("Too many replicas",
			"replicaSet", rs, "need", *(rs.Spec.Replicas), "deleting", diff)

		relatedPods, err := r.getIndirectlyRelatedBindings(ctx, rs)
		utilruntime.HandleError(err)

		// Choose which Pods to delete, preferring those in earlier phases of startup.
		bindingsToDelete := rsutil.GetBindingsToDelete(activeBindings, relatedPods, diff)

		// Snapshot the UIDs (ns/name) of the pods we're expecting to see
		// deleted, so we know to record their expectations exactly once either
		// when we see it as an update of the deletion timestamp, or as a delete.
		// Note that if the labels on a b/rs change in a way that the b gets
		// orphaned, the rs will only wake up after the expectations have
		// expired even if other pods are deleted.
		r.expectations.ExpectDeletions(logger, rsKey, rsutil.GetBindingKeys(bindingsToDelete))

		errCh := make(chan error, diff)
		var wg sync.WaitGroup
		wg.Add(diff)
		for _, b := range bindingsToDelete {
			go func(targetBinding *schedulingv1alpha1.NetworkFunctionBinding) {
				defer wg.Done()
				if err := r.DeleteBinding(ctx, rs.Namespace, targetBinding.Name); err != nil {
					// Decrement the expected number of deletes because the informer won't observe this deletion
					bindingKey := rsutil.BindingKey(targetBinding)
					r.expectations.DeletionObserved(logger, rsKey, bindingKey)
					if !apierrors.IsNotFound(err) {
						logger.V(2).Info("Failed to delete binding, decremented expectations",
							"b", bindingKey, "replicaSet", rs)
						errCh <- err
					}
				}
			}(b)
		}
		wg.Wait()

		select {
		case err := <-errCh:
			// all errors have been reported before and they're likely to be the same,
			// so we'll only return the first one we hit.
			if err != nil {
				return err
			}
		default:
		}
	}

	return nil
}

// slowStartBatch tries to call the provided function a total of 'count' times,
// starting slow to check for errors, then speeding up if calls succeed.
//
// It groups the calls into batches, starting with a group of initialBatchSize.
// Within each batch, it may call the function multiple times concurrently.
//
// If a whole batch succeeds, the next batch may get exponentially larger.
// If there are any failures in a batch, all remaining batches are skipped
// after waiting for the current batch to complete.
//
// It returns the number of successful calls to the function.
func slowStartBatch(count int, initialBatchSize int, fn func() error) (int, error) {
	remaining := count
	successes := 0
	for batchSize := min(remaining, initialBatchSize); batchSize > 0; batchSize = min(2*batchSize, remaining) {
		errCh := make(chan error, batchSize)
		var wg sync.WaitGroup
		wg.Add(batchSize)
		for i := 0; i < batchSize; i++ {
			go func() {
				defer wg.Done()
				if err := fn(); err != nil {
					errCh <- err
				}
			}()
		}
		wg.Wait()
		curSuccesses := batchSize - len(errCh)
		successes += curSuccesses
		if len(errCh) > 0 {
			return successes, <-errCh
		}
		remaining -= batchSize
	}
	return successes, nil
}

func (r *NetworkFunctionReplicaSetReconciler) CreateBindings(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
) error {
	return r.CreateBindingWithGenerateName(ctx, rs, "")
}

func (r *NetworkFunctionReplicaSetReconciler) CreateBindingWithGenerateName(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet, generateName string,
) error {
	binding, err := r.GetBindingFromRS(rs)
	if err != nil {
		return err
	}
	if len(generateName) > 0 {
		binding.ObjectMeta.GenerateName = generateName
	}
	return r.createBindings(ctx, binding)
}
func (r *NetworkFunctionReplicaSetReconciler) GetBindingFromRS(
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
) (*schedulingv1alpha1.NetworkFunctionBinding, error) {
	desiredLabels := rsutil.GetBindingLabelSet(&rs.Spec.Template)
	desiredFinalizers := rsutil.GetBindingFinalizers(&rs.Spec.Template)
	desiredAnnotations := rsutil.GetBindingAnnotationSet(&rs.Spec.Template)
	accessor, err := meta.Accessor(rs)
	if err != nil {
		return nil, fmt.Errorf("nf replicaset does not have ObjectMeta, %v", err)
	}
	prefix := rsutil.GetBindingPrefix(accessor.GetName())

	binding := &schedulingv1alpha1.NetworkFunctionBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       desiredLabels,
			Annotations:  desiredAnnotations,
			GenerateName: prefix,
			Finalizers:   desiredFinalizers,
		},
	}
	err = controllerutil.SetOwnerReference(rs, binding, r.Scheme)
	if err != nil {
		// skip the deep copy of the spec since we won't be creating the binding if we can't set the owner reference
		return nil, fmt.Errorf("failed to set owner reference on binding: %v", err)
	}
	binding.Spec = *rs.Spec.Template.Spec.DeepCopy()
	return binding, nil
}

func (r *NetworkFunctionReplicaSetReconciler) createBindings(ctx context.Context,
	pod *schedulingv1alpha1.NetworkFunctionBinding) error {
	if len(labels.Set(pod.Labels)) == 0 {
		return fmt.Errorf("unable to create binding, no labels")
	}
	err := r.Create(ctx, pod)
	if err != nil {
		return err
	}
	logger := logf.FromContext(ctx)
	logger.V(4).Info("Controller created binding",
		"binding", pod)
	return nil
}

func (r *NetworkFunctionReplicaSetReconciler) DeleteBinding(ctx context.Context, namespace string, podID string) error {
	logger := klog.FromContext(ctx)
	logger.V(2).Info("Deleting binding",
		"binding.Name", podID, "binding.Namespace", namespace)
	binding := &schedulingv1alpha1.NetworkFunctionBinding{ObjectMeta: metav1.ObjectMeta{
		Name:      podID,
		Namespace: namespace,
	}}
	if err := r.Delete(ctx, binding); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(4).Info("NF Binding has already been deleted.",
				"binding.Name", podID, "binding.Namespace", namespace)
			return err
		}
		//r.Recorder.Eventf(object, v1.EventTypeWarning, FailedDeleteBindingReason, "Error deleting: %v", err)
		return fmt.Errorf("unable to delete pods: %v", err)
	}
	//r.Recorder.Eventf(object, v1.EventTypeNormal, SuccessfulDeleteBindingReason, "Deleted pod: %v", podID)

	return nil
}

// getIndirectlyRelatedPods returns all pods that are owned by any ReplicaSet
// that is owned by the given ReplicaSet's owner.
func (r *NetworkFunctionReplicaSetReconciler) getIndirectlyRelatedBindings(
	ctx context.Context, rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
) ([]*schedulingv1alpha1.NetworkFunctionBinding, error) {
	logger := logf.FromContext(ctx)
	var relatedBindings []*schedulingv1alpha1.NetworkFunctionBinding
	seen := make(map[types.UID]*schedulingv1alpha1.NetworkFunctionReplicaSet)
	for _, relatedRS := range r.getReplicaSetsWithSameController(ctx, rs) {
		selector, err := metav1.LabelSelectorAsSelector(relatedRS.Spec.Selector)
		if err != nil {
			// This object has an invalid selector, it does not match any pods
			continue
		}
		bindingList := &schedulingv1alpha1.NetworkFunctionBindingList{}
		err = r.List(ctx, bindingList, client.MatchingLabelsSelector{Selector: selector})
		if err != nil {
			return nil, err
		}
		for _, b := range bindingList.Items {
			if otherRS, found := seen[b.UID]; found {
				logger.V(5).Info("Binding is owned by both",
					"binding", b, "replicaSets", klog.KObjSlice([]klog.KMetadata{otherRS, relatedRS}))
				continue
			}
			seen[b.UID] = relatedRS
			relatedBindings = append(relatedBindings, &b)
		}
	}
	logger.V(4).Info("Found related bindings",
		"replicaSet", rs, "bindings", relatedBindings)
	return relatedBindings, nil
}

// getReplicaSetsWithSameController returns a list of ReplicaSets with the same
// owner as the given ReplicaSet.
func (r *NetworkFunctionReplicaSetReconciler) getReplicaSetsWithSameController(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet) []*schedulingv1alpha1.NetworkFunctionReplicaSet {
	controllerRef := metav1.GetControllerOf(rs)
	if controllerRef == nil {
		utilruntime.HandleError(fmt.Errorf("ReplicaSet has no controller: %v", rs))
		return nil
	}

	relatedRSList := &schedulingv1alpha1.NetworkFunctionReplicaSetList{}
	err := r.List(ctx, relatedRSList,
		client.InNamespace(rs.Namespace), client.MatchingFields{controllerUIDIndex: string(controllerRef.UID)})
	if err != nil {
		utilruntime.HandleError(err)
		return nil
	}
	relatedRSs := make([]*schedulingv1alpha1.NetworkFunctionReplicaSet, 0, len(relatedRSList.Items))
	for i := range len(relatedRSList.Items) {
		relatedRSs = append(relatedRSs, &relatedRSList.Items[i])
	}

	return relatedRSs
}

func (r *NetworkFunctionReplicaSetReconciler) filterOwnedPods(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet, allActiveBindings []*schedulingv1alpha1.NetworkFunctionBinding,
) ([]*schedulingv1alpha1.NetworkFunctionBinding, error) {
	ownedBindings := make([]*schedulingv1alpha1.NetworkFunctionBinding, 0, len(allActiveBindings))
	for i := range allActiveBindings {
		b := allActiveBindings[i]
		if metav1.IsControlledBy(b, rs) {
			ownedBindings = append(ownedBindings, b)
		}
	}
	return ownedBindings, nil
}
