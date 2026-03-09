package util

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/validation"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "loom/api/core/v1alpha1"
	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
	"loom/pkg/util/hash"
)

const (
	RevisionAnnotation        = "networkfunction.iml.io/revision"
	DesiredReplicasAnnotation = "networkfunction.iml.io/desired-replicas"
	MaxReplicasAnnotation     = "networkfunction.iml.io/max-replicas"

	MinimumReplicasAvailable   = "MinimumReplicasAvailable"
	MinimumReplicasUnavailable = "MinimumReplicasUnavailable"
)

func GetAvailableReplicaCountForReplicaSets(replicaSets []*schedulingv1alpha1.NetworkFunctionReplicaSet) int32 {
	availableReplicaCount := int32(0)
	for _, rs := range replicaSets {
		if rs != nil {
			availableReplicaCount += rs.Status.AvailableReplicas
		}
	}
	return availableReplicaCount
}

func GetReplicaCountForReplicaSets(replicaSet []*schedulingv1alpha1.NetworkFunctionReplicaSet) int32 {
	replicaCount := int32(0)
	for _, rs := range replicaSet {
		if rs != nil && rs.Spec.Replicas != nil {
			replicaCount += *rs.Spec.Replicas
		}
	}
	return replicaCount
}

func GetActualReplicaCountForReplicaSets(replicaSet []*schedulingv1alpha1.NetworkFunctionReplicaSet) int32 {
	actualReplicaCount := int32(0)
	for _, rs := range replicaSet {
		if rs != nil {
			actualReplicaCount += rs.Status.Replicas
		}
	}
	return actualReplicaCount
}

func GetReadyReplicaCountForReplicaSets(replicaSet []*schedulingv1alpha1.NetworkFunctionReplicaSet) int32 {
	readyReplicaCount := int32(0)
	for _, rs := range replicaSet {
		if rs != nil {
			readyReplicaCount += rs.Status.ReadyReplicas
		}
	}
	return readyReplicaCount
}

func ResolveFenceposts(maxSurge, maxUnavailable *intstrutil.IntOrString, desired int32) (int32, int32, error) {
	surge, err := intstrutil.GetScaledValueFromIntOrPercent(
		intstrutil.ValueOrDefault(maxSurge, intstrutil.FromInt32(0)), int(desired), true)
	if err != nil {
		return 0, 0, err
	}
	unavailable, err := intstrutil.GetScaledValueFromIntOrPercent(
		intstrutil.ValueOrDefault(maxUnavailable, intstrutil.FromInt32(0)), int(desired), false)
	if err != nil {
		return 0, 0, err
	}

	if surge == 0 && unavailable == 0 {
		// Validation should never allow the user to explicitly use zero values for both maxSurge
		// maxUnavailable. Due to rounding down maxUnavailable though, it may resolve to zero.
		// If both fenceposts resolve to zero, then we should set maxUnavailable to 1 on the
		// theory that surge might not work due to quota.
		unavailable = 1
	}

	return int32(surge), int32(unavailable), nil
}

func IsRollingUpdate(nf *corev1alpha1.NetworkFunction) bool {
	return nf.Spec.Strategy == nil || nf.Spec.Strategy.Type == corev1alpha1.DeploymentStrategyTypeRollingUpdate
}

func MaxUnavailable(nf *corev1alpha1.NetworkFunction) int32 {
	if !IsRollingUpdate(nf) || *(nf.Spec.Replicas) == 0 {
		return 0
	}
	maxUnavailable, _, _ := ResolveFenceposts(nf.Spec.Strategy.RollingUpdate.MaxSurge,
		nf.Spec.Strategy.RollingUpdate.MaxUnavailable, *(nf.Spec.Replicas))
	return maxUnavailable
}

func MinAvailable(nf *corev1alpha1.NetworkFunction) int32 {
	if !IsRollingUpdate(nf) {
		return 0
	}
	return *(nf.Spec.Replicas) - MaxUnavailable(nf)
}

