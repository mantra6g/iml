package apps

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"net"
)

type InstanceFactory struct {
	repo      *db.Registry
	bus       *events.EventBus
	imlClient *iml.Client
}

func NewInstanceFactory(repo *db.Registry, bus *events.EventBus, imlClient *iml.Client) (*InstanceFactory, error) {
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
	}, nil
}

func (f *InstanceFactory) NewLocalInstance(appUID string, instanceIP *net.IPNet, containerID string, ifaceName string) (*models.AppInstance, error) {
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

		f.bus.Publish(events.Event{
			Name:    events.EventLocalAppGroupCreated,
			Payload: *appGroup,
		})
	}
	instance := &models.AppInstance{
		GroupID:     appGroup.ID,
		ContainerID: containerID,
		IfaceName:   ifaceName,
		IP:          instanceIP.String(),
	}

	if err := f.repo.SaveAppInstance(instance); err != nil {
		return nil, fmt.Errorf("failed to save app instance: %v", err)
	}

	return instance, nil
}

func (f *InstanceFactory) DeleteInstance(instance *models.AppInstance) error {
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
		f.bus.Publish(events.Event{
			Name:    events.EventLocalAppGroupRemoved,
			Payload: *appGroup,
		})

		if err := f.repo.RemoveAppGroup(appGroup.ID); err != nil {
			return fmt.Errorf("failed to delete app group %s: %v", appGroup.ID, err)
		}
	}
	return nil
}
