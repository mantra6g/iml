package ipam

import (
	"net/netip"
	"testing"
)

func TestAddrAllocator_New(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"valid ipv4", "10.0.0.0/24", false},
		{"valid ipv6", "2001:db8::/32", false},
		{"unmasked prefix", "10.0.0.1/24", true},
		{"invalid prefix", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, _ := netip.ParsePrefix(tt.prefix)
			_, err := NewAddrAllocator(prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAddrAllocator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddrAllocator_Next(t *testing.T) {
	// A /30 network contains 4 IPs: .0 (net), .1, .2 (usable hosts), .3 (broadcast)
	base := netip.MustParsePrefix("192.168.1.0/30")
	alloc, err := NewAddrAllocator(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1st call -> expecting 192.168.1.1
	ip, err := alloc.Next()
	if err != nil || ip.String() != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %v (err: %v)", ip, err)
	}

	// 2nd call -> expecting 192.168.1.2
	ip, err = alloc.Next()
	if err != nil || ip.String() != "192.168.1.2" {
		t.Errorf("expected 192.168.1.2, got %v (err: %v)", ip, err)
	}

	// 3rd call -> should error out (no more usable IPs)
	_, err = alloc.Next()
	if err == nil {
		t.Errorf("expected error when exhausting IP pool, got nil")
	}
}

func TestPrefixAllocator_New(t *testing.T) {
	base := netip.MustParsePrefix("10.0.0.0/16")

	tests := []struct {
		name      string
		prefix    netip.Prefix
		subnetLen int
		wantErr   bool
	}{
		{"valid subnet len", base, 24, false},
		{"subnet len equal to base", base, 16, true},
		{"subnet len too large for ipv4", base, 33, true},
		{"unmasked base", netip.MustParsePrefix("10.0.0.1/16"), 24, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPrefixAllocator(tt.prefix, tt.subnetLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPrefixAllocator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPrefixAllocator_Next(t *testing.T) {
	// A /23 contains exactly two /24 subnets
	base := netip.MustParsePrefix("10.1.0.0/23")
	alloc, err := NewPrefixAllocator(base, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1st subnet
	p1, err := alloc.Next()
	if err != nil || p1.String() != "10.1.0.0/24" {
		t.Errorf("expected 10.1.0.0/24, got %v (err: %v)", p1, err)
	}

	// 2nd subnet
	p2, err := alloc.Next()
	if err != nil || p2.String() != "10.1.1.0/24" {
		t.Errorf("expected 10.1.1.0/24, got %v (err: %v)", p2, err)
	}

	// Exhausted pool
	_, err = alloc.Next()
	if err == nil {
		t.Errorf("expected error when exhausting prefix pool, got nil")
	}
}

func TestBroadcastAddr(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected string
	}{
		{"ipv4 /24", "192.168.1.0/24", "192.168.1.255"},
		{"ipv4 /30", "10.0.0.0/30", "10.0.0.3"},
		{"ipv4 /23", "10.1.0.0/23", "10.1.1.255"},
		{"ipv6 /126", "fd00::/126", "fd00::3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := netip.MustParsePrefix(tt.prefix)
			got := broadcastAddr(p)
			if got.String() != tt.expected {
				t.Errorf("broadcastAddr() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPrefixAllocator_IPv6(t *testing.T) {
	// Allocating /64 subnets from a /48 IPv6 block
	base := netip.MustParsePrefix("2001:db8::/48")
	alloc, err := NewPrefixAllocator(base, 64)
	if err != nil {
		t.Fatalf("failed to create IPv6 allocator: %v", err)
	}

	tests := []string{
		"2001:db8::/64",
		"2001:db8:0:1::/64",
		"2001:db8:0:2::/64",
	}

	for _, expected := range tests {
		got, err := alloc.Next()
		if err != nil || got.String() != expected {
			t.Errorf("expected %s, got %v (err: %v)", expected, got, err)
		}
	}
}

func TestBroadcastAddr_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected string
	}{
		{"IPv4 /32 (Single IP)", "1.1.1.1/32", "1.1.1.1"},
		{"IPv4 /0 (Internet)", "0.0.0.0/0", "255.255.255.255"},
		{"IPv6 /128", "2001:db8::1/128", "2001:db8::1"},
		{"IPv6 /0", "::/0", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"IPv4 Subnet Boundary", "10.0.0.0/25", "10.0.0.127"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := netip.MustParsePrefix(tt.prefix)
			got := broadcastAddr(p)
			if got.String() != tt.expected {
				t.Errorf("%s: got %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestPrefixAllocator_Boundary(t *testing.T) {
	// Test requesting exactly the same size as the base (should fail in New)
	base := netip.MustParsePrefix("10.0.0.0/24")
	_, err := NewPrefixAllocator(base, 24)
	if err == nil {
		t.Error("expected error when subnet size equals base size")
	}

	// Test requesting a /32 from a /31 (Exactly two IPs)
	base31 := netip.MustParsePrefix("10.0.0.0/31")
	alloc, _ := NewPrefixAllocator(base31, 32)

	p1, _ := alloc.Next()
	if p1.String() != "10.0.0.0/32" {
		t.Errorf("expected 10.0.0.0/32, got %s", p1)
	}

	p2, _ := alloc.Next()
	if p2.String() != "10.0.0.1/32" {
		t.Errorf("expected 10.0.0.1/32, got %s", p2)
	}

	_, err = alloc.Next()
	if err == nil {
		t.Error("expected exhaustion error")
	}
}