func MaxSurge(nf *corev1alpha1.NetworkFunction) int32 {
	if !IsRollingUpdate(nf) {
		return 0
	}
	// Error caught by validation
	maxSurge, _, _ := ResolveFenceposts(nf.Spec.Strategy.RollingUpdate.MaxSurge,
		nf.Spec.Strategy.RollingUpdate.MaxUnavailable, *(nf.Spec.Replicas))
	return maxSurge
}

func MaxRevision(allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet) int64 {
	maxRevision := int64(0)
	for _, rs := range allRSs {
		if v, err := Revision(rs); err != nil {
			continue
		} else {
			maxRevision = v
		}
	}
	return maxRevision
}

// Revision returns the revision number of the input object.
func Revision(obj runtime.Object) (int64, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[RevisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func NewNfCondition(condType corev1alpha1.NFConditionType, status metav1.ConditionStatus,
	reason, message string) *corev1alpha1.NFCondition {
	return &corev1alpha1.NFCondition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

func GetNfCondition(status *corev1alpha1.NetworkFunctionStatus,
	condType corev1alpha1.NFConditionType) *corev1alpha1.NFCondition {
	for i := range status.Conditions {
		cond := &status.Conditions[i]
		if cond.Type == condType {
			return cond
		}
	}
	return nil
}

func SetNfCondition(status *corev1alpha1.NetworkFunctionStatus, condition corev1alpha1.NFCondition) {
	currentCondition := GetNfCondition(status, condition.Type)
	if currentCondition != nil && currentCondition.Status == condition.Status &&
		currentCondition.Reason == condition.Reason {
		// No update needed
		return
	}
	if currentCondition != nil && currentCondition.Status != condition.Status {
		// Status has changed, update the condition and preserve the last transition time
		condition.LastTransitionTime = currentCondition.LastTransitionTime
	}
	newConditions := filterConditions(status.Conditions, condition.Type)
	status.Conditions = append(newConditions, condition)
}

func RemoveNfCondition(status *corev1alpha1.NetworkFunctionStatus, condType corev1alpha1.NFConditionType) {
	status.Conditions = filterConditions(status.Conditions, condType)
}

func filterConditions(
	conditions []corev1alpha1.NFCondition, condType corev1alpha1.NFConditionType,
) []corev1alpha1.NFCondition {
	newConditions := make([]corev1alpha1.NFCondition, 0)
	for i := range conditions {
		if conditions[i].Type != condType {
			newConditions = append(newConditions, conditions[i])
		}
	}
	return newConditions
}

func GenerateReplicaSetName(nfName, currentSpecHash string) string {
	maxNFNameLength := validation.DNS1123SubdomainMaxLength - len("-") - len(currentSpecHash)

	if len(nfName) > maxNFNameLength && maxNFNameLength > 0 {
		return nfName[:maxNFNameLength] + "-" + currentSpecHash
	}

	return nfName + "-" + currentSpecHash
}

func ComputeSpecHash(nf *corev1alpha1.NetworkFunction) string {
	hasher := fnv.New32a()
	hash.DeepHashObject(hasher, nf.Spec.Template)

	// Add collisionCount in the hash if it exists.
	if nf.Status.CollisionCount != nil {
		collisionCountBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(collisionCountBytes, uint32(*nf.Status.CollisionCount))
		_, _ = hasher.Write(collisionCountBytes)
	}

	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

// EqualIgnoreHash returns true if two given podTemplateSpec are equal, ignoring the diff in value of
// Labels[NF_BINDING_SPEC_HASH_LABEL]
// We ignore NF_BINDING_SPEC_HASH_LABEL because:
//  1. The hash result would be different upon podTemplateSpec API changes
//     (e.g. the addition of a new field will cause the hash code to change)
//  2. The deployment template won't have hash labels
func EqualIgnoreHash(t1 *schedulingv1alpha1.NetworkFunctionBindingTemplate,
	t2 *schedulingv1alpha1.NetworkFunctionBindingTemplate) bool {
	t1Copy := t1.DeepCopy()
	t2Copy := t2.DeepCopy()
	// Remove hash labels from template.Labels before comparing
	delete(t1Copy.Labels, corev1alpha1.NF_BINDING_SPEC_HASH_LABEL)
	delete(t2Copy.Labels, corev1alpha1.NF_BINDING_SPEC_HASH_LABEL)
	return apiequality.Semantic.DeepEqual(t1Copy, t2Copy)
}

// FilterActiveReplicaSets returns replica sets that have (or at least ought to have) pods.
func FilterActiveReplicaSets(replicaSets []*schedulingv1alpha1.NetworkFunctionReplicaSet,
) []*schedulingv1alpha1.NetworkFunctionReplicaSet {
	activeFilter := func(rs *schedulingv1alpha1.NetworkFunctionReplicaSet) bool {
		return rs != nil && *(rs.Spec.Replicas) > 0
	}
	return FilterReplicaSets(replicaSets, activeFilter)
}

// FilterAliveReplicaSets returns replica sets that are not marked for deletion (i.e. their DeletionTimestamp is nil).
// This is different from FilterActiveReplicaSets, which filters based on the number of desired replicas.
// A replica set that is marked for deletion may still have desired replicas greater than zero,
// but it should not be considered alive.
func FilterAliveReplicaSets(replicaSets []*schedulingv1alpha1.NetworkFunctionReplicaSet,
) []*schedulingv1alpha1.NetworkFunctionReplicaSet {
	aliveFilter := func(rs *schedulingv1alpha1.NetworkFunctionReplicaSet) bool {
		return rs != nil && rs.ObjectMeta.DeletionTimestamp == nil
	}
	return FilterReplicaSets(replicaSets, aliveFilter)
}

// FilterReplicaSets returns replica sets that are filtered by filterFn (all returned ones should match filterFn).
func FilterReplicaSets(RSes []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	filterFn func(set *schedulingv1alpha1.NetworkFunctionReplicaSet) bool,
) []*schedulingv1alpha1.NetworkFunctionReplicaSet {
	var filtered []*schedulingv1alpha1.NetworkFunctionReplicaSet
	for i := range RSes {
		if filterFn(RSes[i]) {
			filtered = append(filtered, RSes[i])
		}
	}
	return filtered
}

func NeedsScaling(
	ctx context.Context, nf *corev1alpha1.NetworkFunction,
	newRS *schedulingv1alpha1.NetworkFunctionReplicaSet, oldRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
) (upscaleNeeded bool, err error) {
	allRSs := append(oldRSs, newRS)
	for _, rs := range FilterActiveReplicaSets(allRSs) {
		desired, ok := GetDesiredReplicasAnnotation(ctx, rs)
		if !ok {
			continue
		}
		if desired != *(nf.Spec.Replicas) {
			return true, nil
		}
	}
	return false, nil
}

func SetNewReplicaSetAnnotations(ctx context.Context, nf *corev1alpha1.NetworkFunction,
	newRS *schedulingv1alpha1.NetworkFunctionReplicaSet, newRevision string, exists bool,
) (annotationsChanged bool) {
	logger := logf.FromContext(ctx)
	// First, check if the annotations are nil and initialize them if so
	if newRS.Annotations == nil {
		newRS.Annotations = make(map[string]string)
	}
	// Then, copy the annotations from the nf
	for k, v := range nf.Annotations {
		// newRS revision is updated automatically in getNewReplicaSet, and the deployment's revision number is then updated
		// by copying its newRS revision number. We should not copy deployment's revision to its newRS, since the update of
		// deployment revision number may fail (revision becomes stale) and the revision number in newRS is more reliable.
		if _, exist := newRS.Annotations[k]; skipCopyAnnotation(k) || (exist && newRS.Annotations[k] == v) {
			continue
		}
		newRS.Annotations[k] = v
		annotationsChanged = true
	}
	oldRevision, _ := newRS.Annotations[RevisionAnnotation]
	// The newRS's revision should be the greatest among all RSes. Usually, its revision number is newRevision (the max revision number
	// of all old RSes + 1). However, it's possible that some of the old RSes are deleted after the newRS revision being updated, and
	// newRevision becomes smaller than newRS's revision. We should only update newRS revision when it's smaller than newRevision.

	oldRevisionInt, err := strconv.ParseInt(oldRevision, 10, 64)
	if err != nil {
		if oldRevision != "" {
			logger.Info("Updating nf replica set revision OldRevision not int", "err", err)
			return false
		}
		//If the RS annotation is empty then initialise it to 0
		oldRevisionInt = 0
	}
	newRevisionInt, err := strconv.ParseInt(newRevision, 10, 64)
	if err != nil {
		logger.Info("Updating nf replica set revision NewRevision not int", "err", err)
		return false
	}
	if oldRevisionInt < newRevisionInt {
		newRS.Annotations[RevisionAnnotation] = newRevision
		annotationsChanged = true
		logger.V(4).Info("Updating nf replica set revision",
			"replicaSet", newRS, "newRevision", newRevision)
	}
	// If the new replica set is about to be created, we need to add replica annotations to it.
	if !exists && SetReplicasAnnotations(newRS, *(nf.Spec.Replicas), *(nf.Spec.Replicas)+MaxSurge(nf)) {
		annotationsChanged = true
	}
	return annotationsChanged
}

// GetDesiredReplicasAnnotation returns the number of desired replicas
func GetDesiredReplicasAnnotation(ctx context.Context, rs *schedulingv1alpha1.NetworkFunctionReplicaSet) (int32, bool) {
	return getNonNegativeInt32FromAnnotationVerbose(ctx, rs, DesiredReplicasAnnotation)
}
func getNonNegativeInt32FromAnnotationVerbose(
	ctx context.Context, rs *schedulingv1alpha1.NetworkFunctionReplicaSet, annotationKey string) (int32, bool) {
	logger := logf.FromContext(ctx)
	value, ok, err := getNonNegativeInt32FromAnnotation(rs, annotationKey)
	if err != nil {
		logger.V(2).Info("Could not convert the value with annotation key for the replica set",
			"annotationValue", rs.Annotations[annotationKey], "annotationKey", annotationKey, "replicaSet", rs)
	}
	return value, ok
}

func getNonNegativeInt32FromAnnotation(
	rs *schedulingv1alpha1.NetworkFunctionReplicaSet, annotationKey string,
) (int32, bool, error) {
	annotationValue, ok := rs.Annotations[annotationKey]
	if !ok {
		return int32(0), false, nil
	}
	intValue, err := strconv.ParseUint(annotationValue, 10, 32)
	if err != nil {
		return int32(0), false, err
	}
	if intValue > math.MaxInt32 {
		return int32(0), false, fmt.Errorf("value %d is out of range (higher than %d)", intValue, math.MaxInt32)
	}
	return int32(intValue), true, nil
}

func SetReplicasAnnotations(rs *schedulingv1alpha1.NetworkFunctionReplicaSet, desiredReplicas, maxReplicas int32) bool {
	updated := false
	if rs.Annotations == nil {
		rs.Annotations = make(map[string]string)
	}
	desiredString := fmt.Sprintf("%d", desiredReplicas)
	if hasString := rs.Annotations[DesiredReplicasAnnotation]; hasString != desiredString {
		rs.Annotations[DesiredReplicasAnnotation] = desiredString
		updated = true
	}
	maxString := fmt.Sprintf("%d", maxReplicas)
	if hasString := rs.Annotations[MaxReplicasAnnotation]; hasString != maxString {
		rs.Annotations[MaxReplicasAnnotation] = maxString
		updated = true
	}
	return updated
}

var annotationsToSkip = map[string]bool{
	RevisionAnnotation:        true,
	DesiredReplicasAnnotation: true,
	MaxReplicasAnnotation:     true,
}

// skipCopyAnnotation returns true if we should skip copying the annotation with the given annotation key
func skipCopyAnnotation(key string) bool {
	return annotationsToSkip[key]
}

// SetNFRevision updates the revision for a deployment.
func SetNFRevision(nf *corev1alpha1.NetworkFunction, revision string) bool {
	updated := false

	if nf.Annotations == nil {
		nf.Annotations = make(map[string]string)
	}
	if nf.Annotations[RevisionAnnotation] != revision {
		nf.Annotations[RevisionAnnotation] = revision
		updated = true
	}

	return updated
}

// NFDeploymentComplete considers a nf deployment to be complete once all of its desired replicas
// are updated and available, and no old pods are running.
func NFDeploymentComplete(nf *corev1alpha1.NetworkFunction) bool {
	// A deployment is considered complete if it has observed the latest generation
	// and the number of updated replicas equals desired replicas
	return nf.Status.UpdatedReplicas == *(nf.Spec.Replicas) &&
		nf.Status.Replicas == *(nf.Spec.Replicas) &&
		nf.Status.AvailableReplicas == *(nf.Spec.Replicas) &&
		nf.Status.ObservedGeneration >= nf.Generation
}

func ReplicasAnnotationsNeedUpdate(rs *schedulingv1alpha1.NetworkFunctionReplicaSet,
	desiredReplicas, maxReplicas int32) bool {
	if rs.Annotations == nil {
		return true
	}
	desiredString := fmt.Sprintf("%d", desiredReplicas)
	if hasString := rs.Annotations[DesiredReplicasAnnotation]; hasString != desiredString {
		return true
	}
	maxString := fmt.Sprintf("%d", maxReplicas)
	if hasString := rs.Annotations[MaxReplicasAnnotation]; hasString != maxString {
		return true
	}
	return false
}

// ReplicaSetsByCreationTimestamp sorts a list of ReplicaSet by creation timestamp, using their names as a tie breaker.
type ReplicaSetsByCreationTimestamp []*schedulingv1alpha1.NetworkFunctionReplicaSet

func (o ReplicaSetsByCreationTimestamp) Len() int      { return len(o) }
func (o ReplicaSetsByCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o ReplicaSetsByCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

// ReplicaSetsByRevision sorts a list of ReplicaSet by revision, using their creation timestamp or name as a tie breaker.
// By using the creation timestamp, this sorts from old to new replica sets.
type ReplicaSetsByRevision []*schedulingv1alpha1.NetworkFunctionReplicaSet

func (o ReplicaSetsByRevision) Len() int      { return len(o) }
func (o ReplicaSetsByRevision) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o ReplicaSetsByRevision) Less(i, j int) bool {
	revision1, err1 := Revision(o[i])
	revision2, err2 := Revision(o[j])
	if err1 != nil || err2 != nil || revision1 == revision2 {
		return ReplicaSetsByCreationTimestamp(o).Less(i, j)
	}
	return revision1 < revision2
}

// NewRSNewReplicas calculates the number of replicas a nf's new RS should have.
// When one of the followings is true, we're rolling out the nf; otherwise, we're scaling it.
// 1) The new RS is saturated: newRS's replicas == nf's replicas
// 2) Max number of pods allowed is reached: nf's replicas + maxSurge == all RSs' replicas
func NewRSNewReplicas(nf *corev1alpha1.NetworkFunction,
	allRSs []*schedulingv1alpha1.NetworkFunctionReplicaSet,
	newRS *schedulingv1alpha1.NetworkFunctionReplicaSet) (int32, error) {
	switch nf.Spec.Strategy.Type {
	case corev1alpha1.DeploymentStrategyTypeRollingUpdate:
		// Find the total number of pods
		currentPodCount := GetReplicaCountForReplicaSets(allRSs)
		maxTotalPods := *(nf.Spec.Replicas) + MaxSurge(nf)
		if currentPodCount >= maxTotalPods {
			// Cannot scale up.
			return *(newRS.Spec.Replicas), nil
		}
		// Scale up.
		scaleUpCount := maxTotalPods - currentPodCount
		// Do not exceed the number of desired replicas.
		scaleUpCount = min(scaleUpCount, *(nf.Spec.Replicas)-*(newRS.Spec.Replicas))
		return *(newRS.Spec.Replicas) + scaleUpCount, nil
	case corev1alpha1.DeploymentStrategyTypeRecreate:
		return *(nf.Spec.Replicas), nil
	default:
		return 0, fmt.Errorf("deployment type %v isn't supported", nf.Spec.Strategy.Type)
	}
}
