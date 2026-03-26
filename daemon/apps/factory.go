package apps

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/services/router/dataplane"
)

type InstanceFactoryImpl struct {
	repo      db.Registry
	bus       events.EventBus
	imlClient iml.Client
	dataplane dataplane.Manager
}

func NewInstanceFactory(
	repo db.Registry,
	bus events.EventBus,
	dataplane dataplane.Manager,
	imlClient iml.Client) (InstanceFactory, error) {
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if dataplane == nil {
		return nil, fmt.Errorf("dataplane manager is required")
	}
	if imlClient == nil {
		return nil, fmt.Errorf("IML client is required")
	}

	return &InstanceFactoryImpl{
		repo:      repo,
		bus:       bus,
		imlClient: imlClient,
		dataplane: dataplane,
	}, nil
}

func (f *InstanceFactoryImpl) NewLocalInstance(req *RegistrationRequest) (*InstanceRegistrationResponse, error) {
	app, err := f.imlClient.GetApplication(req.ApplicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application %s: %v", req.ApplicationID, err)
	}

	globalConfig, err := env.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to get global config: %v", err)
	}

	response := &InstanceRegistrationResponse{}

	appInstance, appGroup, err := f.createOrGetAppInstance(req, app)
	if err != nil {
		return nil, fmt.Errorf("failed to create or get app instance: %v", err)
	}
	response.IPNet = appInstance.GetIP()
	response.IfaceName = appInstance.IfaceName
	response.ClusterCIDR = *globalConfig.ClusterPoolIPv4CIDR
	response.GatewayIP = appGroup.GetGatewayIP()
	response.BridgeName = appGroup.Bridge

	return response, nil
}

func (f *InstanceFactoryImpl) createOrGetAppInstance(req *RegistrationRequest, app *models.Application) (*models.AppInstance, *models.AppGroup, error) {
	appGroup, _ := f.repo.FindLocalAppGroupByGlobalID(app.GlobalID)
	if appGroup == nil {
		// Create a new app group if it doesn't exist
		appGroup = &models.AppGroup{
			AppID:     app.ID,
			Instances: []models.AppInstance{},
		}
		if err := f.repo.SaveAppGroup(appGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to save new app group for %s: %v", req.ApplicationID, err)
		}

		subnet, err := f.dataplane.AddApplicationSubnet(appGroup.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to request application subnet: %v", err)
		}
		appGroup.Subnet = subnet.Network().String()
		appGroup.GatewayIP = subnet.GatewayIP().String()
		appGroup.Bridge = subnet.Bridge()
		if err := f.repo.SaveAppGroup(appGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to update app group with subnet info: %v", err)
		}
		logger.DebugLogger().Printf("Created new app group with subnet %s and gateway %s for application %s", appGroup.Subnet, appGroup.GatewayIP, req.ApplicationID)

		f.bus.Publish(events.Event{
			Name:    events.EventLocalAppGroupCreated,
			Payload: *appGroup,
		})
	}
	// Check if the instance is already registered
	// If it is, return its details
	appInstance, _ := f.repo.FindAppInstanceByContainerID(req.ContainerID)
	if appInstance != nil {
		return appInstance, appGroup, nil
	}

	instance := &models.AppInstance{
		GroupID:     appGroup.ID,
		ContainerID: req.ContainerID,
	}
	if err := f.repo.SaveAppInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to save app instance: %v", err)
	}

	instanceIPNet, ifaceName, err := f.dataplane.AddApplicationInstance(appGroup.ID, instance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add application instance to dataplane: %v", err)
	}
	instance.IP = instanceIPNet.String()
	instance.IfaceName = ifaceName

	if err := f.repo.SaveAppInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to update app instance with IP %s: %v", instance.IP, err)
	}
	return instance, appGroup, nil
}

func (f *InstanceFactoryImpl) DeleteInstance(containerID string) error {
	// First, find the application instance by container ID
	// If it does not exist, just return
	instance, _ := f.repo.FindAppInstanceByContainerID(containerID)
	if instance == nil {
		return nil
	}

	// Remove the instance from the dataplane
	if err := f.dataplane.RemoveApplicationInstance(instance.GroupID, instance.ID); err != nil {
		return fmt.Errorf("failed to remove application instance from dataplane: %v", err)
	}

	// Delete the instance from the registry
	if err := f.repo.RemoveAppInstance(instance.ID); err != nil {
		return fmt.Errorf("failed to delete app instance %s: %v", instance.ID, err)
	}

	// Fetch the app group to which this instance belongs
	appGroup, err := f.repo.FindAppGroupByID(instance.GroupID)
	if err != nil {
		return fmt.Errorf("app group of instance %s not found: %v", instance.ID, err)
	}

	if len(appGroup.Instances) == 0 {
		// If there are no more instances, delete the app group
		f.dataplane.RemoveApplicationSubnet(appGroup.ID)
		if err := f.repo.RemoveAppGroup(appGroup.ID); err != nil {
			return fmt.Errorf("failed to delete app group %s: %v", appGroup.ID, err)
		}
		f.bus.Publish(events.Event{
			Name:    events.EventLocalAppGroupRemoved,
			Payload: *appGroup,
		})
	}
	return nil
}
