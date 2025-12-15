package server

import (
	"context"
	"fmt"
	apps "iml-oakestra-agent/applications"
	"iml-oakestra-agent/logger"
	nfs "iml-oakestra-agent/networkfunctions"
	chains "iml-oakestra-agent/servicechains"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"go.yaml.in/yaml/v4"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var validate *validator.Validate

type Server struct {
	appsClient   *apps.Client
	nfsClient    *nfs.Client
	chainsClient *chains.Client
	httpServer   *http.Server
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
	}
	validate = validator.New(validator.WithRequiredStructEnabled())
	router.HandleFunc("/api/v1/agent/nsd/deploy", server.handleNSDCreation).Methods("POST")
	router.HandleFunc("/api/v1/agent/nsd/delete", server.handleNSDDeletion).Methods("POST")
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
		logger.ErrorLogger().Printf("Failed to decode NSD: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logger.DebugLogger().Printf("Parsed NSD: %+v", nsd)

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
			logger.ErrorLogger().Printf("Failed to create application: %v", err)
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
				Replicas: nfDescriptor.Replicas,
				Type:     nfs.NetworkFunctionType(nfDescriptor.Type),
				SubFunctions: func() []nfs.SubFunctionSpec {
					subFunctions := make([]nfs.SubFunctionSpec, len(nfDescriptor.SubFunctions))
					for i, sf := range nfDescriptor.SubFunctions {
						subFunctions[i] = nfs.SubFunctionSpec{
							Name: sf.Name,
							ID:   sf.ID,
						}
					}
					return subFunctions
				}(),
				Containers: func() []corev1.Container {
					containers := make([]corev1.Container, len(nfDescriptor.Containers))
					for i, c := range nfDescriptor.Containers {
						containers[i] = corev1.Container{
							Name:    c.Name,
							Image:   c.Image,
							Command: c.Command,
							Args:    c.Args,
						}
					}
					return containers
				}(),
			},
		}
		if err := s.nfsClient.Create(nf); err != nil {
			logger.ErrorLogger().Printf("Failed to create network function: %v", err)
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
				From: &chains.ApplicationReference{
					Name:      scDescriptor.From.Name,
					Namespace: scDescriptor.From.Namespace,
				},
				To: &chains.ApplicationReference{
					Name:      scDescriptor.To.Name,
					Namespace: scDescriptor.To.Namespace,
				},
				Functions: make([]chains.NetworkFunctionReference, len(scDescriptor.Functions)),
			},
		}
		for i, fn := range scDescriptor.Functions {
			chain.Spec.Functions[i] = chains.NetworkFunctionReference{
				Name:      fn.Name,
				Namespace: fn.Namespace,
				SubFunctionID: fn.SubFunctionID,
			}
		}
		if err := s.chainsClient.Create(chain); err != nil {
			logger.ErrorLogger().Printf("Failed to create service chain: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Send response
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("NSD deployed successfully"))
}

func (s *Server) handleNSDDeletion(w http.ResponseWriter, r *http.Request) {
	logger.DebugLogger().Printf("Received NSD deletion request")

	var nsd NetworkServiceDescriptor
	if err := yaml.NewDecoder(r.Body).Decode(&nsd); err != nil {
		logger.ErrorLogger().Printf("Failed to decode NSD: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logger.DebugLogger().Printf("Parsed NSD: %+v", nsd)

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

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("NSD deleted successfully"))
}
