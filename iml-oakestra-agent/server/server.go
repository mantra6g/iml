package server

import (
	"context"
	"encoding/json"
	"fmt"
	apps "iml-oakestra-agent/applications"
	"iml-oakestra-agent/logger"
	nfs "iml-oakestra-agent/networkfunctions"
	chains "iml-oakestra-agent/servicechains"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"go.yaml.in/yaml/v4"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var validate *validator.Validate

type Server struct {
	appsClient  *apps.Client
	nfsClient   *nfs.Client
	chainsClient *chains.Client
	httpServer  *http.Server
	nsdStore    map[uint64]*NetworkServiceDescriptor
	lastID      uint64
}

func New(appsClient *apps.Client, nfsClient *nfs.Client, chainsClient *chains.Client) (*Server, error) {
	// Validate the services
	if appsClient == nil {
		return nil, fmt.Errorf("appsClient cannot be nil")
	}
	if nfsClient == nil {
		return nil, fmt.Errorf("nfsClient cannot be nil")
	}
	if chainsClient == nil {
		return nil, fmt.Errorf("chainsClient cannot be nil")
	}

	router := mux.NewRouter()
	httpServer := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: router,
	}

	// Create a new CNI controller with the services
	server := &Server{
		appsClient:   appsClient,
		nfsClient:    nfsClient,
		chainsClient: chainsClient,
		httpServer:   httpServer,
		nsdStore:     make(map[uint64]*NetworkServiceDescriptor),
		lastID:       0,
	}
	validate = validator.New(validator.WithRequiredStructEnabled())
	router.HandleFunc("/api/v1/agent/nsd", server.handleNSDCreation).Methods("POST")
	router.HandleFunc("/api/v1/agent/nsd/{id}", server.handleNSDDeletion).Methods("DELETE")
	go httpServer.ListenAndServe()
	return server, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleNSDCreation(w http.ResponseWriter, r *http.Request) {
	logger.DebugLogger().Printf("Received new NSD creation request")

	var nsd NetworkServiceDescriptor
	if err := yaml.NewDecoder(r.Body).Decode(&nsd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, appDescriptor := range nsd.ApplicationFunctions {
		app := &apps.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      appDescriptor.Name,
				Namespace: appDescriptor.Namespace,
			},
			Spec: apps.ApplicationSpec{
				OverrideID: appDescriptor.ID,
			},
		}
		if err := s.appsClient.Create(app); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, nfDescriptor := range nsd.NetworkFunctions {
		nf := &nfs.NetworkFunction{
			ObjectMeta: v1.ObjectMeta{
				Name:      nfDescriptor.Name,
				Namespace: nfDescriptor.Namespace,
			},
			Spec: nfs.NetworkFunctionSpec{
				Image:      nfDescriptor.Image,
			},
		}
		if err := s.nfsClient.Create(nf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, scDescriptor := range nsd.ServiceChains {
		chain := &chains.ServiceChain{
			ObjectMeta: v1.ObjectMeta{
				Name:      scDescriptor.Name,
				Namespace: scDescriptor.Namespace,
			},
			Spec: chains.ServiceChainSpec{
				From: &chains.ObjectReference{
					Name:      scDescriptor.From.Name,
					Namespace: scDescriptor.From.Namespace,
				},
				To: &chains.ObjectReference{
					Name:      scDescriptor.To.Name,
					Namespace: scDescriptor.To.Namespace,
				},
				Functions: make([]chains.ObjectReference, len(scDescriptor.Functions)),
			},
		}
		for i, fn := range scDescriptor.Functions {
			chain.Spec.Functions[i] = chains.ObjectReference{
				Name:      fn.Name,
				Namespace: fn.Namespace,
			}
		}
		if err := s.chainsClient.Create(chain); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update and set the NSD ID
	s.lastID++
	nsdID := s.lastID
	s.nsdStore[nsdID] = &nsd

	// Send response
	w.Header().Set("Content-Type", "application/json")
	response := Response{
		Message: "NSD created successfully",
		ID:      nsdID,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.ErrorLogger().Printf("Failed to encode response: %v", err)
	}
}

func (s *Server) handleNSDDeletion(w http.ResponseWriter, r *http.Request) {
	logger.DebugLogger().Printf("Received NSD deletion request")

	nsdIDStr, exists := mux.Vars(r)["id"]
	if !exists {
		http.Error(w, "NSD ID is required", http.StatusBadRequest)
		return
	}

	var nsdID uint64
	if _, err := fmt.Sscanf(nsdIDStr, "%d", &nsdID); err != nil {
		http.Error(w, "Invalid NSD ID", http.StatusBadRequest)
		return
	}

	nsd, found := s.nsdStore[nsdID]
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for _, scDescriptor := range nsd.ServiceChains {
		if err := s.chainsClient.Delete(scDescriptor.Name, scDescriptor.Namespace); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, nfDescriptor := range nsd.NetworkFunctions {
		if err := s.nfsClient.Delete(nfDescriptor.Name, nfDescriptor.Namespace); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, appDescriptor := range nsd.ApplicationFunctions {
		if err := s.appsClient.Delete(appDescriptor.Name, appDescriptor.Namespace); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	delete(s.nsdStore, nsdID)

	w.Header().Set("Content-Type", "application/json")
	response := Response{
		Message: "NSD deleted successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.ErrorLogger().Printf("Failed to encode response: %v", err)
	}
}