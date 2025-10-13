package vnfs

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/helpers"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
)

type InstanceFactory struct {
	repo        *db.Registry
	bus         *events.EventBus
	ipAllocator *helpers.IPAllocator
	imlClient   *iml.Client
}

func NewInstanceFactory(repo *db.Registry, bus *events.EventBus, ipAllocator *helpers.IPAllocator, imlClient *iml.Client) (*InstanceFactory, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if ipAllocator == nil {
		return nil, fmt.Errorf("IP allocator is required")
	}

	return &InstanceFactory{
		repo:        repo,
		bus:         bus,
		ipAllocator: ipAllocator,
		imlClient:   imlClient,
	}, nil
}

func (f *InstanceFactory) NewLocalInstance(nfUID string, containerID string, ifaceName string) (*models.VnfInstance, error) {
	vnf, err := f.imlClient.GetNetworkFunction(nfUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VNF %s: %v", nfUID, err)
	}

	vnfGroup, _ := f.repo.FindLocalVnfGroupByVnfID(nfUID)
	if vnfGroup == nil {
		// Create a new VNF group if it doesn't exist
		groupSID, err := f.ipAllocator.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate SID for VNF group of %s: %v", nfUID, err)
		}

		vnfGroup = &models.VnfGroup{
			VnfID:     vnf.ID,
			SID:       groupSID.String(),
			Instances: []models.VnfInstance{},
		}

		if err := f.repo.SaveVnfGroup(vnfGroup); err != nil {
			return nil, fmt.Errorf("failed to save new VNF group for %s: %v", nfUID, err)
		}

		f.bus.Publish(events.Event{
			Name:    events.EventVnfGroupCreated,
			Payload: *vnfGroup,
		})
	}
	instance := &models.VnfInstance{
		GroupID:     vnfGroup.ID,
		ContainerID: containerID,
		IfaceName:   ifaceName,
	}

	if err := f.repo.SaveVnfInstance(instance); err != nil {
		return nil, fmt.Errorf("failed to save VNF instance: %v", err)
	}
	return instance, nil
}

func (f *InstanceFactory) DeleteInstance(instance *models.VnfInstance) error {
	// Delete the instance from the registry
	if err := f.repo.RemoveVnfInstance(instance.ID); err != nil {
		return fmt.Errorf("failed to delete VNF instance %s: %v", instance.ID, err)
	}

	vnfGroup, err := f.repo.FindVnfGroupByID(instance.GroupID)
	if err != nil {
		return fmt.Errorf("VNF group of instance %s not found: %v", instance.ID, err)
	}

	if len(vnfGroup.Instances) == 0 {
		// If there are no more instances, delete the VNF group
		if err := f.repo.RemoveVnfGroup(vnfGroup.ID); err != nil {
			return fmt.Errorf("failed to delete VNF group %s: %v", vnfGroup.ID, err)
		}
	}

	f.bus.Publish(events.Event{
		Name:    events.EventVnfGroupRemoved,
		Payload: *vnfGroup,
	})

	return nil
}
