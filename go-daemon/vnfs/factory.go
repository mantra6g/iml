package vnfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"iml-daemon/services/iml"
	"iml-daemon/services/router/dataplane"
	"net"
	"net/http"

	"github.com/google/uuid"
)

type InstanceFactory struct {
	repo      db.Registry
	bus       events.EventBus
	dataplane dataplane.Manager
	imlClient iml.Client
}

type RegistrationRequest struct {
	VnfID       string
	ContainerID string
}

type InstanceRegistrationResponse struct {
	IPNet       net.IPNet
	SIDs        []net.IPNet
	IfaceName   string
	ClusterCIDR net.IPNet
	GatewayIP   net.IP
	BridgeName  string
}

func NewInstanceFactory(repo db.Registry, bus events.EventBus, dataplane dataplane.Manager, imlClient iml.Client) (*InstanceFactory, error) {
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
		repo:      repo,
		bus:       bus,
		dataplane: dataplane,
		imlClient: imlClient,
	}, nil
}

func (f *InstanceFactory) NewLocalInstance(req *RegistrationRequest) (*InstanceRegistrationResponse, error) {
	vnf, err := f.imlClient.GetNetworkFunction(req.VnfID)
	if err != nil {
		return nil, fmt.Errorf("failed to get VNF %s: %v", req.VnfID, err)
	}

	globalConfig, err := env.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to get global config: %v", err)
	}

	response := &InstanceRegistrationResponse{}

	switch vnf.Type {
	case models.NetworkFunctionTypeSimple:
		simpleInstance, simpleGroup, err := f.newLocalSimpleInstance(req.VnfID, vnf, req.ContainerID)
		response.IPNet = simpleInstance.GetIP()
		response.SIDs = []net.IPNet{simpleGroup.GetSID()}
		response.ClusterCIDR = *globalConfig.ClusterCIDR
		response.GatewayIP = simpleGroup.GetGatewayIP()
		response.BridgeName = simpleGroup.Bridge
		response.IfaceName = simpleInstance.IfaceName
		return response, err
	case models.NetworkFunctionTypeMultiplexed:
		multiInstance, multiGroup, err := f.newLocalMultiplexedInstance(req.VnfID, vnf, req.ContainerID)
		response.IPNet = multiInstance.GetIP()
		response.ClusterCIDR = *globalConfig.ClusterCIDR
		response.GatewayIP = multiGroup.GetGatewayIP()
		response.BridgeName = multiGroup.Bridge
		response.IfaceName = multiInstance.IfaceName
		for _, sidAssignment := range multiGroup.SidAssignments {
			response.SIDs = append(response.SIDs, sidAssignment.GetSID())
		}
		return response, err
	default:
		return nil, fmt.Errorf("unknown VNF type %s for VNF %s", vnf.Type, req.VnfID)
	}
}

func (f *InstanceFactory) newLocalSimpleInstance(nfUID string, vnf *models.VirtualNetworkFunction, containerID string) (*models.SimpleVnfInstance, *models.SimpleVnfGroup, error) {
	vnfGroup, _ := f.repo.FindLocalSimpleVnfGroupByVnfID(nfUID)
	if vnfGroup == nil {
		vnfGroup = &models.SimpleVnfGroup{
			BaseVnfGroup: models.BaseVnfGroup{
				VnfID: vnf.ID,
			},
			Instances: []models.SimpleVnfInstance{},
		}
		if err := f.repo.SaveVnfGroup(vnfGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to save new VNF group for %s: %v", nfUID, err)
		}

		// Create a new VNF group if it doesn't exist
		subnet, err := f.dataplane.AddVNFSubnet(vnfGroup.ID, 1)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to request subnet for VNF %s: %v", nfUID, err)
		}
		sid := subnet.SIDs()[0]
		vnfGroup.SID = sid.String()
		vnfGroup.Subnet = subnet.Network().String()
		vnfGroup.GatewayIP = subnet.GatewayIP().String()
		vnfGroup.Bridge = subnet.Bridge()
		if err := f.repo.SaveVnfGroup(vnfGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to update VNF group %s with sid %s: %v", vnfGroup.ID, vnfGroup.SID, err)
		}

		f.bus.Publish(events.Event{
			Name:    events.EventLocalSimpleVnfGroupCreated,
			Payload: *vnfGroup,
		})
	}
	instance, err := f.repo.FindVnfInstanceByContainerID(containerID)
	if err == nil && instance != nil {
		// Instance already exists, return it
		return instance, vnfGroup, nil
	}

	instance = &models.SimpleVnfInstance{
		BaseVnfInstance: models.BaseVnfInstance{
			GroupID:     vnfGroup.ID,
			ContainerID: containerID,
		},
	}
	if err := f.repo.SaveVnfInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to save VNF instance: %v", err)
	}

	instanceIPNet, ifaceName, err := f.dataplane.AddVNFInstance(vnfGroup.ID, instance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add VNF instance to dataplane: %v", err)
	}
	instance.IP = instanceIPNet.String()
	instance.IfaceName = ifaceName

	if err := f.repo.SaveVnfInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to update VNF instance with IP %s: %v", instance.IP, err)
	}
	return instance, vnfGroup, nil
}

