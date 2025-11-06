package vnfs

import (
	"context"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/vnfs"
	"net/http"
)

func (svc *VnfService) RegisterLocalVnfInstance(request *VnfInstanceRegistrationRequest) (*models.VnfInstance, services.Error) {
	// If the VNF instance is already registered, return its details
	vnfInstance, _ := svc.registry.FindVnfInstanceByContainerID(request.ContainerID)
	if vnfInstance != nil {
		return vnfInstance, nil
	}

	// Generate an interface name for the VNF instance
	ifaceName, err := helpers.GenerateUniqueInterfaceName(request.ContainerID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to generate interface name for VNF instance %s: %v", request.ContainerID, err)
	}

	instance, err := svc.vnfFactory.NewLocalInstance(request.VnfID, request.ContainerID, ifaceName)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to create VNF instance %s: %v", request.ContainerID, err)
	}

	return instance, nil
}

func (r *VnfService) TeardownVnfInstance(request *VnfInstanceTeardownRequest) services.Error {
	// Find the VNF instance by container ID
	// If it does not exist, return nil
	vnfInstance, _ := r.registry.FindVnfInstanceByContainerID(request.ContainerID)
	if vnfInstance == nil {
		return nil
	}

	// Remove the VNF instance from the registry
	err := r.vnfFactory.DeleteInstance(vnfInstance)
	if err != nil {
		return services.Errorf(
			http.StatusInternalServerError,
			"failed to delete VNF instance %s: %v", request.ContainerID, err)
	}

	return nil
}

func (r *VnfService) Shutdown(ctx context.Context) error {
	// Perform any necessary cleanup here
	// For now, this is a no-op
	return nil
}

func InitializeVnfService(
	registry *db.Registry, eb *events.EventBus, imlClient *iml.Client, vnfFactory *vnfs.InstanceFactory) (*VnfService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Create a new VNF service with the provided registry
	return &VnfService{
		registry:   registry,
		eventBus:   eb,
		imlClient:  imlClient,
		vnfFactory: vnfFactory,
	}, nil
}
