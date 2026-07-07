package util

import (
	"context"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/fnv"
	"net/netip"
	"reflect"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	"github.com/mantra6g/iml/operator/pkg/util/ptr"
	v1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/dump"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// semanticIgnoreResourceVersion does semantic deep equality checks for objects
// but excludes ResourceVersion of ObjectReference. They are used when comparing
// endpoints in Endpoints and EndpointSlice objects to avoid unnecessary updates
// caused by nf resourceVersion change.
var semanticIgnoreResourceVersion = conversion.EqualitiesOrDie(
	func(a, b v1.ObjectReference) bool {
		a.ResourceVersion = ""
		b.ResourceVersion = ""
		return a == b
	},
)

// NFProjectionKey encapsulates all nf information required to find services that may need their endpoints to be updated.
type NFProjectionKey struct {
	Namespace string
	Labels    labels.Set // this is set to the nf's current labels
	OldLabels labels.Set // this is set if the nf's labels changed (in an Update event)
	NFChanged bool       // set to true if the nf's fields changed in a way that may affect its endpoints membership
}

func GetNFUpdateProjectionKey(oldObj, newObj interface{}) *NFProjectionKey {
	newNF := getNFFromObject(newObj)
	oldNF := getNFFromObject(oldObj)
	if newNF == nil && oldNF == nil {
		utilruntime.HandleError(fmt.Errorf("unexpected nf event with both old/new values as nil, ignoring"))
		return nil
	}
	if oldNF == nil {
		return &NFProjectionKey{
			Namespace: newNF.Namespace,
			Labels:    newNF.Labels,
		}
	}
	if newNF == nil {
		return &NFProjectionKey{
			Namespace: oldNF.Namespace,
			Labels:    oldNF.Labels,
		}
	}

	// If we reached here, both old/new nf objects are non-nil, so it's an update event.
	// Safe to ignore nf informer resync events as service informer already handles resync for all services.
	if newNF.ResourceVersion == oldNF.ResourceVersion {
		return nil
	}

	nfChanged, labelsChanged := nfEndpointsChanged(oldNF, newNF)

	// If both the nf and labels are unchanged, no update is needed.
	if !nfChanged && !labelsChanged {
		return nil
	}

	// If only the nf has changed, projection key can be created with just the new nf.
	if !labelsChanged {
		return &NFProjectionKey{
			Namespace: newNF.Namespace,
			Labels:    newNF.Labels,
		}
	}

	// The nf labels have changed, so the projection key needs to include both its old/new labels.
	// Additionally, NFChanged field indicates if union/diff of matching services should be reconciled.
	return &NFProjectionKey{
		Namespace: newNF.Namespace,
		Labels:    newNF.Labels,
		OldLabels: oldNF.Labels,
		NFChanged: nfChanged,
	}
}

func GetServicesToUpdate(k8sClient client.Client, ctx context.Context, key *NFProjectionKey) (sets.Set[string], error) {
	if key == nil {
		return nil, nil
	}

	services, err := getServicesForNF(k8sClient, ctx, key.Namespace, key.Labels)
	if err != nil {
		return nil, err
	}

	// If the nf's labels changed in an update event, we'll need to consider all the affected services.
	if key.OldLabels != nil {
		oldServices, err := getServicesForNF(k8sClient, ctx, key.Namespace, key.OldLabels)
		if err != nil {
			return nil, err
		}

		services = determineNeededServiceUpdates(oldServices, services, key.NFChanged)
	}

	return services, nil
}

// getServicesForNF returns a set of network services matching the given nf's namespace and labels (via service selector).
func getServicesForNF(k8sClient client.Client, ctx context.Context, namespace string, nfLabels labels.Set) (sets.Set[string], error) {
	netservices := &corev1alpha1.NetworkServiceList{}
	err := k8sClient.List(ctx, netservices, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	set := sets.Set[string]{}
	for i := range netservices.Items {
		service := &netservices.Items[i]
		if service.Spec.Selector == nil {
			// If the service has a nil selector this means selectors match nothing, not everything.
			continue
		}

		if labels.ValidatedSetSelector(service.Spec.Selector).Matches(nfLabels) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(service)
			if err != nil {
				return nil, err
			}
			set.Insert(key)
		}
	}
	return set, nil
}

// PortMapKey is used to uniquely identify groups of endpoint ports.
type PortMapKey string

// NewPortMapKey generates a PortMapKey from endpoint ports.
func NewPortMapKey(endpointPorts []discovery.EndpointPort) PortMapKey {
	// Normalize nil to empty slice so they hash the same.
	if endpointPorts == nil {
		endpointPorts = []discovery.EndpointPort{}
	}
	sort.Sort(portsInOrder(endpointPorts))
	return PortMapKey(deepHashObjectToString(endpointPorts))
}

// deepHashObjectToString creates a unique hash string from a go object.
func deepHashObjectToString(objectToWrite interface{}) string {
	hasher := fnv.New128a()
	deepHashObject(hasher, objectToWrite)
	return hex.EncodeToString(hasher.Sum(nil)[0:])
}

// ShouldNFBeInEndpoints returns true if a specified nf should be in an
// Endpoints or EndpointSlice resource. Terminating nfs are only included if
// includeTerminating is true.
func ShouldNFBeInEndpoints(nf *corev1alpha1.NetworkFunction, includeTerminating bool) bool {
	// "Terminal" describes when a NF is complete (in a succeeded or failed phase).
	// This is distinct from the "Terminating" condition which represents when a NF
	// is being terminated (metadata.deletionTimestamp is non nil).
	if isNFTerminal(nf) {
		return false
	}

	if len(nf.Status.AssignedIP) == 0 {
		return false
	}

	if !includeTerminating && nf.DeletionTimestamp != nil {
		return false
	}

	return true
}

// nfEndpointsChanged returns two boolean values. The first is true if the nf has
// changed in a way that may change existing endpoints. The second value is true if the
// nf has changed in a way that may affect which Services it matches.
func nfEndpointsChanged(oldNF, newNF *corev1alpha1.NetworkFunction) (bool, bool) {
	// Check if the nf labels have changed, indicating a possible
	// change in the service membership
	labelsChanged := false
	if !reflect.DeepEqual(newNF.Labels, oldNF.Labels) {
		labelsChanged = true
	}

	// If the nf's deletion timestamp is set, remove endpoint from ready address.
	if newNF.DeletionTimestamp != oldNF.DeletionTimestamp {
		return true, labelsChanged
	}
	// If the nf's readiness has changed, the associated endpoint address
	// will move from the unready endpoints set to the ready endpoints.
	// So for the purposes of an endpoint, a readiness change on a nf
	// means we have a changed nf.
	if IsNFReady(oldNF) != IsNFReady(newNF) {
		return true, labelsChanged
	}

	// Check if the nf IPs have changed
	if oldNF.Status.AssignedIP != newNF.Status.AssignedIP {
		return true, labelsChanged
	}

	// Endpoints may also reference a nf's Name, Namespace, UID, and NodeName, but
	// the first three are immutable, and NodeName is immutable once initially set,
	// which happens before the nf gets an IP.

	return false, labelsChanged
}

func getNFFromObject(obj interface{}) *corev1alpha1.NetworkFunction {
	if obj == nil {
		return nil
	}
	if nf, ok := obj.(*corev1alpha1.NetworkFunction); ok {
		return nf
	}
	// If we reached here it means the nf was deleted but its final state is unrecorded.
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
		return nil
	}
	nf, ok := tombstone.Obj.(*corev1alpha1.NetworkFunction)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a NetworkFunction: %#v", obj))
		return nil
	}
	return nf
}

