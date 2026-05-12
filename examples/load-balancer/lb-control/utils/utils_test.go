package utils

import (
	"net"
	"testing"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Sample network status JSON data for testing
const (
	// This is the actual format returned by Multus - it's a JSON array of network statuses
	// but GetMultusNetworkStatus tries to parse a single object, so we test individual network interfaces
	networkStatusWithIPv4AndIPv6 = `{
		"name": "loom-system/loom-cni",
		"interface": "iml0",
		"ips": [
			"10.123.0.9",
			"fd00::2:0:9"
		],
		"dns": {}
	}`

	networkStatusIPv4Only = `{
		"name": "test-network",
		"interface": "eth1",
		"ips": [
			"192.168.1.100"
		],
		"dns": {}
	}`

	networkStatusIPv6Only = `{
		"name": "test-network-v6",
		"interface": "eth2",
		"ips": [
			"2001:db8::1"
		],
		"dns": {}
	}`

	networkStatusNoIPs = `{
		"name": "test-network-no-ip",
		"interface": "eth3",
		"ips": [],
		"dns": {}
	}`

	networkStatusInvalidJSON = `{invalid json}`

	// Array format tests - this is what Multus actually returns
	networkStatusArrayFormat = `[{
		"name": "kindnet",
		"interface": "eth0",
		"ips": [
			"10.244.0.40"
		],
		"mac": "2a:40:4c:b0:f2:41",
		"default": true,
		"dns": {},
		"gateway": [
			"<nil>"
		]
	},{
		"name": "loom-system/loom-cni",
		"interface": "iml0",
		"ips": [
			"10.123.0.9",
			"fd00::2:0:9"
		],
		"dns": {}
	}]`

	networkStatusArraySingleEntry = `[{
		"name": "test-network",
		"interface": "eth1",
		"ips": [
			"172.16.0.50"
		],
		"dns": {}
	}]`

	networkStatusArrayIPv6Only = `[{
		"name": "test-v6-network",
		"interface": "eth2",
		"ips": [
			"2001:db8::100"
		],
		"dns": {}
	}]`

	networkStatusArrayEmpty = `[]`
)

func createPodWithNetworkStatus(statusJSON string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
	if statusJSON != "" {
		pod.Annotations = map[string]string{
			netdefv1.NetworkStatusAnnot: statusJSON,
		}
	}
	return pod
}

// TestGetMultusNetworkStatusString tests the GetMultusNetworkStatusString function
func TestGetMultusNetworkStatusString(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		expected string
	}{
		{
			name:     "nil pod",
			pod:      nil,
			expected: "",
		},
		{
			name:     "pod with no annotations",
			pod:      createPodWithNetworkStatus(""),
			expected: "",
		},
		{
			name:     "pod with network status annotation",
			pod:      createPodWithNetworkStatus(networkStatusIPv4Only),
			expected: networkStatusIPv4Only,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMultusNetworkStatusString(tt.pod)
			if result != tt.expected {
				t.Errorf("GetMultusNetworkStatusString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetMultusNetworkStatus tests the GetMultusNetworkStatus function
func TestGetMultusNetworkStatus(t *testing.T) {
	tests := []struct {
		name    string
		pod     *v1.Pod
		wantNil bool
	}{
		{
			name:    "nil pod",
			pod:     nil,
			wantNil: true,
		},
		{
			name:    "pod with no annotations",
			pod:     createPodWithNetworkStatus(""),
			wantNil: true,
		},
		{
			name:    "pod with invalid JSON",
			pod:     createPodWithNetworkStatus(networkStatusInvalidJSON),
			wantNil: true,
		},
		{
			name:    "pod with valid network status (not loom-cni)",
			pod:     createPodWithNetworkStatus(networkStatusIPv4Only),
			wantNil: true, // Returns nil because it's not loom-cni network
		},
		{
			name:    "pod with valid loom-cni network status",
			pod:     createPodWithNetworkStatus(networkStatusWithIPv4AndIPv6),
			wantNil: false, // This is loom-cni, so should return the status
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetIMLMultusNetworkStatus(tt.pod)
			if (result == nil) != tt.wantNil {
				t.Errorf("GetMultusNetworkStatus() = %v, want nil = %v", result, tt.wantNil)
			}
		})
	}
}

// TestGetPodIMLIPv4Addr tests the GetPodIMLIPv4Addr function
func TestGetPodIMLIPv4Addr(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		expected string
	}{
		{
			name:     "nil pod",
			pod:      nil,
			expected: "",
		},
		{
			name:     "pod with no network status",
			pod:      createPodWithNetworkStatus(""),
			expected: "",
		},
		{
			name:     "pod with IPv4 address (not loom-cni)",
			pod:      createPodWithNetworkStatus(networkStatusIPv4Only),
			expected: "", // Not loom-cni, should return empty
		},
		{

			name:     "pod_with_IPv4_and_IPv6_addresses",
			pod:      createPodWithNetworkStatus(networkStatusWithIPv4AndIPv6),
			expected: "10.123.0.9",
		},
		{
			name:     "pod with IPv6 only",
			pod:      createPodWithNetworkStatus(networkStatusIPv6Only),
			expected: "",
		},
		{
			name:     "pod with no IPs",
			pod:      createPodWithNetworkStatus(networkStatusNoIPs),
			expected: "",
		},
		{
			name:     "pod with invalid network status JSON",
			pod:      createPodWithNetworkStatus(networkStatusInvalidJSON),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPodIMLIPv4Addr(tt.pod)
			resultStr := ""
			if result != nil {
				resultStr = result.String()
			}
			if resultStr != tt.expected {
				t.Errorf("GetPodIMLIPv4Addr() = %v, want %v", resultStr, tt.expected)
			}
		})
	}
}

// TestGetPodIMLIPv6Addr tests the GetPodIMLIPv6Addr function
func TestGetPodIMLIPv6Addr(t *testing.T) {
	tests := []struct {
		name     string
		pod      *v1.Pod
		expected string
	}{
		{
			name:     "nil pod",
			pod:      nil,
			expected: "",
		},
		{
			name:     "pod with no network status",
			pod:      createPodWithNetworkStatus(""),
			expected: "",
		},
		{
			name:     "pod with IPv6 address (not loom-cni)",
			pod:      createPodWithNetworkStatus(networkStatusIPv6Only),
			expected: "", // Not loom-cni, should return empty
		},
		{
			name:     "pod with IPv4 and IPv6 addresses",
			pod:      createPodWithNetworkStatus(networkStatusWithIPv4AndIPv6),
			expected: "fd00::2:0:9",
		},
		{
			name:     "pod with IPv4 only",
			pod:      createPodWithNetworkStatus(networkStatusIPv4Only),
			expected: "",
		},
		{
			name:     "pod with no IPs",
			pod:      createPodWithNetworkStatus(networkStatusNoIPs),
			expected: "",
		},
		{
			name:     "pod with invalid network status JSON",
			pod:      createPodWithNetworkStatus(networkStatusInvalidJSON),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPodIMLIPv6Addr(tt.pod)
			resultStr := ""
			if result != nil {
				resultStr = result.String()
			}
			if resultStr != tt.expected {
				t.Errorf("GetPodIMLIPv6Addr() = %v, want %v", resultStr, tt.expected)
			}
		})
	}
}

// TestIPv4AndIPv6Extraction tests extraction of both IPv4 and IPv6 from the same network status
func TestIPv4AndIPv6Extraction(t *testing.T) {
	pod := createPodWithNetworkStatus(networkStatusWithIPv4AndIPv6)

	ipv4 := GetPodIMLIPv4Addr(pod)
	ipv6 := GetPodIMLIPv6Addr(pod)

	if ipv4 == nil {
		t.Error("Expected IPv4 address but got nil")
	} else if ipv4.String() != "10.123.0.9" {
		t.Errorf("Got IPv4 = %v, want 10.123.0.9", ipv4.String())
	}

	if ipv6 == nil {
		t.Error("Expected IPv6 address but got nil")
	} else if ipv6.String() != "fd00::2:0:9" {
		t.Errorf("Got IPv6 = %v, want fd00::2:0:9", ipv6.String())
	}
}

// TestNetIPTypes tests that returned IPs are of correct type
func TestNetIPTypes(t *testing.T) {
	pod := createPodWithNetworkStatus(networkStatusWithIPv4AndIPv6)

	ipv4 := GetPodIMLIPv4Addr(pod)
	if ipv4 != nil && len(ipv4) != net.IPv4len {
		t.Errorf("IPv4 address has wrong length: %d, want %d", len(ipv4), net.IPv4len)
	}

	ipv6 := GetPodIMLIPv6Addr(pod)
	if ipv6 != nil && len(ipv6) != net.IPv6len {
		t.Errorf("IPv6 address has wrong length: %d, want %d", len(ipv6), net.IPv6len)
	}
}

// TestArrayFormatNetworkStatus tests handling of array-formatted network status JSON
// This is the actual format returned by Multus in the network annotation
func TestArrayFormatNetworkStatus(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantNil bool
	}{
		{
			name:    "array with multiple entries",
			jsonStr: networkStatusArrayFormat,
			wantNil: false, // Should successfully parse loom-cni entry
		},
		{
			name:    "array with single entry",
			jsonStr: networkStatusArraySingleEntry,
			wantNil: true, // Single entry is not loom-cni, should return nil
		},
		{
			name:    "empty array",
			jsonStr: networkStatusArrayEmpty,
			wantNil: true, // Empty array should return nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := createPodWithNetworkStatus(tt.jsonStr)
			result := GetIMLMultusNetworkStatus(pod)
			if (result == nil) != tt.wantNil {
				t.Errorf("GetMultusNetworkStatus() with array format: got nil=%v, want nil=%v", result == nil, tt.wantNil)
			}
		})
	}
}

