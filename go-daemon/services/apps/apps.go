package apps

import (
	"context"
	"fmt"
	"net/http"

	"iml-daemon/apps"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"iml-daemon/services/events"
)

func (svc *AppService) RegisterLocalAppInstance(request *AppInstanceRegistrationRequest) (*models.AppInstance, services.Error) {
	// Check if the instance is already registered
	// If it is, return its details
	appInstance, _ := svc.registry.FindAppInstanceByContainerID(request.ContainerID)
	if appInstance != nil {
		return appInstance, nil
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

	appDetails, err := svc.appFactory.NewLocalInstance(request.ApplicationID, appIP, request.ContainerID, ifaceName)
	if err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to create app instance %s: %v", request.ContainerID, err)
	}

	return appDetails, nil
}


func (svc *AppService) TeardownAppInstance(request *AppInstanceTeardownRequest) services.Error {
	// First, find the application instance by container ID
	// If it does not exist, just return
	appInstance, _ := svc.registry.FindAppInstanceByContainerID(request.ContainerID)
	if appInstance == nil {
		return nil
	}

	if err := svc.appFactory.DeleteInstance(appInstance); err != nil {
		return services.Errorf(
			http.StatusInternalServerError,
			"failed to delete app instance %s: %v", request.ContainerID, err)
	}

	return nil
}

func (svc *AppService) Shutdown(ctx context.Context) error {
	// Place any necessary cleanup logic here
	// For now, this is a no-op
	return nil
}

func InitializeAppService(
	registry *db.Registry, appIP, vnfIP *helpers.IPAllocator, eb *events.EventBus, appFactory *apps.InstanceFactory) (*AppService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Initialize the application service with the provided registry
	return &AppService{
		registry:   registry,
		appIP:      appIP,
		vnfIP:      vnfIP,
		eventBus:   eb,
		appFactory: appFactory,
	}, nil
}