func determineNeededServiceUpdates(oldServices, services sets.Set[string], nfChanged bool) sets.Set[string] {
	if nfChanged {
		// if the labels and nf changed, all services need to be updated
		services = services.Union(oldServices)
	} else {
		// if only the labels changed, services not common to both the new
		// and old service set (the disjuntive union) need to be updated
		services = services.Difference(oldServices).Union(oldServices.Difference(services))
	}
	return services
}

// portsInOrder helps sort endpoint ports in a consistent way for hashing.
type portsInOrder []discovery.EndpointPort

func (sl portsInOrder) Len() int      { return len(sl) }
func (sl portsInOrder) Swap(i, j int) { sl[i], sl[j] = sl[j], sl[i] }
func (sl portsInOrder) Less(i, j int) bool {
	h1 := deepHashObjectToString(sl[i])
	h2 := deepHashObjectToString(sl[j])
	return h1 < h2
}

// EndpointsEqualBeyondHash returns true if endpoints have equal attributes
// but excludes equality checks that would have already been covered with
// endpoint hashing (see hashEndpoint func for more info) and ignores difference
// in ResourceVersion of TargetRef.
func EndpointsEqualBeyondHash(ep1, ep2 *discovery.Endpoint) bool {
	if stringPtrChanged(ep1.NodeName, ep2.NodeName) {
		return false
	}

	if stringPtrChanged(ep1.Zone, ep2.Zone) {
		return false
	}

	if boolPtrChanged(ep1.Conditions.Ready, ep2.Conditions.Ready) {
		return false
	}

	if boolPtrChanged(ep1.Conditions.Serving, ep2.Conditions.Serving) {
		return false
	}

	if boolPtrChanged(ep1.Conditions.Terminating, ep2.Conditions.Terminating) {
		return false
	}

	if !semanticIgnoreResourceVersion.DeepEqual(ep1.TargetRef, ep2.TargetRef) {
		return false
	}

	return true
}

