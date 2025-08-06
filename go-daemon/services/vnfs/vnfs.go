package vnfs

import (
	"context"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"iml-daemon/services/eventbus"
	"net/http"
)

func (svc *VnfService) RegisterLocalVnfInstance(request *VnfInstanceRegistrationRequest) (*models.VnfInstance, services.Error) {
	// If the VNF instance is already registered, return its details
	vnfInstance, _ := svc.registry.FindVnfInstanceByContainerID(request.ContainerID)
	if vnfInstance != nil {
		return vnfInstance, nil
	}

	// Verify that the VNF exists in the registry
	vnf, _ := svc.registry.FindNetworkFunctionByGlobalID(request.VnfID)
	if vnf == nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"VNF %s not found", request.VnfID)
	}

	// Check if there is already a VNF Group for this vnf
	vnfGroup, _ := svc.registry.FindLocalVnfGroupByVnfID(request.VnfID)
	var errDetails services.Error
	if vnfGroup == nil {
		vnfGroup, errDetails = svc.RegisterLocalVnfGroup(vnf)
		if errDetails != nil {
			return nil, errDetails
		}
	}

	// Generate an interface name for the VNF instance
	ifaceName, err := helpers.GenerateUniqueInterfaceName(request.ContainerID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to generate interface name for VNF instance %s: %v", request.ContainerID, err)
	}

	// Create the VNF instance details
	details := &models.VnfInstance{
		GroupID:     vnfGroup.ID,
		ContainerID: request.ContainerID,
		IfaceName:   ifaceName,
	}

	// Save/Update the VNF instance in the registry
	err = svc.registry.SaveVnfInstance(details)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to register VNF instance %s: %v", request.ContainerID, err)
	}

	return details, nil
}

func (svc *VnfService) RegisterLocalVnfGroup(vnf *models.VirtualNetworkFunction) (*models.VnfGroup, services.Error) {
	sid, err := svc.vnfIP.Next()
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to allocate IP for VNF group %s: %v", vnf.GlobalID, err)
	}

	vnfGroup := &models.VnfGroup{
		VnfID:   vnf.ID,
		WorkerID: nil,
		SID:     sid.String(),
	}
	if err := svc.registry.SaveVnfGroup(vnfGroup); err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to save VNF group %s: %v", vnf.GlobalID, err)
	}

	svc.eventBus.Publish(eventbus.Event{
		Name: "VnfGroupRegistered",
		Payload: *vnfGroup,
	})

	return vnfGroup, nil
}

func (r *VnfService) TeardownVnfInstance(request *VnfInstanceTeardownRequest) services.Error {
	// Find the VNF instance by container ID
	// If it does not exist, return nil
	vnfInstance, _ := r.registry.FindVnfInstanceByContainerID(request.ContainerID)
	if vnfInstance == nil {
		return nil
	}

	// Remove the VNF instance from the registry
	if err := r.registry.RemoveVnfInstanceByContainerID(request.ContainerID); err != nil {
		return services.Errorf(
			http.StatusInternalServerError,
			"failed to remove VNF instance %s: %v", request.ContainerID, err)
	}

	return nil
}

func (r *VnfService) Shutdown(ctx context.Context) error {
	// Perform any necessary cleanup here
	// For now, this is a no-op
	return nil
}

func InitializeVnfService(
	registry *db.Registry, appIP, vnfIP *helpers.IPAllocator, eb *eventbus.EventBus) (*VnfService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Create a new VNF service with the provided registry
	return &VnfService{
		registry: registry,
		appIP:    appIP,
		vnfIP:    vnfIP,
		eventBus: eb,
	}, nil
}