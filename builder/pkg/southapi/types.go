package southapi

import (
	"net"
)

type SubnetResponse struct {
	ClusterCIDR net.IPNet `json:"cluster_cidr"`
	AppSubnet   net.IPNet `json:"app_subnet"`
	NFSubnet    net.IPNet `json:"nf_subnet"`
	SIDSubnet   net.IPNet `json:"sid_subnet"`
	TunSubnet   net.IPNet `json:"tun_subnet"`
}