// boolPtrChanged returns true if a set of bool pointers have different values.
func boolPtrChanged(ptr1, ptr2 *bool) bool {
	if (ptr1 == nil) != (ptr2 == nil) {
		return true
	}
	if ptr1 != nil && ptr2 != nil && *ptr1 != *ptr2 {
		return true
	}
	return false
}

// stringPtrChanged returns true if a set of string pointers have different values.
func stringPtrChanged(ptr1, ptr2 *string) bool {
	if (ptr1 == nil) != (ptr2 == nil) {
		return true
	}
	if ptr1 != nil && ptr2 != nil && *ptr1 != *ptr2 {
		return true
	}
	return false
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
// copied from k8s.io/kubernetes/pkg/util/hash
func deepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	fmt.Fprint(hasher, dump.ForHash(objectToWrite))
}

// IsNFReady returns true if the NetworkFunction's Ready condition is true
func IsNFReady(nf *corev1alpha1.NetworkFunction) bool {
	return isNFReadyConditionTrue(nf.Status)
}

// IsNfTerminal returns true if a nf is terminal, all containers are stopped and cannot ever regress.
func isNFTerminal(nf *corev1alpha1.NetworkFunction) bool {
	return isNFPhaseTerminal(nf.Status.Phase)
}

// IsNFPhaseTerminal returns true if the nf's phase is terminal.
func isNFPhaseTerminal(phase corev1alpha1.NetworkFunctionPhase) bool {
	return phase == corev1alpha1.NetworkFunctionFailed
}

// IsNFReadyConditionTrue returns true if a nf is ready; false otherwise.
func isNFReadyConditionTrue(status corev1alpha1.NetworkFunctionStatus) bool {
	condition := getNFReadyCondition(&status)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// getNFReadyCondition extracts the nf ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func getNFReadyCondition(status *corev1alpha1.NetworkFunctionStatus) *corev1alpha1.NetworkFunctionCondition {
	if status == nil || status.Conditions == nil {
		return nil
	}

	for i := range status.Conditions {
		if status.Conditions[i].Type == corev1alpha1.NetworkFunctionReady {
			return &status.Conditions[i]
		}
	}
	return nil
}

// NFToEndpoint returns an Endpoint object generated from a NetworkFunction, a P4Target, and a NetworkService.
func NFToEndpoint(nf *corev1alpha1.NetworkFunction, p4target *corev1alpha1.P4Target, ns *corev1alpha1.NetworkService) discovery.Endpoint {
	serving := IsNFReady(nf)
	terminating := nf.DeletionTimestamp != nil
	// For compatibility reasons, "ready" should never be "true" if a pod is terminatng, unless
	// publishNotReadyAddresses was set.
	ready := serving && !terminating
	ep := discovery.Endpoint{
		Addresses: getEndpointAddresses(nf.Status, discovery.AddressTypeIPv6),
		Conditions: discovery.EndpointConditions{
			Ready:       &ready,
			Serving:     &serving,
			Terminating: &terminating,
		},
		TargetRef: &v1.ObjectReference{
			Kind:      "Pod",
			Namespace: nf.ObjectMeta.Namespace,
			Name:      nf.ObjectMeta.Name,
			UID:       nf.ObjectMeta.UID,
		},
	}

	if nf.Spec.TargetName != "" {
		ep.NodeName = &nf.Spec.TargetName
	}

	// Removed because there is no support for TopologyAware routing and load balancing yet.
	//if p4target != nil && p4target.Labels[v1.LabelTopologyZone] != "" {
	//	ep.Zone = ptr.To(p4target.Labels[v1.LabelTopologyZone])
	//}

	// No hostname on the network functions
	//if endpointutil.ShouldSetHostname(nf, ns) {
	//	ep.Hostname = &nf.Spec.Hostname
	//}

	return ep
}

