package southapi

import (
	"builder/pkg/cache"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	iplib "github.com/c-robinson/iplib/v2"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/types"
)

type SouthboundAPIController struct {
	ClusterCIDR     iplib.Net6
	AppNetAllocator *Subnet6Allocator
	NFNetAllocator  *Subnet6Allocator
	SIDNetAllocator *Subnet6Allocator
	TunNetAllocator *Subnet6Allocator
	Cache           *cache.Service
	logger          logr.Logger
}

func InitializeSouthboundAPI(cache *cache.Service, logger logr.Logger) (*http.Server, error) {
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
		ClusterCIDR:      iplib.Net6FromStr("fd00::/15"),
		AppNetAllocator:  appNetAllocator,
		NFNetAllocator:   nfNetAllocator,
		SIDNetAllocator:  sidNetAllocator,
		TunNetAllocator:  tunNetAllocator,
		Cache:            cache,
		logger:           logger,
	}
	router := mux.NewRouter()
	server := &http.Server{
		Addr:    ":3267",
		Handler: router,
	}
	router.HandleFunc("/api/v1/nodemanager/subnet", controller.handleSubnetRequest).Methods("GET")
	router.HandleFunc("/api/v1/chains/{id}", controller.handleSCDefinitionRequest).Methods("GET")
	router.HandleFunc("/api/v1/apps/{id}", controller.handleAppDefinitionRequest).Methods("GET")
	router.HandleFunc("/api/v1/nfs/{id}", controller.handleNFDefinitionRequest).Methods("GET")
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

func (c *SouthboundAPIController) handleAppDefinitionRequest(w http.ResponseWriter, r *http.Request) {
	c.logger.Info("Received request for application definition")

	appID, exists := mux.Vars(r)["id"]
	if !exists {
		c.logger.Error(fmt.Errorf("application ID not provided in request"), "Bad request")
		http.Error(w, "Application ID is required", http.StatusBadRequest)
		return
	}

	appDefinition, err := c.Cache.GetApp(types.UID(appID))
	if err != nil {
		c.logger.Info("Application not found", "appID", appID)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(appDefinition); err != nil {
		c.logger.Error(err, "Failed to encode application definition")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *SouthboundAPIController) handleNFDefinitionRequest(w http.ResponseWriter, r *http.Request) {
	c.logger.Info("Received request for network function definition")

	// TODO: Implement NF definition retrieval
	nfID, exists := mux.Vars(r)["id"]
	if !exists {
		c.logger.Error(fmt.Errorf("network Function ID not provided in request"), "Bad request")
		http.Error(w, "Network Function ID is required", http.StatusBadRequest)
		return
	}

	nfDefinition, err := c.Cache.GetNF(types.UID(nfID))
	if err != nil {
		c.logger.Info("Network Function not found", "nfID", nfID)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(nfDefinition); err != nil {
		c.logger.Error(err, "Failed to encode network function definition")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *SouthboundAPIController) handleSCDefinitionRequest(w http.ResponseWriter, r *http.Request) {
	c.logger.Info("Received request for service chain definition")

	// TODO: Implement service chain definition retrieval
	scID, exists := mux.Vars(r)["id"]
	if !exists {
		c.logger.Error(fmt.Errorf("service Chain ID not provided in request"), "Bad request")
		http.Error(w, "Service Chain ID is required", http.StatusBadRequest)
		return
	}

	scDefinition, err := c.Cache.GetServiceChain(types.UID(scID))
	if err != nil {
		c.logger.Info("Service chain not found", "scID", scID)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(scDefinition); err != nil {
		c.logger.Error(err, "Failed to encode service chain definition")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
