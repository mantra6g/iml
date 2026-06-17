package utils

import (
	"encoding/json"
	"net/netip"

	netdefv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	v1 "k8s.io/api/core/v1"
)

func GetPodIMLIPv6Addr(pod *v1.Pod) netip.Addr {
	status := GetIMLMultusNetworkStatus(pod)
	if status == nil {
		return netip.Addr{}
	}
	for _, ip := range status.IPs {
		addr, _ := netip.ParseAddr(ip)
		if !addr.IsValid() {
			continue
		}
		if addr.Is6() {
			return addr
		}
	}
	return netip.Addr{}
}

func GetMultusNetworkStatusString(pod *v1.Pod) string {
	if pod == nil {
		return ""
	}
	annotations := pod.GetAnnotations()
	if annotations == nil {
		return ""
	}
	netStatus, ok := annotations[netdefv1.NetworkStatusAnnot]
	if !ok {
		return ""
	}
	return netStatus
}

func GetIMLMultusNetworkStatus(pod *v1.Pod) *netdefv1.NetworkStatus {
	statusString := GetMultusNetworkStatusString(pod)
	if statusString == "" {
		return nil
	}

	// Try to parse as a single NetworkStatus object first
	status := &netdefv1.NetworkStatus{}
	if err := json.Unmarshal([]byte(statusString), status); err == nil {
		// Successfully parsed as single object
		// Check if it's the loom-cni network
		if status.Name == "loom-cni" || status.Name == "loom-system/loom-cni" {
			return status
		}
		return nil
	}

	// If single object parsing fails, try to parse as an array of NetworkStatus objects
	var statuses []netdefv1.NetworkStatus
	if err := json.Unmarshal([]byte(statusString), &statuses); err != nil {
		// Neither format works, return nil
		return nil
	}

	// If array is empty, return nil
	if len(statuses) == 0 {
		return nil
	}

	// Return the network status with the name "loom-cni" or containing "loom-cni"
	for i := range statuses {
		if statuses[i].Name == "loom-cni" || statuses[i].Name == "loom-system/loom-cni" {
			return &statuses[i]
		}
	}
	return nil
}
