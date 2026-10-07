package ipam

import (
	"errors"
	"net/netip"
	"testing"

	"k8s.io/apimachinery/pkg/types"
)

var (
	ownerA = types.NamespacedName{Namespace: "ns", Name: "a"}
	ownerB = types.NamespacedName{Namespace: "ns", Name: "b"}
	ownerC = types.NamespacedName{Namespace: "ns", Name: "c"}
)

func newTestPool(t *testing.T, prefix string) *AddrPool {
	t.Helper()
	pool, err := NewAddrPool(netip.MustParsePrefix(prefix))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return pool
}

func mustAllocate(t *testing.T, pool *AddrPool, owner types.NamespacedName) netip.Addr {
	t.Helper()
	addr, err := pool.Allocate(owner)
	if err != nil {
		t.Fatalf("Allocate(%v) failed: %v", owner, err)
	}
	return addr
}

func TestAddrPool_New(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"valid ipv4", "10.112.0.0/16", false},
		{"valid ipv6", "fd02::/112", false},
		{"unmasked prefix", "10.112.0.1/16", true},
		{"invalid prefix", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, _ := netip.ParsePrefix(tt.prefix)
			_, err := NewAddrPool(prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAddrPool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddrPool_AllocateIsIdempotent(t *testing.T) {
	pool := newTestPool(t, "10.112.0.0/16")

	first := mustAllocate(t, pool, ownerA)
	if first.String() != "10.112.0.1" {
		t.Errorf("expected 10.112.0.1, got %v", first)
	}
	if again := mustAllocate(t, pool, ownerA); again != first {
		t.Errorf("expected the same address %v, got %v", first, again)
	}
	if other := mustAllocate(t, pool, ownerB); other == first {
		t.Errorf("expected a different address than %v", first)
	}
}

func TestAddrPool_Exhaustion(t *testing.T) {
	// A /30 has two usable addresses: .1 and .2. The network (.0) and broadcast (.3)
	// addresses are never handed out.
	pool := newTestPool(t, "192.168.1.0/30")

	got := map[string]bool{
		mustAllocate(t, pool, ownerA).String(): true,
		mustAllocate(t, pool, ownerB).String(): true,
	}
	if !got["192.168.1.1"] || !got["192.168.1.2"] {
		t.Errorf("expected 192.168.1.1 and 192.168.1.2, got %v", got)
	}

	_, err := pool.Allocate(ownerC)
	if !errors.Is(err, ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestAddrPool_ReleaseAndReuse(t *testing.T) {
	pool := newTestPool(t, "192.168.1.0/30")

	a := mustAllocate(t, pool, ownerA)
	mustAllocate(t, pool, ownerB)
	pool.Release(ownerA)

	if c := mustAllocate(t, pool, ownerC); c != a {
		t.Errorf("expected released address %v to be reused, got %v", a, c)
	}
}

func TestAddrPool_NextFit(t *testing.T) {
	pool := newTestPool(t, "10.112.0.0/24")

	a := mustAllocate(t, pool, ownerA)
	pool.Release(ownerA)

	// The released address is only reused once the rest of the pool has been tried.
	if b := mustAllocate(t, pool, ownerB); b == a {
		t.Errorf("expected a fresh address, got the just released %v", b)
	}
}

func TestAddrPool_Reserve(t *testing.T) {
	pool := newTestPool(t, "10.112.0.0/16")
	addr := netip.MustParseAddr("10.112.3.4")

	if err := pool.Reserve(addr, ownerA); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}
	if err := pool.Reserve(addr, ownerA); err != nil {
		t.Errorf("reserving the same address twice for the same owner should succeed: %v", err)
	}
	if err := pool.Reserve(addr, ownerB); err == nil {
		t.Error("expected an error reserving an address held by another owner")
	}
	if got := mustAllocate(t, pool, ownerA); got != addr {
		t.Errorf("expected Allocate to return the reserved address %v, got %v", addr, got)
	}

	// Reserving a new address for an owner releases its previous one.
	other := netip.MustParseAddr("10.112.3.5")
	if err := pool.Reserve(other, ownerA); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}
	if err := pool.Reserve(addr, ownerB); err != nil {
		t.Errorf("expected the previous address to be released: %v", err)
	}
}

func TestAddrPool_ReserveRejectsUnusableAddresses(t *testing.T) {
	pool := newTestPool(t, "10.112.0.0/16")

	for _, addr := range []string{"10.96.0.10", "10.112.0.0", "10.112.255.255"} {
		if err := pool.Reserve(netip.MustParseAddr(addr), ownerA); err == nil {
			t.Errorf("expected an error reserving %s", addr)
		}
	}
}

func TestAddrPool_IPv6(t *testing.T) {
	// IPv6 has no broadcast address, so the last address of the prefix is usable.
	pool := newTestPool(t, "fd02::/127")

	if addr := mustAllocate(t, pool, ownerA); addr.String() != "fd02::1" {
		t.Errorf("expected fd02::1, got %v", addr)
	}
	if _, err := pool.Allocate(ownerB); !errors.Is(err, ErrPoolExhausted) {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}
