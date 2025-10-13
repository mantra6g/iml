package southapi

import (
	"net"
)

type SubnetResponse struct {
	ClusterCIDR   net.IPNet `json:"cluster_cidr"`
	AppSubnet     net.IPNet `json:"app_subnet"`
	NFSubnet      net.IPNet `json:"nf_subnet"`
	NFRouterAppIP net.IP    `json:"nf_router_app_ip"`
	NFRouterVNFIP net.IP    `json:"nf_router_vnf_ip"`
}