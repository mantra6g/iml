package dataplane

import (
)

type NetworkServiceChain struct {
	Applications []string `json:"apps"`
	NetFunctions []string `json:"nfs"`
	ServiceChain []string `json:"service_chain"`
}