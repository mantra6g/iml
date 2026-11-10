package reconcilers

import (
	"net/netip"
	"reflect"
	"testing"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var allFirewallTables = []string{
	FirewallSrcIPv4Table, FirewallDstIPv4Table, FirewallSrcIPv6Table, FirewallDstIPv6Table,
}

func newApp(name string, subnets map[string][]corev1alpha1.DualStackNetwork) *corev1alpha1.Application {
	return &corev1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     corev1alpha1.ApplicationStatus{Subnets: subnets},
	}
}

// tablePrefixes flattens a table's entries into "addr/len" strings, checking the field, match type and action.
func tablePrefixes(t *testing.T, cfg corev1alpha1.TableConfig, wantField string) []string {
	t.Helper()
	out := make([]string, 0, len(cfg.Entries))
	for i, entry := range cfg.Entries {
		if len(entry.MatchFields) != 1 {
			t.Fatalf("entry %d MatchFields count = %d, want 1", i, len(entry.MatchFields))
		}
		mf := entry.MatchFields[0]
		if mf.Name != wantField {
			t.Errorf("entry %d field = %q, want %q", i, mf.Name, wantField)
		}
		if mf.Type != corev1alpha1.LPMMatch || mf.LPM == nil {
			t.Fatalf("entry %d is not an LPM match", i)
		}
		if entry.Action.Name != AllowAction {
			t.Errorf("entry %d action = %q, want %q", i, entry.Action.Name, AllowAction)
		}
		var addr string
		switch {
		case mf.LPM.Value.IPv4Address != nil:
			addr = *mf.LPM.Value.IPv4Address
		case mf.LPM.Value.IPv6Address != nil:
			addr = *mf.LPM.Value.IPv6Address
		default:
			t.Fatalf("entry %d LPM value has no address", i)
		}
		out = append(out, addr+"/"+mf.LPM.PrefixLen)
	}
	return out
}

func TestBuildFirewallTables_Subnets(t *testing.T) {
	src := newApp("web-client", map[string][]corev1alpha1.DualStackNetwork{
		"node-a": {{IPv4Net: "10.1.0.0/24", IPv6Net: "fd00:1::/64"}},
		"node-b": {{IPv4Net: "10.1.1.0/24"}},
	})
	dst := newApp("web-server", map[string][]corev1alpha1.DualStackNetwork{
		"node-c": {{IPv4Net: "10.2.0.0/16", IPv6Net: "fd00:2::/64"}},
	})

	tables, err := BuildFirewallTables(src, dst)
	if err != nil {
		t.Fatalf("BuildFirewallTables() error = %v", err)
	}
	if len(tables) != len(allFirewallTables) {
		t.Fatalf("table count = %d, want %d", len(tables), len(allFirewallTables))
	}

	tests := []struct {
		table string
		field string
		want  []string
	}{
		{FirewallSrcIPv4Table, "hdr.inner_ipv4.src_addr", []string{"10.1.0.0/24", "10.1.1.0/24"}},
		{FirewallDstIPv4Table, "hdr.inner_ipv4.dst_addr", []string{"10.2.0.0/16"}},
		{FirewallSrcIPv6Table, "hdr.inner_ipv6.src_addr", []string{"fd00:1::/64"}},
		{FirewallDstIPv6Table, "hdr.inner_ipv6.dst_addr", []string{"fd00:2::/64"}},
	}
	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			got := tablePrefixes(t, tables[tt.table], tt.field)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entries = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildFirewallTables_MissingApp(t *testing.T) {
	app := newApp("web-server", map[string][]corev1alpha1.DualStackNetwork{
		"node-c": {{IPv4Net: "10.2.0.0/24", IPv6Net: "fd00:2::/64"}},
	})

	tests := []struct {
		name       string
		src, dst   *corev1alpha1.Application
		emptyTable []string
	}{
		{"missing source", nil, app, []string{FirewallSrcIPv4Table, FirewallSrcIPv6Table}},
		{"missing destination", app, nil, []string{FirewallDstIPv4Table, FirewallDstIPv6Table}},
		{"both missing", nil, nil, allFirewallTables},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables, err := BuildFirewallTables(tt.src, tt.dst)
			if err != nil {
				t.Fatalf("BuildFirewallTables() error = %v", err)
			}
			for _, table := range allFirewallTables {
				if _, ok := tables[table]; !ok {
					t.Fatalf("table %q missing from result", table)
				}
			}
			for _, table := range tt.emptyTable {
				if n := len(tables[table].Entries); n != 0 {
					t.Errorf("table %q entry count = %d, want 0", table, n)
				}
			}
		})
	}
}

func TestBuildFirewallTables_DuplicateAndInvalidSubnets(t *testing.T) {
	src := newApp("web-client", map[string][]corev1alpha1.DualStackNetwork{
		"node-a": {{IPv4Net: "10.1.0.0/24"}, {IPv4Net: "not-a-cidr"}},
		// Same subnet reported by another node, written with host bits set
		"node-b": {{IPv4Net: "10.1.0.9/24"}},
	})
	dst := newApp("web-server", map[string][]corev1alpha1.DualStackNetwork{
		"node-c": {{IPv4Net: "10.2.0.0/24", IPv6Net: "garbage"}},
	})

	tables, err := BuildFirewallTables(src, dst)
	if err != nil {
		t.Fatalf("BuildFirewallTables() error = %v", err)
	}
	if got := tablePrefixes(t, tables[FirewallSrcIPv4Table], "hdr.inner_ipv4.src_addr"); !reflect.DeepEqual(got, []string{"10.1.0.0/24"}) {
		t.Errorf("source IPv4 entries = %v, want [10.1.0.0/24]", got)
	}
	if n := len(tables[FirewallDstIPv6Table].Entries); n != 0 {
		t.Errorf("destination IPv6 entry count = %d, want 0", n)
	}
}

func TestBuildFirewallTables_DeterministicOrder(t *testing.T) {
	subnets := make(map[string][]corev1alpha1.DualStackNetwork)
	for i, n := range []string{"node-a", "node-b", "node-c", "node-d", "node-e"} {
		subnets[n] = []corev1alpha1.DualStackNetwork{{
			IPv4Net: netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 1, byte(i), 0}), 24).String(),
		}}
	}
	src := newApp("web-client", subnets)
	dst := newApp("web-server", map[string][]corev1alpha1.DualStackNetwork{
		"node-c": {{IPv4Net: "10.2.0.0/24"}},
	})

	first, err := BuildFirewallTables(src, dst)
	if err != nil {
		t.Fatalf("BuildFirewallTables() error = %v", err)
	}
	for range 20 {
		next, err := BuildFirewallTables(src, dst)
		if err != nil {
			t.Fatalf("BuildFirewallTables() error = %v", err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("BuildFirewallTables() output changed between calls")
		}
	}
}

func TestBuildFirewallTables_TooManySubnets(t *testing.T) {
	srcSubnets := make([]corev1alpha1.DualStackNetwork, 0, MaxFirewallEntries+1)
	for i := range MaxFirewallEntries + 1 {
		srcSubnets = append(srcSubnets, corev1alpha1.DualStackNetwork{
			IPv4Net: netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(i / 256), byte(i % 256), 0}), 24).String(),
		})
	}
	src := newApp("web-client", map[string][]corev1alpha1.DualStackNetwork{"node-a": srcSubnets})

	if _, err := BuildFirewallTables(src, nil); err == nil {
		t.Fatalf("BuildFirewallTables() expected an error for %d subnets", MaxFirewallEntries+1)
	}
}
