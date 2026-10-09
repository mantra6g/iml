package loomservice

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/validation"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

// nameSeparator joins the loom Service name and namespace into the Kubernetes Service name.
// Dots are not allowed in Service names, so "<name>.<namespace>" becomes "<name>--<namespace>".
const nameSeparator = "--"

// KubeServiceName returns the name of the Kubernetes Service generated for the loom Service
// <namespace>/<name>. Names longer than a DNS-1035 label are truncated and suffixed with a hash
// of the full name so that they stay unique.
func KubeServiceName(namespace, name string) string {
	fullName := name + nameSeparator + namespace
	if len(fullName) <= validation.DNS1035LabelMaxLength {
		return fullName
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(namespace + "/" + name))
	suffix := rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
	prefix := strings.TrimRight(fullName[:validation.DNS1035LabelMaxLength-len(suffix)-1], "-")
	return prefix + "-" + suffix
}

// Hostname returns the fully qualified DNS name of the generated Kubernetes Service.
func Hostname(kubeServiceName string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", kubeServiceName, corev1alpha1.ServiceTargetNamespace)
}

// OwnerLabels returns the labels that link a generated object back to its loom Service.
func OwnerLabels(svc *corev1alpha1.Service) map[string]string {
	return map[string]string{
		corev1alpha1.ServiceNameLabel:      svc.Name,
		corev1alpha1.ServiceNamespaceLabel: svc.Namespace,
	}
}

// OwnerKeyFromLabels returns the key of the loom Service referenced by the given labels, if any.
func OwnerKeyFromLabels(labels map[string]string) (types.NamespacedName, bool) {
	name, hasName := labels[corev1alpha1.ServiceNameLabel]
	namespace, hasNamespace := labels[corev1alpha1.ServiceNamespaceLabel]
	if !hasName || !hasNamespace || name == "" || namespace == "" {
		return types.NamespacedName{}, false
	}
	return types.NamespacedName{Namespace: namespace, Name: name}, true
}

// IsOwnedBy returns true if the labels reference the given loom Service.
func IsOwnedBy(labels map[string]string, svc *corev1alpha1.Service) bool {
	key, ok := OwnerKeyFromLabels(labels)
	return ok && key.Name == svc.Name && key.Namespace == svc.Namespace
}

// LoomNetworkStatus returns the loom-cni entry of the pod's multus network-status annotation.
func LoomNetworkStatus(pod *corev1.Pod) (*netdefv1.NetworkStatus, bool) {
	annotation, ok := pod.GetAnnotations()[netdefv1.NetworkStatusAnnot]
	if !ok || annotation == "" {
		return nil, false
	}

	// Multus writes a list of statuses, but tolerate a single object as well.
	var statuses []netdefv1.NetworkStatus
	if err := json.Unmarshal([]byte(annotation), &statuses); err != nil {
		status := netdefv1.NetworkStatus{}
		if err := json.Unmarshal([]byte(annotation), &status); err != nil {
			return nil, false
		}
		statuses = []netdefv1.NetworkStatus{status}
	}

	for i := range statuses {
		if isLoomNetwork(statuses[i].Name) {
			return &statuses[i], true
		}
	}
	return nil, false
}

// isLoomNetwork matches both "loom-cni" and namespaced "<namespace>/loom-cni" network names.
func isLoomNetwork(name string) bool {
	if name == corev1alpha1.LoomCNINetworkName {
		return true
	}
	_, networkName, found := strings.Cut(name, "/")
	return found && networkName == corev1alpha1.LoomCNINetworkName
}