func (f *InstanceFactory) newLocalMultiplexedInstance(nfUID string, vnf *models.VirtualNetworkFunction, containerID string) (*models.MultiplexedVnfInstance, *models.MultiplexedVnfGroup, error) {
	vnfGroup, _ := f.repo.FindLocalMultiplexedGroupByVnfID(nfUID)
	if vnfGroup == nil {
		vnfGroup = &models.MultiplexedVnfGroup{
			BaseVnfGroup: models.BaseVnfGroup{
				VnfID: vnf.ID,
			},
			Instances: []models.MultiplexedVnfInstance{},
		}
		if err := f.repo.SaveVnfMultiplexedGroup(vnfGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to save new multiplexed VNF group for %s: %v", nfUID, err)
		}

		// Create a new VNF group if it doesn't exist
		subnet, err := f.dataplane.AddVNFSubnet(vnfGroup.ID, len(vnf.Subfunctions))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to request subnet for VNF %s: %v", nfUID, err)
		}
		vnfGroup.Subnet = subnet.Network().String()
		vnfGroup.GatewayIP = subnet.GatewayIP().String()
		vnfGroup.Bridge = subnet.Bridge()

		sidAssignments := []models.SidAssignment{}
		for i, sid := range subnet.SIDs() {
			sidAssignments = append(sidAssignments, models.SidAssignment{
				VNFSubfunctionID:      vnf.Subfunctions[i].SubfunctionID,
				SID:                   sid.String(),
				VnfMultiplexedGroupID: vnfGroup.ID,
			})
		}
		vnfGroup.SidAssignments = sidAssignments

		if err := f.publishAssignmentsToP4Controller(nfUID, vnfGroup.ID, sidAssignments); err != nil {
			logger.ErrorLogger().Printf("failed to publish SID assignments to P4 controller for multiplexed VNF group %s: %v", vnfGroup.ID, err)
		}

		if err := f.repo.SaveVnfMultiplexedGroup(vnfGroup); err != nil {
			return nil, nil, fmt.Errorf("failed to update multiplexed VNF group %s: %v", vnfGroup.ID, err)
		}

		f.bus.Publish(events.Event{
			Name:    events.EventLocalVnfMultiplexedGroupCreated,
			Payload: *vnfGroup,
		})
	}
	instance, err := f.repo.FindVnfMultiplexedInstanceByContainerID(containerID)
	if err == nil && instance != nil {
		// Instance already exists, return it
		return instance, vnfGroup, nil
	}

	instance = &models.MultiplexedVnfInstance{
		BaseVnfInstance: models.BaseVnfInstance{
			GroupID:     vnfGroup.ID,
			ContainerID: containerID,
		},
	}
	if err := f.repo.SaveVnfMultiplexedInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to save multiplexed VNF instance: %v", err)
	}

	instanceIPNet, ifaceName, err := f.dataplane.AddVNFInstance(vnfGroup.ID, instance.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add multiplexed VNF instance to dataplane: %v", err)
	}
	instance.IP = instanceIPNet.String()
	instance.IfaceName = ifaceName

	if err := f.repo.SaveVnfMultiplexedInstance(instance); err != nil {
		return nil, nil, fmt.Errorf("failed to update multiplexed VNF instance with IP %s: %v", instance.IP, err)
	}
	return instance, vnfGroup, nil
}

