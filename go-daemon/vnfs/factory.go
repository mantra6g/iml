package vnfs

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/services/router"
)

type InstanceFactory struct {
	repo        *db.Registry
	bus         *events.EventBus
	dataplane   *router.Dataplane
	imlClient   *iml.Client
}

func NewInstanceFactory(repo *db.Registry, bus *events.EventBus, dataplane *router.Dataplane, imlClient *iml.Client) (*InstanceFactory, error) {
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}
	if repo == nil {
		return nil, fmt.Errorf("repository is required")
	}
	if dataplane == nil {
		return nil, fmt.Errorf("dataplane is required")
	}

	return &InstanceFactory{
		repo:        repo,
		bus:         bus,
		dataplane:   dataplane,
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
		vnfGroup = &models.VnfGroup{
			VnfID:     vnf.ID,
			Instances: []models.VnfInstance{},
		}
		if err := f.repo.SaveVnfGroup(vnfGroup); err != nil {
			return nil, fmt.Errorf("failed to save new VNF group for %s: %v", nfUID, err)
		}

		// Create a new VNF group if it doesn't exist
		subnet, err := f.dataplane.AddVNFSubnet(vnfGroup.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to request subnet for VNF %s: %v", nfUID, err)
		}
		vnfGroup.SID = subnet.SID.String()
		vnfGroup.Subnet = subnet.Network.String()
		vnfGroup.GatewayIP = subnet.GatewayIP.String()
		vnfGroup.Bridge = subnet.Bridge.Attrs().Name
		if err := f.repo.SaveVnfGroup(vnfGroup); err != nil {
			return nil, fmt.Errorf("failed to update VNF group %s with sid %s: %v", vnfGroup.ID, vnfGroup.SID, err)
		}

		f.bus.Publish(events.Event{
			Name:    events.EventLocalVnfGroupCreated,
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

	instanceIPNet, err := f.dataplane.AddVNFInstance(vnfGroup.ID, instance.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to add VNF instance to dataplane: %v", err)
	}
	instance.IP = instanceIPNet.String()
	if err := f.repo.SaveVnfInstance(instance); err != nil {
		return nil, fmt.Errorf("failed to update VNF instance with IP %s: %v", instance.IP, err)
	}
	return instance, nil
}

func (f *InstanceFactory) DeleteInstance(instance *models.VnfInstance) error {
	// Remove the instance from the dataplane
	if err := f.dataplane.RemoveVNFInstance(instance.GroupID, instance.ID); err != nil {
		return fmt.Errorf("failed to remove VNF instance from dataplane: %v", err)
	}

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
		f.dataplane.RemoveVNFSubnet(vnfGroup.ID)
		if err := f.repo.RemoveVnfGroup(vnfGroup.ID); err != nil {
			return fmt.Errorf("failed to delete VNF group %s: %v", vnfGroup.ID, err)
		}
		f.bus.Publish(events.Event{
			Name:    events.EventLocalVnfGroupRemoved,
			Payload: *vnfGroup,
		})
	}
	return nil
}
