package vrf

import "testing"

func TestIsInvalidDropRule(t *testing.T) {
	tests := []struct {
		name string
		rule string
		want bool
	}{
		{"kube-proxy nfacct invalid drop",
			"-A KUBE-FORWARD -m conntrack --ctstate INVALID -m nfacct --nfacct-name ct_state_invalid_dropped_pkts -j DROP", true},
		{"older kube-proxy invalid drop",
			"-A KUBE-FORWARD -m conntrack --ctstate INVALID -j DROP", true},
		{"chain declaration", "-N KUBE-FORWARD", false},
		{"forwarding mark accept",
			`-A KUBE-FORWARD -m comment --comment "kubernetes forwarding rules" -m mark --mark 0x4000/0x4000 -j ACCEPT`, false},
		{"established accept",
			"-A KUBE-FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidDropRule(tt.rule); got != tt.want {
				t.Errorf("isInvalidDropRule(%q) = %v, want %v", tt.rule, got, tt.want)
			}
		})
	}
}
