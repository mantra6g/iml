package networkfunctiondeployment

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	schedulingv1alpha1 "github.com/mantra6g/iml/operator/api/scheduling/v1alpha1"
	deploymentutil "github.com/mantra6g/iml/operator/internal/controller/scheduling/networkfunctiondeployment/util"
	"github.com/mantra6g/iml/operator/pkg/util/ptr"
)

func (r *NetworkFunctionDeploymentReconciler) sortAndSplitReplicaSets(ctx context.Context,
	nfDeployment *schedulingv1alpha1.NetworkFunctionDeployment, replicaSets []*schedulingv1alpha1.NetworkFunctionReplicaSet) (
	current *schedulingv1alpha1.NetworkFunctionReplicaSet, old []*schedulingv1alpha1.NetworkFunctionReplicaSet) {
	old = make([]*schedulingv1alpha1.NetworkFunctionReplicaSet, 0)
	// Calculate the hash of the current nfDeployment spec
	currentSpecHash := deploymentutil.ComputeSpecHash(nfDeployment)
	// Sort the replica sets by creation timestamp,
	sort.Sort(deploymentutil.ReplicaSetsByCreationTimestamp(replicaSets))
	// Get the new replicaSet (if it exists)
	for _, rs := range replicaSets {
		if rs != nil && rs.Labels[schedulingv1alpha1.NFSpecHashLabel] == currentSpecHash {
			current = rs
			break
		}
	}
	// Iterate through the replica sets and separate them into current and old based on the hash
	for i, rs := range replicaSets {
		if current != nil && rs != nil && rs.UID == current.UID {
			continue
		}
		old = append(old, replicaSets[i])
	}
	return current, old
}

func (r *NetworkFunctionDeploymentReconciler) listReplicaSets(ctx context.Context,
	nfDeployment *schedulingv1alpha1.NetworkFunctionDeployment) ([]*schedulingv1alpha1.NetworkFunctionReplicaSet, error) {
	replicaSetList := &schedulingv1alpha1.NetworkFunctionReplicaSetList{}
	nfReplicaSetSelector, err := metav1.LabelSelectorAsSelector(nfDeployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("network function %s/%s has invalid label selector: %v",
			nfDeployment.Namespace, nfDeployment.Name, err)
	}
	err = r.List(ctx, replicaSetList,
		client.InNamespace(nfDeployment.Namespace),
		client.MatchingLabelsSelector{Selector: nfReplicaSetSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list NetworkFunctionReplicaSets for NetworkFunctionDeployment %s/%s: %v",
			nfDeployment.Namespace, nfDeployment.Name, err)
	}
	replicaList := make([]*schedulingv1alpha1.NetworkFunctionReplicaSet, 0, len(replicaSetList.Items))
	for i := range replicaSetList.Items {
		replicaList = append(replicaList, &replicaSetList.Items[i])
	}
	return replicaList, nil
	//desiredReplicas := *nfDeployment.Spec.Replicas // Desired number of replicas from the NetworkFunctionDeployment spec
	//updatedReplicas := int32(0)          // Number of replicas that are up to date with the new NetworkFunctionDeployment spec
	//totalReplicas := int32(0)            // Total number of replicas across all ReplicaSets associated with this NF Deployment
	//availableReplicas := int32(0)        // Number of replicas that are currently ready across all ReplicaSets
	//unavailableReplicas := int32(0)      // Number of replicas that are currently not ready across all ReplicaSets
	//for _, replicaSet := range replicaSetList.Items {
	//  specHash := replicaSet.Labels[cachev1alpha1.NFSpecHashLabel]
	//  totalReplicas += replicaSet.Status.CurrentReplicas
	//  availableReplicas += replicaSet.Status.ReadyReplicas
	//  unavailableReplicas += replicaSet.Status.CurrentReplicas - replicaSet.Status.ReadyReplicas
	//  if specHash == updatedSpecHash { // Is the replica set up to date with the current NetworkFunctionDeployment spec?
	//    updatedReplicas += replicaSet.Status.ReadyReplicas
	//  }
	//}
	//return &ReplicaState{desiredReplicas, updatedReplicas, totalReplicas,
	//  availableReplicas, unavailableReplicas}, nil
}

