package vrf

import (
	"slices"
	"testing"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewP4TargetLinks(t *testing.T) {
	target := func(network *corev1alpha1.P4TargetNetwork) *corev1alpha1.P4Target {
		return &corev1alpha1.P4Target{
			ObjectMeta: metav1.ObjectMeta{Name: "bf3-node1"},
			Spec:       corev1alpha1.P4TargetSpec{Network: network},
		}
	}
	prefix := p4TargetLinkPrefix("bf3-node1")

	tests := []struct {
		name    string
		network *corev1alpha1.P4TargetNetwork
		want    p4TargetLinks
	}{
		{"no network: single iml0", nil,
			p4TargetLinks{BridgeVeth: prefix + "b"}},
		{"no host interfaces: single iml0", &corev1alpha1.P4TargetNetwork{},
			p4TargetLinks{BridgeVeth: prefix + "b"}},
		{"create without count", &corev1alpha1.P4TargetNetwork{HostInterfaces: &corev1alpha1.HostInterfaces{
			Strategy: corev1alpha1.HostInterfaceStrategyCreate,
		}}, p4TargetLinks{BridgeVeth: prefix + "b", Dispatcher: prefix + "d", Ports: []string{prefix + "p0"}}},
		{"create", &corev1alpha1.P4TargetNetwork{HostInterfaces: &corev1alpha1.HostInterfaces{
			Strategy: corev1alpha1.HostInterfaceStrategyCreate,
			Create:   &corev1alpha1.CreateHostInterfaces{Count: 2},
		}}, p4TargetLinks{BridgeVeth: prefix + "b", Dispatcher: prefix + "d", Ports: []string{prefix + "p0", prefix + "p1"}}},
		{"reuse", &corev1alpha1.P4TargetNetwork{HostInterfaces: &corev1alpha1.HostInterfaces{
			Strategy: corev1alpha1.HostInterfaceStrategyReuse,
			Reuse:    &corev1alpha1.ReuseHostInterfaces{Names: []string{"nf0", "nf1"}},
		}}, p4TargetLinks{BridgeVeth: prefix + "b", Dispatcher: prefix + "d", Ports: []string{"nf0", "nf1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newP4TargetLinks(target(tt.network))
			if err != nil {
				t.Fatal(err)
			}
			if got.BridgeVeth != tt.want.BridgeVeth || got.Dispatcher != tt.want.Dispatcher || !slices.Equal(got.Ports, tt.want.Ports) {
				t.Fatalf("got %+v, want %+v", *got, tt.want)
			}
			for _, name := range append([]string{got.BridgeVeth, got.Dispatcher}, got.Ports...) {
				if len(name) > maxIfaceNameLen {
					t.Errorf("interface name %q is longer than %d", name, maxIfaceNameLen)
				}
			}
		})
	}

	t.Run("reuse without names", func(t *testing.T) {
		_, err := newP4TargetLinks(target(&corev1alpha1.P4TargetNetwork{HostInterfaces: &corev1alpha1.HostInterfaces{
			Strategy: corev1alpha1.HostInterfaceStrategyReuse,
		}}))
		if err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("names are stable and distinct per target", func(t *testing.T) {
		if p4TargetLinkPrefix("bf3-node1") != prefix {
			t.Fatal("prefix changed between calls")
		}
		if p4TargetLinkPrefix("bf3-node2") == prefix {
			t.Fatal("different targets share a prefix")
		}
	})
}
