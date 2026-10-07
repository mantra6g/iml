package coredns

import (
	"strings"
	"testing"
)

const kubeadmCorefile = `.:53 {
    errors
    health {
       lameduck 5s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
       ttl 30
    }
    prometheus :9153
    forward . /etc/resolv.conf {
       max_concurrent 1000
    }
    cache 30
    loop
    reload
    loadbalance
}
`

func TestBlock(t *testing.T) {
	want := `# BEGIN loom-dns (managed by loom-dns, do not edit)
loom.local:53 {
    errors
    cache 5
    forward . 10.96.0.53
}
# END loom-dns
`
	if got := Block("loom.local.", "10.96.0.53", 5); got != want {
		t.Errorf("Block() =\n%s\nwant\n%s", got, want)
	}
	if got := Block("loom.local", "10.96.0.53", 0); strings.Contains(got, "cache") {
		t.Errorf("Block() with zero ttl has a cache plugin:\n%s", got)
	}
}

func TestUpsert(t *testing.T) {
	block := Block("loom.local", "10.96.0.53", 5)
	oldBlock := Block("loom.local", "10.96.0.99", 5)

	tests := []struct {
		name     string
		corefile string
		want     string
		changed  bool
	}{
		{"appends to stock Corefile", kubeadmCorefile, kubeadmCorefile + block, true},
		{"already up to date", kubeadmCorefile + block, kubeadmCorefile + block, false},
		{"replaces stale block", kubeadmCorefile + oldBlock, kubeadmCorefile + block, true},
		{"moves block to the end and keeps surrounding text",
			kubeadmCorefile + oldBlock + "example.org:53 {\n    whoami\n}\n",
			kubeadmCorefile + "example.org:53 {\n    whoami\n}\n" + block, true},
		{"adds missing trailing newline", strings.TrimSuffix(kubeadmCorefile, "\n"), kubeadmCorefile + block, true},
		{"empty Corefile", "", block, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := Upsert(tt.corefile, block)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Upsert() =\n%s\nwant\n%s", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("Upsert() changed = %v, want %v", changed, tt.changed)
			}
			again, changed, err := Upsert(got, block)
			if err != nil || changed || again != got {
				t.Errorf("Upsert() is not idempotent: changed = %v, err = %v", changed, err)
			}
		})
	}
}

func TestUpsertMalformedMarkers(t *testing.T) {
	block := Block("loom.local", "10.96.0.53", 5)
	for name, corefile := range map[string]string{
		"begin without end":  kubeadmCorefile + beginMarker + "\n",
		"end without begin":  kubeadmCorefile + endMarker + "\n",
		"two begin markers":  kubeadmCorefile + beginMarker + "\n" + block,
		"end before begin":   kubeadmCorefile + endMarker + "\n" + block,
		"two managed blocks": kubeadmCorefile + block + block,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Upsert(corefile, block); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
