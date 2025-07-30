package vnfs

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"net/http"
)

func (svc *VnfService) RegisterVnfInstance(request *VnfInstanceRegistrationRequest) (*models.VnfInstance, services.Error) {
	// If the VNF instance is already registered, return its details
	vnfInstance, _ := svc.registry.FindVnfInstanceByContainerID(request.ContainerID)
	if vnfInstance != nil {
		return vnfInstance, nil
	}

	// Verify that the VNF exists in the registry
	vnf, _ := svc.registry.FindNetworkFunctionByID(request.VnfID)
	if vnf == nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"VNF %s not found", request.VnfID)
	}

	// Generate an interface name for the VNF instance
	ifaceName, err := helpers.GenerateUniqueInterfaceName(request.ContainerID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to generate interface name for VNF instance %s: %v", request.ContainerID, err)
	}

	// Check if there is already a VNF Group for this vnf
	vnfGroup, _ := svc.registry.FindVnfGroupByVnfID(request.VnfID)
	if vnfGroup == nil {

		// If it does not exist, assign the next VNF IP from the existing group
		vnfIP, err := svc.vnfIP.Next()
		if err != nil {
			return nil, services.Errorf(
				http.StatusInternalServerError,
				"failed to allocate VNF IP for VNF %s: %v", request.VnfID, err)
		}

		vnfGroup = &models.VnfGroup{
			VnfID: request.VnfID,
			IP:    vnfIP.String(),
		}
	}

	// Create the VNF instance details
	details := &models.VnfInstance{
		ContainerID: request.ContainerID,
		Interface:   ifaceName,
	}

	// Assign the VNF instance to the VNF group
	vnfGroup.Instances = []models.VnfInstance{*details}

	// Save/Update the VNF group in the registry
	err = svc.registry.SaveVnfGroup(vnfGroup)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to register VNF instance %s: %v", request.ContainerID, err)
	}

	return details, nil
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

func InitializeVnfService(registry *db.Registry, appIP, vnfIP *helpers.IPAllocator) (*VnfService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Create a new VNF service with the provided registry
	return &VnfService{
		registry: registry,
		appIP:    appIP,
		vnfIP:    vnfIP,
	}, nil
}
