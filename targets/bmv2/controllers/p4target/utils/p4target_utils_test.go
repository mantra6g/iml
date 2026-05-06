package utils

import (
	"net"
	"testing"
)

func TestFilterIPs(t *testing.T) {
	tests := []struct {
		name     string
		input    []net.IP
		expected []string
	}{
		{
			name:     "empty input",
			input:    []net.IP{},
			expected: []string{},
		},
		{
			name:     "valid IPv4 addresses",
			input:    []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("10.0.0.1")},
			expected: []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:     "valid IPv6 addresses",
			input:    []net.IP{net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")},
			expected: []string{"2001:db8::1", "2001:db8::2"},
		},
		{
			name:     "loopback IPv4 filtered",
			input:    []net.IP{net.ParseIP("127.0.0.1")},
			expected: []string{},
		},
		{
			name:     "loopback IPv6 filtered",
			input:    []net.IP{net.ParseIP("::1")},
			expected: []string{},
		},
		{
			name:     "unspecified IPv6 filtered",
			input:    []net.IP{net.ParseIP("::")},
			expected: []string{},
		},
		{
			name:     "link-local IPv4 filtered",
			input:    []net.IP{net.ParseIP("169.254.1.1")},
			expected: []string{},
		},
		{
			name:     "link-local IPv6 filtered",
			input:    []net.IP{net.ParseIP("fe80::1")},
			expected: []string{},
		},
		{
			name:     "link-local IPv6 address (fe80::5469:97ff:fee6:939d) filtered",
			input:    []net.IP{net.ParseIP("fe80::5469:97ff:fee6:939d")},
			expected: []string{},
		},
		{
			name:     "IPv4 multicast filtered",
			input:    []net.IP{net.ParseIP("224.0.0.1")},
			expected: []string{},
		},
		{
			name:     "IPv6 multicast filtered",
			input:    []net.IP{net.ParseIP("ff02::1")},
			expected: []string{},
		},
		{
			name:     "mixed valid and invalid IPs",
			input:    []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1"), net.ParseIP("::1")},
			expected: []string{"192.168.1.1", "10.0.0.1"},
		},
		{
			name:     "all invalid IPs",
			input:    []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1"), net.ParseIP("224.0.0.1")},
			expected: []string{},
		},
		{
			name:     "IPv6 interface-local multicast filtered",
			input:    []net.IP{net.ParseIP("ff01::1")},
			expected: []string{},
		},
		{
			name: "large IPv4 address range",
			input: []net.IP{
				net.ParseIP("172.16.0.1"),
				net.ParseIP("192.0.2.1"),
				net.ParseIP("203.0.113.1"),
			},
			expected: []string{"172.16.0.1", "192.0.2.1", "203.0.113.1"},
		},
		{
			name: "private IPv4 addresses",
			input: []net.IP{
				net.ParseIP("10.0.0.1"),
				net.ParseIP("172.16.0.1"),
				net.ParseIP("192.168.0.1"),
			},
			expected: []string{"10.0.0.1", "172.16.0.1", "192.168.0.1"},
		},
		{
			name:     "IPv6 global unicast",
			input:    []net.IP{net.ParseIP("2001:db8::8a2e:370:7334")},
			expected: []string{"2001:db8::8a2e:370:7334"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterIPs(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("FilterIPs() returned %d IPs, expected %d", len(result), len(tt.expected))
			}

			// Check if all expected IPs are in the result
			for i, exp := range tt.expected {
				if i >= len(result) {
					t.Errorf("FilterIPs() mismatch at index %d: got fewer results than expected", i)
					return
				}

				if result[i] != exp {
					t.Errorf("FilterIPs() mismatch at index %d: got %s, expected %s", i, result[i], exp)
					return
				}
			}
		})
	}
}
