package southapi

import (
	"builder/pkg/cache"
	"encoding/json"
	"fmt"
	"net/http"

	iplib "github.com/c-robinson/iplib/v2"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/types"
)

type SouthboundAPIController struct {
	ClusterCIDR   iplib.Net6
	AppRange      iplib.Net6
	NFRange       iplib.Net6
	LastAppSubnet *iplib.Net6
	LastNFSubnet  *iplib.Net6
	Cache         *cache.Service
	logger        logr.Logger
}

func InitializeSouthboundAPI(cache *cache.Service, logger logr.Logger) (*http.Server, error) {
	controller := &SouthboundAPIController{
		ClusterCIDR:   iplib.Net6FromStr("fd00::/15"),
		AppRange:      iplib.Net6FromStr("fd00::/16"),
		NFRange:       iplib.Net6FromStr("fd01::/16"),
		LastAppSubnet: nil,
		LastNFSubnet:  nil,
		Cache:         cache,
		logger:        logger,
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
	// TODO: Check assignments
	var appnet iplib.Net6
	if c.LastAppSubnet != nil {
		appnet = c.LastAppSubnet.NextNet(32)
	} else {
		appnets, err := c.AppRange.Subnet(32, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		appnet = appnets[0]
	}

	var nfnet iplib.Net6
	if c.LastNFSubnet != nil {
		nfnet = c.LastNFSubnet.NextNet(32)
	} else {
		nfnets, err := c.NFRange.Subnet(32, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nfnet = nfnets[0]
	}

	// get the nfrouter addresses
	nfrouterAppIP, err := appnet.NextIP(appnet.FirstAddress())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nfrouterVnfIP, err := nfnet.NextIP(nfnet.FirstAddress())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := SubnetResponse{
		ClusterCIDR: c.ClusterCIDR.IPNet,
		AppSubnet: appnet.IPNet,
		NFSubnet:  nfnet.IPNet,
		NFRouterAppIP: nfrouterAppIP,
		NFRouterVNFIP: nfrouterVnfIP,
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
		c.logger.Info("Application %s not found", appID)
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
		c.logger.Info("Network Function with ID %s not found", nfID)
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
		c.logger.Info("Service chain with ID %s not found", scID)
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