// getEndpointPorts returns a list of EndpointPorts generated from a Service
// and Pod.
func getEndpointPorts(logger klog.Logger, service *v1.Service, pod *v1.Pod) []discovery.EndpointPort {
	endpointPorts := []discovery.EndpointPort{}

	// Allow headless service not to have ports.
	if len(service.Spec.Ports) == 0 && service.Spec.ClusterIP == v1.ClusterIPNone {
		return endpointPorts
	}

	for i := range service.Spec.Ports {
		servicePort := &service.Spec.Ports[i]

		portNum, err := FindPort(pod, servicePort)
		if err != nil {
			logger.V(4).Info("Failed to find port for service", "service", klog.KObj(service), "err", err)
			continue
		}

		endpointPorts = append(endpointPorts, discovery.EndpointPort{
			Name:        ptr.To(servicePort.Name),
			Port:        ptr.To(int32(portNum)),
			Protocol:    ptr.To(servicePort.Protocol),
			AppProtocol: servicePort.AppProtocol,
		})
	}

	return endpointPorts
}

// getEndpointAddresses returns a list of addresses generated from a pod status.
func getEndpointAddresses(nfStatus corev1alpha1.NetworkFunctionStatus, addressType discovery.AddressType) []string {
	addresses := make([]string, 0)

	// We parse and restringify the pod IP in case it is in an irregular format.
	ip, err := netip.ParseAddr(nfStatus.AssignedIP)
	if err != nil {
		return addresses
	}
	isIPv6PodIP := ip.Is6()
	if isIPv6PodIP && addressType == discovery.AddressTypeIPv6 {
		addresses = append(addresses, ip.String())
	}

	if !isIPv6PodIP && addressType == discovery.AddressTypeIPv4 {
		addresses = append(addresses, ip.String())
	}

	return addresses
}

// NewEndpointSlice returns an EndpointSlice generated from a network service and
// endpointMeta.
func NewEndpointSlice(logger logr.Logger, ns *corev1alpha1.NetworkService, controllerName string) *discovery.EndpointSlice {
	gvk := schema.GroupVersionKind{Version: "v1alpha1", Kind: "NetworkService"}
	ownerRef := metav1.NewControllerRef(ns, gvk)
	epSlice := &discovery.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          map[string]string{},
			GenerateName:    getEndpointSlicePrefix(ns.Name),
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
			Namespace:       ns.Namespace,
		},
		AddressType: discovery.AddressTypeIPv6,
		Endpoints:   []discovery.Endpoint{},
	}
	// add parent service labels
	epSlice.Labels, _ = SetEndpointSliceLabels(logger, epSlice, ns, controllerName)

	return epSlice
}

// getEndpointSlicePrefix returns a suitable prefix for an EndpointSlice name.
func getEndpointSlicePrefix(serviceName string) string {
	// use the dash (if the name isn't too long) to make the pod name a bit prettier
	prefix := fmt.Sprintf("%s-", serviceName)
	if len(apimachineryvalidation.NameIsDNSSubdomain(prefix, true)) != 0 {
		prefix = serviceName
	}
	return prefix
}

// OwnedBy returns true if the provided EndpointSlice is owned by the provided
// NetworkService.
func OwnedBy(endpointSlice *discovery.EndpointSlice, ns *corev1alpha1.NetworkService) bool {
	for _, o := range endpointSlice.OwnerReferences {
		if o.UID == ns.UID && o.Kind == ns.Kind && o.APIVersion == ns.APIVersion {
			return true
		}
	}
	return false
}

// GetSliceToFill will return the EndpointSlice that will be closest to full
// when numEndpoints are added. If no EndpointSlice can be found, a nil pointer
// will be returned.
func GetSliceToFill(endpointSlices []*discovery.EndpointSlice, numEndpoints, maxEndpoints int) (slice *discovery.EndpointSlice) {
	closestDiff := maxEndpoints
	var closestSlice *discovery.EndpointSlice
	for _, endpointSlice := range endpointSlices {
		currentDiff := maxEndpoints - (numEndpoints + len(endpointSlice.Endpoints))
		if currentDiff >= 0 && currentDiff < closestDiff {
			closestDiff = currentDiff
			closestSlice = endpointSlice
			if closestDiff == 0 {
				return closestSlice
			}
		}
	}
	return closestSlice
}