func (f *InstanceFactory) TeardownVnfInstance(containerID string) error {
	instance, err := f.repo.FindVnfInstanceByContainerID(containerID)
	if err == nil && instance != nil {
		return f.deleteSimpleInstance(instance)
	}

	multiInstance, err := f.repo.FindVnfMultiplexedInstanceByContainerID(containerID)
	if err == nil && multiInstance != nil {
		return f.deleteMultiplexedInstance(multiInstance)
	}
	// If no instance found, nothing to do
	return nil
}

func (f *InstanceFactory) deleteSimpleInstance(instance *models.SimpleVnfInstance) error {
	// Remove the instance from the dataplane
	if err := f.dataplane.RemoveVNFInstance(instance.GroupID, instance.ID); err != nil {
		return fmt.Errorf("failed to remove VNF instance from dataplane: %v", err)
	}

	// Delete the instance from the registry
	if err := f.repo.RemoveVnfInstance(instance.ID); err != nil {
		return fmt.Errorf("failed to delete VNF instance %s: %v", instance.ID, err)
	}

	vnfGroup, err := f.repo.FindVnfGroupByIDWithPrepopulatedInstanceList(instance.GroupID)
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
			Name:    events.EventLocalSimpleVnfGroupRemoved,
			Payload: *vnfGroup,
		})
	}
	return nil
}

func (f *InstanceFactory) deleteMultiplexedInstance(instance *models.MultiplexedVnfInstance) error {
	// Remove the instance from the dataplane
	if err := f.dataplane.RemoveVNFInstance(instance.GroupID, instance.ID); err != nil {
		return fmt.Errorf("failed to remove VNF instance from dataplane: %v", err)
	}

	// Delete the instance from the registry
	if err := f.repo.RemoveVnfMultiplexedInstance(instance.ID); err != nil {
		return fmt.Errorf("failed to delete VNF instance %s: %v", instance.ID, err)
	}

	vnfGroup, err := f.repo.FindVnfMultiplexedGroupByIDWithPrepopulatedInstanceList(instance.GroupID)
	if err != nil {
		return fmt.Errorf("VNF group of instance %s not found: %v", instance.ID, err)
	}

	if len(vnfGroup.Instances) == 0 {
		// If there are no more instances, delete the VNF group
		f.dataplane.RemoveVNFSubnet(vnfGroup.ID)
		if err := f.repo.RemoveVnfMultiplexedGroup(vnfGroup.ID); err != nil {
			return fmt.Errorf("failed to delete VNF group %s: %v", vnfGroup.ID, err)
		}
		f.bus.Publish(events.Event{
			Name:    events.EventLocalSimpleVnfGroupRemoved,
			Payload: *vnfGroup,
		})
	}
	return nil
}

func (f *InstanceFactory) publishAssignmentsToP4Controller(vnfID string, groupID uuid.UUID, sidAssignments []models.SidAssignment) error {
	type AssignmentPayload struct {
		SubfunctionID uint32 `json:"subfunction_id"`
		SID           string `json:"sid"`
	}

	var payloads []AssignmentPayload
	for _, assignment := range sidAssignments {
		payloads = append(payloads, AssignmentPayload{
			SubfunctionID: assignment.VNFSubfunctionID,
			SID:           assignment.SID,
		})
	}

	p4ControllerURL := fmt.Sprintf("%s/api/v1/p4controller/assignments/%s/%s", env.P4_CONTROLLER_API_URL, vnfID, groupID.String())

	payloadBytes, err := json.Marshal(payloads)
	if err != nil {
		return fmt.Errorf("failed to marshal SID assignments payload: %v", err)
	}

	resp, err := http.Post(p4ControllerURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to send SID assignments to P4 controller: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("P4 controller returned non-2xx status: %s", resp.Status)
	}

	return nil
}
