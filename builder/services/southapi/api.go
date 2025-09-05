package southapi

import (
	"builder/api/v1alpha1"
	"encoding/json"
	"net/http"

	iplib "github.com/c-robinson/iplib/v2"
	"github.com/gorilla/mux"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SouthboundAPIController struct {
	AppRange      iplib.Net6
	NFRange       iplib.Net6
	LastAppSubnet *iplib.Net6
	LastNFSubnet  *iplib.Net6
	Reader        client.Reader
}

func InitializeSouthboundAPI(k8sClient client.Reader) (*http.Server, error) {
	controller := &SouthboundAPIController{
		AppRange:      iplib.Net6FromStr("fd00::/16"),
		NFRange:       iplib.Net6FromStr("fd01::/16"),
		LastAppSubnet: nil,
		LastNFSubnet:  nil,
		Reader:        k8sClient,
	}
	router := mux.NewRouter()
	server := &http.Server{
		Addr:    ":3267",
		Handler: router,
	}
	router.HandleFunc("/api/v1/nodemanager/subnet", controller.handleSubnetRequest).Methods("GET")
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

	response := SubnetResponse{
		AppSubnet: appnet.IPNet,
		NFSubnet:  nfnet.IPNet,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
}

func (c *SouthboundAPIController) handleAppDefinitionRequest(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement app definition retrieval
	appID, exists := mux.Vars(r)["id"]
	if !exists {
		http.Error(w, "App ID is required", http.StatusBadRequest)
		return
	}

	var apps v1alpha1.ApplicationList
	if err := c.Reader.List(r.Context(), &apps, client.MatchingFields{"metadata.uid": appID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(apps.Items) == 0 {
		http.NotFound(w, r)
		return
	}

	// Return the first matching object (UID is unique)
	app := apps.Items[0]
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(app); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *SouthboundAPIController) handleNFDefinitionRequest(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement NF definition retrieval
	nfID, exists := mux.Vars(r)["id"]
	if !exists {
		http.Error(w, "NF ID is required", http.StatusBadRequest)
		return
	}

	var nfs v1alpha1.NetworkFunctionList
	if err := c.Reader.List(r.Context(), &nfs, client.MatchingFields{"metadata.uid": nfID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(nfs.Items) == 0 {
		http.NotFound(w, r)
		return
	}

	// Return the first matching object (UID is unique)
	nf := nfs.Items[0]
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(nf); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
