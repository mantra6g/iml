package chains

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"iml-daemon/services/eventbus"
	"net/http"
)

type ChainService struct {
	registry *db.Registry
	appIP    *helpers.IPAllocator
	vnfIP    *helpers.IPAllocator
	eventBus *eventbus.EventBus
}

func (svc *ChainService) RegisterNetworkService(request *NetworkServiceRegistrationRequest) (*models.ServiceChain, services.Error) {
	// Check if the chain is already registered
	// If it is, return an error
	chainInstance, _ := svc.registry.FindNetworkServiceChainByGlobalID(request.ChainID)
	if chainInstance != nil {
		return chainInstance, services.Errorf(
			http.StatusConflict,
			"network service chain with ID %s already exists", request.ChainID)
	}

	// Find the applications and VNFs involved in the chain
	srcApp, err := svc.registry.FindAppByGlobalID(request.SrcAppID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"source application %s not found: %v", request.SrcAppID, err)
	}
	dstApp, err := svc.registry.FindAppByGlobalID(request.DstAppID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"destination application %s not found: %v", request.DstAppID, err)
	}
	var vnfs []models.ServiceChainVnfs
	for i, vnfID := range request.Vnfs {
		vnf, err := svc.registry.FindNetworkFunctionByGlobalID(vnfID)
		if err != nil {
			return nil, services.Errorf(
				http.StatusNotFound,
				"VNF %s not found: %v", vnfID, err)
		}
		vnfs = append(vnfs, models.ServiceChainVnfs{
			VnfID:    vnf.ID,
			Position: uint8(i),
		})
	}

	// Create a new network service details object
	details := &models.ServiceChain{
		GlobalID: request.ChainID,
		SrcAppID: srcApp.ID,
		DstAppID: dstApp.ID,
		Elements: vnfs,
	}

	// Save the network service chain to the registry
	err = svc.registry.SaveNetworkServiceChain(details)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to save network service chain: %v", err)
	}

	// Publish an event to the event bus
	svc.eventBus.Publish(eventbus.Event{
		Name:    "ServiceChainAdded",
		Payload: *details,
	})

	return details, nil
}

func InitializeNetworkServiceChainService(
	registry *db.Registry, appIP, vnfIP *helpers.IPAllocator, eb *eventbus.EventBus) (*ChainService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	return &ChainService{
		registry: registry,
		appIP:    appIP,
		vnfIP:    vnfIP,
		eventBus: eb,
	}, nil
}