func (r *NetworkFunctionDeploymentReconciler) ensureUpdatedReplicaSet(ctx context.Context,
	nfDeployment *schedulingv1alpha1.NetworkFunctionDeployment, existingNewRS *schedulingv1alpha1.NetworkFunctionReplicaSet,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet) (updated bool, err error) {
	logger := logf.FromContext(ctx)
	// Get the max revision number
	maxOldRevision := deploymentutil.MaxRevision(allRSs)
	newRevision := strconv.FormatInt(maxOldRevision, 10)

	if existingNewRS != nil {
		annotationsUpdated := deploymentutil.SetNewReplicaSetAnnotations(ctx, nfDeployment, existingNewRS, newRevision, true)
		if annotationsUpdated {
			if err = r.Update(ctx, existingNewRS); err != nil {
				return false, fmt.Errorf("failed to update NetworkFunctionReplicaSet %s/%s: %v",
					existingNewRS.Namespace, existingNewRS.Name, err)
			}
			return true, nil
		}

		// Should use the revision in existingNewRS's annotation, since it set by before
		needsUpdate := deploymentutil.SetNFDeploymentRevision(nfDeployment, existingNewRS.Annotations[deploymentutil.RevisionAnnotation])
		if needsUpdate {
			if err = r.Status().Update(ctx, nfDeployment); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	// new ReplicaSet does not exist, create one.
	updatedSpecHash := deploymentutil.ComputeSpecHash(nfDeployment)
	newRS := &schedulingv1alpha1.NetworkFunctionReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentutil.GenerateReplicaSetName(nfDeployment.Name, updatedSpecHash),
			Namespace: nfDeployment.Namespace,
			Labels:    nfDeployment.Labels,
		},
		Spec: schedulingv1alpha1.NetworkFunctionReplicaSetSpec{
			Replicas: ptr.To[int32](0),
			Selector: nfDeployment.Spec.Selector.DeepCopy(),
			Template: *nfDeployment.Spec.Template.DeepCopy(),
		},
	}
	if newRS.ObjectMeta.Labels == nil {
		newRS.ObjectMeta.Labels = make(map[string]string)
	}
	newRS.ObjectMeta.Labels[schedulingv1alpha1.NFSpecHashLabel] = updatedSpecHash
	newRS.Spec.Selector.MatchLabels[schedulingv1alpha1.NFSpecHashLabel] = updatedSpecHash
	newRS.Spec.Template.Labels[schedulingv1alpha1.NFSpecHashLabel] = updatedSpecHash

	// Calculate the number of replicas for the new ReplicaSet.
	// This is based on the desired number of replicas in the NetworkFunctionDeployment spec,
	newReplicasCount, err := deploymentutil.NewRSNewReplicas(nfDeployment, allRSs, newRS)
	if err != nil {
		return false, fmt.Errorf("failed to calculate new replicas count for NetworkFunctionDeployment %s/%s: %v",
			nfDeployment.Namespace, nfDeployment.Name, err)
	}
	newRS.Spec.Replicas = &newReplicasCount

	// Set the ownerRef for the ReplicaSet, ensuring that the ReplicaSet
	// will be deleted when the NetworkFunctionDeployment CR is deleted.
	err = controllerutil.SetControllerReference(nfDeployment, newRS, r.Scheme)
	if err != nil {
		return false, fmt.Errorf("failed to set owner reference for NetworkFunctionReplicaSet %s/%s: %v",
			newRS.Namespace, newRS.Name, err)
	}

	// Set annotations for the replicaset
	deploymentutil.SetNewReplicaSetAnnotations(ctx, nfDeployment, newRS, newRevision, false)

	// Create the new ReplicaSet. If it already exists, then we need to check for possible
	// hash collisions. If there is any other error, we need to report it in the status of
	// the Deployment.
	//alreadyExists := false
	err = r.Create(ctx, newRS)

	switch {
	// We may end up hitting this due to a slow cache or a fast resync of the Deployment.
	case errors.IsAlreadyExists(err):
		//alreadyExists = true

		// Fetch a copy of the ReplicaSet.
		preExistingRS := &schedulingv1alpha1.NetworkFunctionReplicaSet{}
		getErr := r.Get(ctx, client.ObjectKey{Namespace: newRS.Namespace, Name: newRS.Name}, preExistingRS)
		if getErr != nil {
			return false, err
		}

		// If the Deployment owns the ReplicaSet and the ReplicaSet's PodTemplateSpec is semantically
		// deep equal to the PodTemplateSpec of the Deployment, it's the Deployment's new ReplicaSet.
		// Otherwise, this is a hash collision and we need to increment the collisionCount field in
		// the status of the Deployment and requeue to try the creation in the next sync.
		controllerRef := metav1.GetControllerOf(preExistingRS)
		if controllerRef != nil &&
			controllerRef.UID == nfDeployment.UID &&
			deploymentutil.EqualIgnoreHash(&nfDeployment.Spec.Template, &preExistingRS.Spec.Template) {
			err = nil
			break
		}

		// Matching ReplicaSet is not equal - increment the collisionCount in the DeploymentStatus
		// and requeue the Deployment.
		if nfDeployment.Status.CollisionCount == nil {
			nfDeployment.Status.CollisionCount = new(int32)
		}
		preCollisionCount := *nfDeployment.Status.CollisionCount
		*nfDeployment.Status.CollisionCount++
		// Update the collisionCount for the Deployment and let it requeue by returning the original
		// error.
		dErr := r.Status().Update(ctx, nfDeployment)
		if dErr == nil {
			logger.V(2).Info("Found a hash collision for network function - bumping collisionCount to resolve it",
				"nfDeployment", nfDeployment, "oldCollisionCount", preCollisionCount, "newCollisionCount", *nfDeployment.Status.CollisionCount)
		}
		return false, err
	case errors.HasStatusCause(err, v1.NamespaceTerminatingCause):
		// if the namespace is terminating, all subsequent creates will fail and we can safely do nothing
		return false, err
	case err != nil:
		return false, err
	}
	//if !alreadyExists && newReplicasCount > 0 {
	//	r.eventRecorder.Eventf(nfDeployment, v1.EventTypeNormal, "ScalingReplicaSet", "Scaled up replica set %s from 0 to %d", createdRS.Name, newReplicasCount)
	//}

	needsUpdate := deploymentutil.SetNFDeploymentRevision(nfDeployment, newRevision)
	if needsUpdate {
		err = r.Status().Update(ctx, nfDeployment)
	}
	return true, err
}

