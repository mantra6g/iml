package apps

import (
	"fmt"
	"net/http"

	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
)

func (svc *AppService) RegisterAppInstance(request *AppInstanceRegistrationRequest) (*models.AppInstance, services.Error) {
	// Check if the instance is already registered
	// If it is, return its details
	appInstance, _ := svc.registry.FindAppInstanceByContainerID(request.ContainerID)
	if appInstance != nil {
		return appInstance, nil
	}

	// Verify that the application exists in the registry
	_, err := svc.registry.FindAppById(request.ApplicationID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"application %s not found: %v", request.ApplicationID, err)
	}

	// Allocate an IP for the application instance
	appIP, err := svc.appIP.Next()
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to allocate IP for app instance %s: %v", request.ContainerID, err)
	}

	// Generate a new interface name for the application instance
	ifaceName, err := helpers.GenerateUniqueInterfaceName(request.ContainerID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to generate interface name for app instance %s: %v", request.ContainerID, err)
	}

	// Create the application details
	details := &models.AppInstance{
		ApplicationID: request.ApplicationID,
		ContainerID:   request.ContainerID,
		IP:            appIP.String(),
		Interface:     ifaceName,
	}

	// Save the application instance in the registry
	// This will also assign an ID to the object automatically
	err = svc.registry.SaveAppInstance(details)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to register app instance %s: %v", request.ContainerID, err)
	}

	return details, nil
}

func (svc *AppService) TeardownAppInstance(request *AppInstanceTeardownRequest) (services.Error) {
	// First, find the application instance by container ID
	// If it does not exist, just return
	appInstance, _ := svc.registry.FindAppInstanceByContainerID(request.ContainerID)
	if appInstance == nil {
		return nil
	}

	// Remove the application instance from the registry
	if err := svc.registry.RemoveAppInstanceByContainerID(request.ContainerID); err != nil {
		return services.Errorf(
			http.StatusInternalServerError,
			"failed to remove app instance with container id %s: %v", request.ContainerID, err)
	}

	return nil
}

func InitializeAppService(registry *db.Registry, appIP, vnfIP *helpers.IPAllocator) (*AppService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Initialize the application service with the provided registry
	return &AppService{
		registry: registry,
	}, nil
}
