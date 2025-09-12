package southapi

import (
	"net"
	"time"
)

type SubnetResponse struct {
	AppSubnet net.IPNet `json:"app_subnet"`
	NFSubnet  net.IPNet `json:"nf_subnet"`
}

type ApplicationStatusResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type NFStatusResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

type SCStatusResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}