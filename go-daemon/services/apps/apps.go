package apps

import (
	"context"
	"fmt"
	"net/http"

	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services"
	"iml-daemon/services/eventbus"
)

func (svc *AppService) RegisterLocalAppInstance(request *AppInstanceRegistrationRequest) (*models.AppInstance, services.Error) {
	// Check if the instance is already registered
	// If it is, return its details
	appInstance, _ := svc.registry.FindAppInstanceByContainerID(request.ContainerID)
	if appInstance != nil {
		return appInstance, nil
	}

	// Verify that the application exists in the registry
	app, err := svc.registry.FindAppByGlobalID(request.ApplicationID)
	if err != nil {
		return nil, services.Errorf(
			http.StatusNotFound,
			"application %s not found: %v", request.ApplicationID, err)
	}

	// Check if there is already an App Group for this app
	appGroup, _ := svc.registry.FindLocalAppGroupByGlobalID(request.ApplicationID)
	var errDetails services.Error
	if appGroup == nil {
		appGroup, errDetails = svc.RegisterLocalAppGroup(app)
		if errDetails != nil {
			return nil, errDetails
		}
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
		GroupID:     appGroup.ID,
		ContainerID: request.ContainerID,
		IfaceName:   ifaceName,
		IP:          appIP.String(),
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

func (svc *AppService) RegisterLocalAppGroup(app *models.Application) (*models.AppGroup, services.Error) {
	appGroup := &models.AppGroup{
		AppID:     app.ID,
		WorkerID:  nil,
	}

	if err := svc.registry.SaveAppGroup(appGroup); err != nil {
		return nil, services.Errorf(
			http.StatusInternalServerError,
			"failed to save app group %s: %v", app.GlobalID, err)
	}

	svc.eventBus.Publish(eventbus.Event{
		Name:    "AppGroupCreated",
		Payload: *appGroup,
	})

	return appGroup, nil
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

func (svc *AppService) Shutdown(ctx context.Context) error {
	// Place any necessary cleanup logic here
	// For now, this is a no-op
	return nil
}

func InitializeAppService(
	registry *db.Registry, appIP, vnfIP *helpers.IPAllocator, eb *eventbus.EventBus) (*AppService, error) {
	// Validate the registry
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	// Initialize the application service with the provided registry
	return &AppService{
		registry: registry,
		appIP: 	appIP,
		vnfIP: 	vnfIP,
		eventBus: eb,
	}, nil
}