// TestGetPodIMLIPv4AddrWithArrayFormat tests IPv4 extraction from array-formatted network status
func TestGetPodIMLIPv4AddrWithArrayFormat(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected string
	}{
		{
			name:     "array format with multiple entries",
			jsonStr:  networkStatusArrayFormat,
			expected: "10.123.0.9", // Should get IPv4 from loom-cni entry
		},
		{
			name:     "array format with single entry",
			jsonStr:  networkStatusArraySingleEntry,
			expected: "", // Single entry is not loom-cni, should return empty
		},
		{
			name:     "array format IPv6 only",
			jsonStr:  networkStatusArrayIPv6Only,
			expected: "", // IPv6-only entry is not loom-cni, should return empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := createPodWithNetworkStatus(tt.jsonStr)
			result := GetPodIMLIPv4Addr(pod)
			resultStr := ""
			if result != nil {
				resultStr = result.String()
			}
			if resultStr != tt.expected {
				t.Errorf("GetPodIMLIPv4Addr() with array format = %v, want %v", resultStr, tt.expected)
			}
		})
	}
}

// TestGetPodIMLIPv6AddrWithArrayFormat tests IPv6 extraction from array-formatted network status
func TestGetPodIMLIPv6AddrWithArrayFormat(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		expected string
	}{
		{
			name:     "array format with multiple entries",
			jsonStr:  networkStatusArrayFormat,
			expected: "fd00::2:0:9", // Should get IPv6 from loom-cni entry
		},
		{
			name:     "array format with single entry",
			jsonStr:  networkStatusArraySingleEntry,
			expected: "", // Single entry is not loom-cni, should return empty
		},
		{
			name:     "array format IPv6 only",
			jsonStr:  networkStatusArrayIPv6Only,
			expected: "", // IPv6-only entry is not loom-cni, should return empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := createPodWithNetworkStatus(tt.jsonStr)
			result := GetPodIMLIPv6Addr(pod)
			resultStr := ""
			if result != nil {
				resultStr = result.String()
			}
			if resultStr != tt.expected {
				t.Errorf("GetPodIMLIPv6Addr() with array format = %v, want %v", resultStr, tt.expected)
			}
		})
	}
}
