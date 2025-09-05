package southapi

import (
	"net"
)

type SubnetResponse struct {
	AppSubnet net.IPNet `json:"app_subnet"`
	NFSubnet  net.IPNet `json:"nf_subnet"`
}
