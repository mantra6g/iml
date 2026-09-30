package loomservice

import (
	"strings"
	"testing"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

const (
	testName      = "web"
	testNamespace = "default"
)

func TestKubeServiceName(t *testing.T) {
	if got := KubeServiceName(testNamespace, testName); got != "web--default" {
		t.Errorf("KubeServiceName() = %q, want %q", got, "web--default")
	}

	longName := strings.Repeat("a", 40)
	longNamespace := strings.Repeat("b", 40)
	got := KubeServiceName(longNamespace, longName)
	if errs := validation.IsDNS1035Label(got); len(errs) > 0 {
		t.Errorf("KubeServiceName() = %q is not a DNS-1035 label: %v", got, errs)
	}
	if got == KubeServiceName(longNamespace+"c", longName) {
		t.Errorf("KubeServiceName() collides for different namespaces: %q", got)
	}
	if got != KubeServiceName(longNamespace, longName) {
		t.Errorf("KubeServiceName() is not deterministic")
	}

	// Truncating right after a dash must not leave a trailing dash before the hash.
	dashed := KubeServiceName(strings.Repeat("c", 40), strings.Repeat("d", 51)+"-x")
	if errs := validation.IsDNS1035Label(dashed); len(errs) > 0 {
		t.Errorf("KubeServiceName() = %q is not a DNS-1035 label: %v", dashed, errs)
	}
}

func TestHostname(t *testing.T) {
	want := "web--default.loom-system.svc.cluster.local"
	if got := Hostname("web--default"); got != want {
		t.Errorf("Hostname() = %q, want %q", got, want)
	}
}

func TestOwnerLabels(t *testing.T) {
	svc := &corev1alpha1.Service{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: testNamespace}}
	labels := OwnerLabels(svc)
	key, ok := OwnerKeyFromLabels(labels)
	if !ok || key.Name != testName || key.Namespace != testNamespace {
		t.Errorf("OwnerKeyFromLabels() = %v, %v", key, ok)
	}
	if !IsOwnedBy(labels, svc) {
		t.Errorf("IsOwnedBy() = false, want true")
	}
	other := &corev1alpha1.Service{ObjectMeta: metav1.ObjectMeta{Name: testName, Namespace: "other"}}
	if IsOwnedBy(labels, other) {
		t.Errorf("IsOwnedBy() = true for a different Service")
	}
	if _, ok := OwnerKeyFromLabels(map[string]string{corev1alpha1.ServiceNameLabel: testName}); ok {
		t.Errorf("OwnerKeyFromLabels() accepted labels without a namespace")
	}
}

func TestLoomNetworkStatus(t *testing.T) {
	tests := []struct {
		name       string
		annotation *string
		wantIPs    []string
	}{
		{name: "no annotation"},
		{name: "invalid json", annotation: new("{invalid")},
		{name: "empty list", annotation: new("[]")},
		{
			name:       "other network only",
			annotation: new(`[{"name":"cbr0","ips":["10.0.0.4"]}]`),
		},
		{
			name: "list with loom-cni",
			annotation: new(`[{"name":"cbr0","ips":["10.0.0.4"]},` +
				`{"name":"loom-cni","ips":["10.1.0.4","fd00::4"]}]`),
			wantIPs: []string{"10.1.0.4", "fd00::4"},
		},
		{
			name:       "namespaced loom-cni",
			annotation: new(`[{"name":"loom-system/loom-cni","ips":["fd00::5"]}]`),
			wantIPs:    []string{"fd00::5"},
		},
		{
			name:       "single object",
			annotation: new(`{"name":"loom-cni","ips":["fd00::6"]}`),
			wantIPs:    []string{"fd00::6"},
		},
		{
			name:       "similar network name",
			annotation: new(`[{"name":"loom-system/loom-cni-2","ips":["fd00::7"]}]`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{}
			if tt.annotation != nil {
				pod.Annotations = map[string]string{netdefv1.NetworkStatusAnnot: *tt.annotation}
			}
			status, ok := LoomNetworkStatus(pod)
			if ok != (tt.wantIPs != nil) {
				t.Fatalf("LoomNetworkStatus() ok = %v, want %v", ok, tt.wantIPs != nil)
			}
			if !ok {
				return
			}
			if strings.Join(status.IPs, ",") != strings.Join(tt.wantIPs, ",") {
				t.Errorf("LoomNetworkStatus() IPs = %v, want %v", status.IPs, tt.wantIPs)
			}
		})
	}
}
