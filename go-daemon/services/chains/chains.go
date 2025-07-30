package chains

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"net/http"
)

type ChainService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
}

func (svc *ChainService) RegisterNetworkService(request *NetworkServiceRegistrationRequest) (*models.NetworkServiceChain, services.Error) {
	// Check if the chain is already registered
	// If it is, return an error
	chainInstance, _ := svc.registry.FindNetworkServiceChainByID(request.ChainID)
	if chainInstance != nil {
		return chainInstance, services.Errorf(
			http.StatusConflict,
			"network service chain with ID %s already exists", request.ChainID)
	}

	// Create a new network service details object
	details := &models.NetworkServiceChain{
		ID:          request.ChainID,
		// Populate other fields as necessary
	}

	// Save the network service chain to the registry
	err := svc.registry.SaveNetworkServiceChain(details)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to save network service chain: %v", err)
	}

	return details, nil
}

func InitializeNetworkServiceChainService(registry *db.Registry, appIP, vnfIP *helpers.IPAllocator) (*ChainService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	return &ChainService{
		registry: registry,
		appIP:    appIP,
		vnfIP:    vnfIP,
	}, nil
}