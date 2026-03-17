package southapi

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	iplib "github.com/c-robinson/iplib/v2"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

type SouthboundAPIController struct {
	ClusterCIDR     iplib.Net6
	AppNetAllocator *Subnet6Allocator
	NFNetAllocator  *Subnet6Allocator
	SIDNetAllocator *Subnet6Allocator
	TunNetAllocator *Subnet6Allocator
	logger          logr.Logger
}

func InitializeSouthboundAPI(logger logr.Logger) (*http.Server, error) {
	_, appSuperNet, err := net.ParseCIDR("fd00:0000::/20")
	if err != nil {
		return nil, fmt.Errorf("failed to parse app supernet: %v", err)
	}
	_, nfSuperNet, err := net.ParseCIDR("fd00:1000::/20")
	if err != nil {
		return nil, fmt.Errorf("failed to parse nf supernet: %v", err)
	}
	_, sidSuperNet, err := net.ParseCIDR("fd00:2000::/20")
	if err != nil {
		return nil, fmt.Errorf("failed to parse sid supernet: %v", err)
	}
	_, tunSuperNet, err := net.ParseCIDR("fd00:3000::/20")
	if err != nil {
		return nil, fmt.Errorf("failed to parse tun supernet: %v", err)
	}

	appNetAllocator, err := NewSubnet6Allocator(appSuperNet, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to create app subnet allocator: %v", err)
	}
	nfNetAllocator, err := NewSubnet6Allocator(nfSuperNet, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to create nf subnet allocator: %v", err)
	}
	sidNetAllocator, err := NewSubnet6Allocator(sidSuperNet, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to create sid subnet allocator: %v", err)
	}
	tunNetAllocator, err := NewSubnet6Allocator(tunSuperNet, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to create tun subnet allocator: %v", err)
	}

	controller := &SouthboundAPIController{
		ClusterCIDR:     iplib.Net6FromStr("fd00::/15"),
		AppNetAllocator: appNetAllocator,
		NFNetAllocator:  nfNetAllocator,
		SIDNetAllocator: sidNetAllocator,
		TunNetAllocator: tunNetAllocator,
		logger:          logger,
	}
	router := mux.NewRouter()
	server := &http.Server{
		Addr:    ":3267",
		Handler: router,
	}
	router.HandleFunc("/api/v1/nodemanager/subnet", controller.handleSubnetRequest).Methods("GET")
	go server.ListenAndServe()
	return server, nil
}

func (c *SouthboundAPIController) handleSubnetRequest(w http.ResponseWriter, r *http.Request) {
	appNet, err := c.AppNetAllocator.Allocate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nfNet, err := c.NFNetAllocator.Allocate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sidNet, err := c.SIDNetAllocator.Allocate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tunNet, err := c.TunNetAllocator.Allocate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := SubnetResponse{
		ClusterCIDR: c.ClusterCIDR.IPNet,
		AppSubnet:   *appNet,
		NFSubnet:    *nfNet,
		TunSubnet:   *tunNet,
		SIDSubnet:   *sidNet,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