// cleanupOldReplicaSets is responsible for removing old ReplicaSets that are no longer needed after
// a new ReplicaSet has been created. It should only be called once the new ReplicaSet is up and running,
// and the old ReplicaSets have been scaled down to zero. This function will delete all old ReplicaSets
// and return any errors encountered during the deletion process.
func (r *NetworkFunctionDeploymentReconciler) cleanupOldReplicaSets(ctx context.Context,
	oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet, nfDeployment *schedulingv1alpha1.NetworkFunctionDeployment) error {
	logger := logf.FromContext(ctx)
	cleanableRSes := deploymentutil.FilterAliveReplicaSets(oldRSs)

	sort.Sort(deploymentutil.ReplicaSetsByRevision(cleanableRSes))
	logger.V(4).Info("Looking to cleanup old replica sets for deployment",
		"NetworkFunctionDeployment", nfDeployment)

	for i := 0; i < len(cleanableRSes); i++ {
		rs := cleanableRSes[i]
		// Avoid delete replica set with non-zero replica counts
		if rs.Status.Replicas != 0 || *(rs.Spec.Replicas) != 0 || rs.Generation > rs.Status.ObservedGeneration || rs.DeletionTimestamp != nil {
			continue
		}
		logger.V(4).Info("Trying to cleanup nf replica set for nf deployment",
			"NetworkFunctionReplicaSet", rs, "NetworkFunctionDeployment", nfDeployment)
		if err := r.Delete(ctx, rs); err != nil && !errors.IsNotFound(err) {
			// Return error instead of aggregating and continuing DELETEs on the theory
			// that we may be overloading the api server.
			return err
		}
	}

	return nil
}

func (r *NetworkFunctionDeploymentReconciler) scaleReplicaSet(ctx context.Context,
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet, newScale int32,
	nfDeployment *schedulingv1alpha1.NetworkFunctionDeployment, forceUpdate bool,
) (scaled bool, updatedRS *schedulingv1alpha1.NetworkFunctionReplicaSet, err error) {
	// Don't scale, unless it's a forced update or the replicas actually differ
	if !forceUpdate && *(rs.Spec.Replicas) == newScale {
		return false, rs, nil
	}

	sizeNeedsUpdate := *(rs.Spec.Replicas) != newScale
	annotationsNeedUpdate := deploymentutil.ReplicasAnnotationsNeedUpdate(rs, *(nfDeployment.Spec.Replicas), *(nfDeployment.Spec.Replicas)+deploymentutil.MaxSurge(nfDeployment))

	//scaled := false
	if sizeNeedsUpdate || annotationsNeedUpdate {
		//oldScale := *(rs.Spec.Replicas)
		rsCopy := rs.DeepCopy()
		*(rsCopy.Spec.Replicas) = newScale
		deploymentutil.SetReplicasAnnotations(rsCopy, *(nfDeployment.Spec.Replicas), *(nfDeployment.Spec.Replicas)+deploymentutil.MaxSurge(nfDeployment))
		err = r.Update(ctx, rsCopy)
		//if err == nil && sizeNeedsUpdate {
		//	var scalingOperation string
		//	if oldScale < newScale {
		//		scalingOperation = "up"
		//	} else {
		//		scalingOperation = "down"
		//	}
		//	scaled = true
		//	r.eventRecorder.Eventf(nfDeployment, v1.EventTypeNormal, "ScalingReplicaSet", "Scaled %s replica set %s from %d to %d", scalingOperation, rs.Name, oldScale, newScale)
		//}
	}
	return scaled, rs, err
}
