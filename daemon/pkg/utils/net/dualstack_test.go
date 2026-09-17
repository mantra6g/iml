package netutils

import "testing"

func TestParseDualStackNetworkFromStrings_DualStack(t *testing.T) {
	result, err := ParseDualStackNetworkFromStrings([]string{"10.123.1.0/24", "fd00:0:0:1::/64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IPv4Net == nil || result.IPv4Net.String() != "10.123.1.0/24" {
		t.Errorf("unexpected IPv4Net: %v", result.IPv4Net)
	}
	if result.IPv6Net == nil || result.IPv6Net.String() != "fd00:0:0:1::/64" {
		t.Errorf("unexpected IPv6Net: %v", result.IPv6Net)
	}
}

func TestParseDualStackAddressListFromStrings_DualStack(t *testing.T) {
	result, err := ParseDualStackAddressListFromStrings([]string{"10.123.1.5", "fd00:0:0:1::5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.IPv4Addresses) != 1 || result.IPv4Addresses[0].String() != "10.123.1.5" {
		t.Errorf("unexpected IPv4Addresses: %v", result.IPv4Addresses)
	}
	if len(result.IPv6Addresses) != 1 || result.IPv6Addresses[0].String() != "fd00:0:0:1::5" {
		t.Errorf("unexpected IPv6Addresses: %v", result.IPv6Addresses)
	}
}
