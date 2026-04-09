package env

import (
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultIMLConfigMapPath = "/etc/iml/config"
)

type ClusterCIDRConfig struct {
	ClusterPoolIPv4CIDR     netip.Prefix
	ClusterPoolIPv6CIDR     netip.Prefix
	ClusterPoolIPv4MaskSize int64
	ClusterPoolIPv6MaskSize int64
}

func ParseClusterCIDRConfig() (*ClusterCIDRConfig, error) {
	get := func(key string) (string, error) {
		data, err := os.ReadFile(filepath.Join(DefaultIMLConfigMapPath, key))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}

	ipv4CidrStr, err := get("cluster-pool-ipv4-cidr")
	if err != nil {
		return nil, err
	}
	ipv4Cidr, err := netip.ParsePrefix(ipv4CidrStr)
	if err != nil {
		return nil, err
	}

	ipv6CidrStr, err := get("cluster-pool-ipv6-cidr")
	if err != nil {
		return nil, err
	}
	ipv6Cidr, err := netip.ParsePrefix(ipv6CidrStr)
	if err != nil {
		return nil, err
	}

	ipv4MaskSizeStr, err := get("cluster-pool-ipv4-mask-size")
	if err != nil {
		return nil, err
	}
	ipv4MaskSize, err := strconv.ParseInt(ipv4MaskSizeStr, 10, 64)
	if err != nil {
		return nil, err
	}

	ipv6MaskSizeStr, err := get("cluster-pool-ipv6-mask-size")
	if err != nil {
		return nil, err
	}
	ipv6MaskSize, err := strconv.ParseInt(ipv6MaskSizeStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return &ClusterCIDRConfig{
		ClusterPoolIPv4CIDR:     ipv4Cidr,
		ClusterPoolIPv6CIDR:     ipv6Cidr,
		ClusterPoolIPv4MaskSize: ipv4MaskSize,
		ClusterPoolIPv6MaskSize: ipv6MaskSize,
	}, nil
}
