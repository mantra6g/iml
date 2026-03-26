package db

import (
	"fmt"
	"iml-daemon/logger"
	"iml-daemon/models"

	"github.com/google/uuid"
)

func (r *InMemoryRegistry) FindActiveAppByLocalID(localID uuid.UUID) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "id = ? AND status = ?", localID, models.AppStatusActive).Error; err != nil {
		return nil, fmt.Errorf("application with local ID %s not found: %w", localID, err)
	}
	return &app, nil
}

func (r *InMemoryRegistry) FindActiveAppByGlobalID(globalID string) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "global_id = ? AND status = ?", globalID, models.AppStatusActive).Error; err != nil {
		return nil, fmt.Errorf("application with global ID %s not found: %w", globalID, err)
	}
	return &app, nil
}

func (r *InMemoryRegistry) FindLocalAppGroupByGlobalID(globalAppID string) (*models.AppGroup, error) {
	// As the app group model does not have global_id directly, we obtain it by joining with the applications table.
	var group models.AppGroup
	if err := r.dbHandle.Joins("JOIN applications ON applications.id = app_groups.app_id").Where("applications.global_id = ?", globalAppID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("local app group with global ID %s not found: %w", globalAppID, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindAppGroupByID(id uuid.UUID) (*models.AppGroup, error) {
	var group models.AppGroup
	if err := r.dbHandle.First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("app group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindAppInstanceByContainerID(containerID string) (*models.AppInstance, error) {
	var instance models.AppInstance
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("app instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *InMemoryRegistry) FindActiveNetworkFunctionByLocalID(localID uuid.UUID) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.First(&vnf, "id = ? AND status = ?", localID, models.VNFStatusActive).Error; err != nil {
		return nil, fmt.Errorf("VNF with local ID %s not found: %w", localID, err)
	}
	return &vnf, nil
}

func (r *InMemoryRegistry) FindActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.First(&vnf, "global_id = ? AND status = ?", globalID, models.VNFStatusActive).Error; err != nil {
		return nil, fmt.Errorf("VNF with global ID %s not found: %w", globalID, err)
	}
	return &vnf, nil
}

func (r *InMemoryRegistry) FindCompleteActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.Preload("Subfunctions").First(&vnf, "global_id = ? AND status = ?", globalID, models.VNFStatusActive).Error; err != nil {
		return nil, fmt.Errorf("VNF with global ID %s not found: %w", globalID, err)
	}
	return &vnf, nil
}

// func (r *InMemoryRegistry) FindVnfInstancesByVNFID(vnfID uint) ([]*models.SimpleVnfInstance, error) {
// 	var instances []*models.SimpleVnfInstance
// 	if err := r.dbHandle.Find(&instances, "vnf_id = ?", vnfID).Error; err != nil {
// 		return nil, fmt.Errorf("failed to find VNF instances for VNF ID %d: %w", vnfID, err)
// 	}
// 	return instances, nil
// }

func (r *InMemoryRegistry) FindVnfInstanceByContainerID(containerID string) (*models.SimpleVnfInstance, error) {
	var instance models.SimpleVnfInstance
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("VNF instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *InMemoryRegistry) FindVnfMultiplexedInstanceByContainerID(containerID string) (*models.MultiplexedVnfInstance, error) {
	var instance models.MultiplexedVnfInstance
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("multiplexed VNF instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *InMemoryRegistry) FindVnfGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.SimpleVnfGroup, error) {
	var group models.SimpleVnfGroup
	if err := r.dbHandle.Preload("Instances").First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("VNF group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindSubfunctionPreloadedVnfMultiplexedGroupByID(id uuid.UUID) (*models.MultiplexedVnfGroup, error) {
	var group models.MultiplexedVnfGroup
	if err := r.dbHandle.Preload("SidAssignments").First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("multiplexed VNF group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindVnfMultiplexedGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.MultiplexedVnfGroup, error) {
	var group models.MultiplexedVnfGroup
	if err := r.dbHandle.Preload("Instances").First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("multiplexed VNF group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindLocalSimpleVnfGroupByVnfID(globalVnfID string) (*models.SimpleVnfGroup, error) {
	var group models.SimpleVnfGroup
	if err := r.dbHandle.Joins("JOIN virtual_network_functions ON virtual_network_functions.id = simple_vnf_groups.vnf_id").Where("virtual_network_functions.global_id = ?", globalVnfID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("local VNF group with VNF ID %s not found: %w", globalVnfID, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindLocalMultiplexedGroupByVnfID(globalVnfID string) (*models.MultiplexedVnfGroup, error) {
	var group models.MultiplexedVnfGroup
	if err := r.dbHandle.Joins("JOIN virtual_network_functions ON virtual_network_functions.id = multiplexed_vnf_groups.vnf_id").Where("virtual_network_functions.global_id = ?", globalVnfID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("local multiplexed VNF group with VNF ID %s not found: %w", globalVnfID, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindAllNetworkServiceChains() ([]models.ServiceChain, error) {
	var chains []models.ServiceChain
	if err := r.dbHandle.Preload("Elements").Find(&chains).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network service chains: %w", err)
	}
	return chains, nil
}

func (r *InMemoryRegistry) FindActiveNetworkServiceChainByGlobalID(globalChainID string) (*models.ServiceChain, error) {
	var chain models.ServiceChain
	if err := r.dbHandle.First(&chain, "global_id = ? AND status = ?", globalChainID, models.ServiceChainStatusActive).Error; err != nil {
		return nil, fmt.Errorf("network service chain with global ID %s not found: %w", globalChainID, err)
	}
	return &chain, nil
}

func (r *InMemoryRegistry) FindActiveNodeByGlobalID(globalNodeID string) (*models.Worker, error) {
	var node models.Worker
	if err := r.dbHandle.First(&node, "global_id = ? AND status = ?", globalNodeID, models.WorkerStatusActive).Error; err != nil {
		return nil, fmt.Errorf("node with global ID %s not found: %w", globalNodeID, err)
	}
	return &node, nil
}

func (r *InMemoryRegistry) FindRemoteAppGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteAppGroup, error) {
	var group models.RemoteAppGroup
	if err := r.dbHandle.First(&group, "node_id = ? AND external_group_id = ?", nodeID, groupID).Error; err != nil {
		return nil, fmt.Errorf("remote app group with node ID %s and external ID %s not found: %w", nodeID, groupID, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) FindRemoteVnfGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteVnfGroup, error) {
	var group models.RemoteVnfGroup
	if err := r.dbHandle.First(&group, "node_id = ? AND external_group_id = ?", nodeID, groupID).Error; err != nil {
		return nil, fmt.Errorf("remote VNF group with node ID %s and external ID %s not found: %w", nodeID, groupID, err)
	}
	return &group, nil
}

func (r *InMemoryRegistry) SaveApp(app *models.Application) error {
	if err := r.dbHandle.Save(app).Error; err != nil {
		return fmt.Errorf("failed to save application: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveAppGroup(group *models.AppGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save app group: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveAppInstance(instance *models.AppInstance) error {
	if err := r.dbHandle.Save(instance).Error; err != nil {
		return fmt.Errorf("failed to save app instance: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveVnf(function *models.VirtualNetworkFunction) error {
	if err := r.dbHandle.Save(function).Error; err != nil {
		return fmt.Errorf("failed to save virtual network function: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveSubfunctions(subfunctions []models.Subfunction) error {
	logger.DebugLogger().Printf("Saving %d subfunctions", len(subfunctions))
	for _, sf := range subfunctions {
		logger.DebugLogger().Printf("Saving subfunction: %+v", sf)
		if err := r.dbHandle.Save(&sf).Error; err != nil {
			return fmt.Errorf("failed to save subfunction: %w", err)
		}
	}
	return nil
}

func (r *InMemoryRegistry) SaveVnfGroup(group *models.SimpleVnfGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save VNF group: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveVnfMultiplexedGroup(group *models.MultiplexedVnfGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save multiplexed VNF group: %w", err)
	}
	return nil
}

// func (r *InMemoryRegistry) SaveSidAssignments(assignments []models.SidAssignment) error {
// 	for _, assignment := range assignments {
// 		if err := r.dbHandle.Save(&assignment).Error; err != nil {
// 			return fmt.Errorf("failed to save SID assignment: %w", err)
// 		}
// 	}
// 	return nil
// }

func (r *InMemoryRegistry) SaveVnfInstance(instance *models.SimpleVnfInstance) error {
	if err := r.dbHandle.Save(instance).Error; err != nil {
		return fmt.Errorf("failed to save VNF instance: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveVnfMultiplexedInstance(instance *models.MultiplexedVnfInstance) error {
	if err := r.dbHandle.Save(instance).Error; err != nil {
		return fmt.Errorf("failed to save multiplexed VNF instance: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveNetworkServiceChain(chain *models.ServiceChain) error {
	if err := r.dbHandle.Save(chain).Error; err != nil {
		return fmt.Errorf("failed to save network service chain: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveAppInstance(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.AppInstance{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove app instance with ID %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveAppGroup(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.AppGroup{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove app group with ID %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveVnfInstance(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.SimpleVnfInstance{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete VNF instance %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveVnfMultiplexedInstance(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.MultiplexedVnfInstance{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete multiplexed VNF instance %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveVnfGroup(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.SimpleVnfGroup{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove VNF group with ID %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveVnfMultiplexedGroup(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.MultiplexedVnfGroup{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove multiplexed VNF group with ID %s: %w", id, err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveRemoteAppGroup(group *models.RemoteAppGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save remote app group: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveRemoteVnfGroup(group *models.RemoteVnfGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save remote VNF group: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) SaveNode(node *models.Worker) error {
	if err := r.dbHandle.Save(node).Error; err != nil {
		return fmt.Errorf("failed to save node: %w", err)
	}
	return nil
}

func (r *InMemoryRegistry) MarkAppAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.Application{}).Where("global_id = ? AND status = ?", globalID, models.AppStatusActive).Update("status", models.AppStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark application %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *InMemoryRegistry) MarkVnfAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.VirtualNetworkFunction{}).Where("global_id = ? AND status = ?", globalID, models.VNFStatusActive).Update("status", models.VNFStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark VNF %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *InMemoryRegistry) MarkServiceChainAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.ServiceChain{}).Where("global_id = ? AND status = ?", globalID, models.ServiceChainStatusActive).Update("status", models.ServiceChainStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark service chain %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *InMemoryRegistry) MarkNodeAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.Worker{}).Where("global_id = ? AND status = ?", globalID, models.WorkerStatusActive).Update("status", models.WorkerStatusInactive).Error; err != nil {
		return fmt.Errorf("failed to mark node %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveRemoteAppInstancesByIP(ips []string, groupID uuid.UUID) error {
	if len(ips) == 0 {
		return nil
	}
	if err := r.dbHandle.Delete(&models.RemoteAppInstance{}, "group_id = ? AND ip IN ?", groupID, ips).Error; err != nil {
		return fmt.Errorf("failed to remove remote app instances by IPs %v: %w", ips, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveRemoteAppGroupsByGlobalAppID(globalAppID string) error {
	if err := r.dbHandle.Delete(&models.RemoteAppGroup{}, "app_id = ?", globalAppID).Error; err != nil {
		return fmt.Errorf("failed to remove remote app groups for App ID %s: %w", globalAppID, err)
	}
	return nil
}

func (r *InMemoryRegistry) RemoveRemoteVnfGroupsByGlobalVnfID(globalVnfID string) error {
	if err := r.dbHandle.Delete(&models.RemoteVnfGroup{}, "vnf_id = ?", globalVnfID).Error; err != nil {
		return fmt.Errorf("failed to remove remote VNF groups for VNF ID %s: %w", globalVnfID, err)
	}
	return nil
}
