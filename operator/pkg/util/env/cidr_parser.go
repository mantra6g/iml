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

	clusterPoolIPv4CidrStr, err := get("cluster-pool-ipv4-cidr")
	if err != nil {
		return nil, err
	}
	clusterPoolIPv4Cidr, err := netip.ParsePrefix(clusterPoolIPv4CidrStr)
	if err != nil {
		return nil, err
	}

	clusterPoolIPv6CidrStr, err := get("cluster-pool-ipv6-cidr")
	if err != nil {
		return nil, err
	}
	clusterPoolIPv6Cidr, err := netip.ParsePrefix(clusterPoolIPv6CidrStr)
	if err != nil {
		return nil, err
	}

	clusterPoolIPv4MaskSizeStr, err := get("cluster-pool-ipv4-mask-size")
	if err != nil {
		return nil, err
	}
	clusterPoolIPv4MaskSize, err := strconv.ParseInt(clusterPoolIPv4MaskSizeStr, 10, 64)
	if err != nil {
		return nil, err
	}

	clusterPoolIPv6MaskSizeStr, err := get("cluster-pool-ipv6-mask-size")
	if err != nil {
		return nil, err
	}
	clusterPoolIPv6MaskSize, err := strconv.ParseInt(clusterPoolIPv6MaskSizeStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return &ClusterCIDRConfig{
		ClusterPoolIPv4CIDR:     clusterPoolIPv4Cidr,
		ClusterPoolIPv6CIDR:     clusterPoolIPv6Cidr,
		ClusterPoolIPv4MaskSize: clusterPoolIPv4MaskSize,
		ClusterPoolIPv6MaskSize: clusterPoolIPv6MaskSize,
	}, nil
}
