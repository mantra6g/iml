package apps

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/services/router"
)

type InstanceFactory struct {
	repo      *db.Registry
	bus       *events.EventBus
	imlClient *iml.Client
	dataplane *router.Dataplane
}

func NewInstanceFactory(
	repo *db.Registry, 
	bus *events.EventBus, 
	dataplane *router.Dataplane,
	imlClient *iml.Client) (*InstanceFactory, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}

	return &InstanceFactory{
		repo:      repo,
		bus:       bus,
		imlClient: imlClient,
		dataplane: dataplane,
	}, nil
}

func (f *InstanceFactory) NewLocalInstance(appUID string, containerID string, ifaceName string) (*models.AppInstance, error) {
	app, err := f.imlClient.GetApplication(appUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get application %s: %v", appUID, err)
	}

	appGroup, _ := f.repo.FindLocalAppGroupByGlobalID(appUID)
	if appGroup == nil {
		// Create a new app group if it doesn't exist
		appGroup = &models.AppGroup{
			AppID:     app.ID,
			Instances: []models.AppInstance{},
		}
		if err := f.repo.SaveAppGroup(appGroup); err != nil {
			return nil, fmt.Errorf("failed to save new app group for %s: %v", appUID, err)
		}

		subnet, err := f.dataplane.AddApplicationSubnet(appGroup.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to request application subnet: %v", err)
		}
		appGroup.Subnet = subnet.Network.String()
		appGroup.GatewayIP = subnet.GatewayIP.String()
		appGroup.Bridge = subnet.Bridge.Attrs().Name
		if err := f.repo.SaveAppGroup(appGroup); err != nil {
			return nil, fmt.Errorf("failed to update app group with subnet info: %v", err)
		}
		logger.DebugLogger().Printf("Created new app group with subnet %s and gateway %s for application %s", appGroup.Subnet, appGroup.GatewayIP, appUID)

		f.bus.Publish(events.Event{
			Name:    events.EventLocalAppGroupCreated,
			Payload: *appGroup,
		})
	}

	instance := &models.AppInstance{
		GroupID:     appGroup.ID,
		ContainerID: containerID,
		IfaceName:   ifaceName,
	}
	if err := f.repo.SaveAppInstance(instance); err != nil {
		return nil, fmt.Errorf("failed to save app instance: %v", err)
	}

	instanceIPNet, err := f.dataplane.AddApplicationInstance(appGroup.ID, instance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to add application instance to dataplane: %v", err)
	}
	instance.IP = instanceIPNet.String()
	if err := f.repo.SaveAppInstance(instance); err != nil {
		return nil, fmt.Errorf("failed to update app instance with IP %s: %v", instance.IP, err)
	}
	return instance, nil
}

func (f *InstanceFactory) DeleteInstance(instance *models.AppInstance) error {
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