// AddTriggerTimeAnnotation adds a triggerTime annotation to an EndpointSlice
func AddTriggerTimeAnnotation(endpointSlice *discovery.EndpointSlice, triggerTime time.Time) {
	if endpointSlice.Annotations == nil {
		endpointSlice.Annotations = make(map[string]string)
	}

	if !triggerTime.IsZero() {
		endpointSlice.Annotations[v1.EndpointsLastChangeTriggerTime] = triggerTime.UTC().Format(time.RFC3339Nano)
	} else { // No new trigger time, clear the annotation.
		delete(endpointSlice.Annotations, v1.EndpointsLastChangeTriggerTime)
	}
}

// ServiceControllerKey returns a controller key for a Service but derived from
// an EndpointSlice.
func ServiceControllerKey(endpointSlice *discovery.EndpointSlice) (string, error) {
	if endpointSlice == nil {
		return "", fmt.Errorf("nil EndpointSlice passed to ServiceControllerKey()")
	}
	serviceName, ok := endpointSlice.Labels[discovery.LabelServiceName]
	if !ok || serviceName == "" {
		return "", fmt.Errorf("EndpointSlice missing %s label", discovery.LabelServiceName)
	}
	return fmt.Sprintf("%s/%s", endpointSlice.Namespace, serviceName), nil
}

// SetEndpointSliceLabels returns a map with the new endpoint slices labels and true if there was an update.
// Slices labels must be equivalent to the NetworkService labels except for the reserved IsHeadlessService, LabelServiceName and LabelManagedBy labels
// Changes to IsHeadlessService, LabelServiceName and LabelManagedBy labels on the Service do not result in updates to EndpointSlice labels.
func SetEndpointSliceLabels(logger logr.Logger, epSlice *discovery.EndpointSlice, ns *corev1alpha1.NetworkService, controllerName string) (map[string]string, bool) {
	updated := false
	epLabels := make(map[string]string)
	nsLabels := make(map[string]string)

	// check if the endpoint slice and the service have the same labels
	// clone current slice labels except the reserved labels
	for key, value := range epSlice.Labels {
		if isReservedLabelKey(key) {
			continue
		}
		// copy endpoint slice labels
		epLabels[key] = value
	}

	for key, value := range ns.Labels {
		if isReservedLabelKey(key) {
			logger.Info("NetworkService using reserved endpoint slices label", "networkservice", ns, "skipping", key, "label", value)
			continue
		}
		// copy networkservice labels
		nsLabels[key] = value
	}

	// if the labels are not identical update the slice with the corresponding networkservice labels
	if !apiequality.Semantic.DeepEqual(epLabels, nsLabels) {
		updated = true
	}

	// override endpoint slices reserved labels
	nsLabels[corev1alpha1.LabelNetworkServiceName] = ns.Name
	nsLabels[corev1alpha1.LabelManagedBy] = controllerName

	return nsLabels, updated
}

// isReservedLabelKey return true if the label is one of the reserved label for slices
func isReservedLabelKey(label string) bool {
	if label == corev1alpha1.LabelNetworkServiceName ||
		label == corev1alpha1.LabelManagedBy {
		return true
	}
	return false
}

// EndpointSliceEndpointLen helps sort endpoint slices by the number of
// endpoints they contain.
type EndpointSliceEndpointLen []*discovery.EndpointSlice

func (sl EndpointSliceEndpointLen) Len() int      { return len(sl) }
func (sl EndpointSliceEndpointLen) Swap(i, j int) { sl[i], sl[j] = sl[j], sl[i] }
func (sl EndpointSliceEndpointLen) Less(i, j int) bool {
	return len(sl[i].Endpoints) > len(sl[j].Endpoints)
}

// FindPort locates the container port for the given pod and portName.  If the
// targetPort is a number, use that.  If the targetPort is a string, look that
// string up in all named ports in all containers in the target pod.  If no
// match is found, fail.
func FindPort(pod *v1.Pod, svcPort *v1.ServicePort) (int, error) {
	portName := svcPort.TargetPort
	switch portName.Type {
	case intstr.String:
		name := portName.StrVal
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if port.Name == name && port.Protocol == svcPort.Protocol {
					return int(port.ContainerPort), nil
				}
			}
		}
		// also support sidecar container (initContainer with restartPolicy=Always)
		for _, container := range pod.Spec.InitContainers {
			if container.RestartPolicy == nil || *container.RestartPolicy != v1.ContainerRestartPolicyAlways {
				continue
			}
			for _, port := range container.Ports {
				if port.Name == name && port.Protocol == svcPort.Protocol {
					return int(port.ContainerPort), nil
				}
			}
		}
	case intstr.Int:
		return portName.IntValue(), nil
	}

	return 0, fmt.Errorf("no suitable port for manifest: %s", pod.UID)
}
