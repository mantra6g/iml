package db

import (
	"fmt"
	"iml-daemon/models"

	"github.com/google/uuid"
)

func (r *Registry) FindAppById(id uuid.UUID) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("application with id %s not found: %w", id, err)
	}
	return &app, nil
}

func (r *Registry) FindActiveAppByLocalID(localID uuid.UUID) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "id = ? AND status = ?", localID, models.AppStatusActive).Error; err != nil {
		return nil, fmt.Errorf("application with local ID %s not found: %w", localID, err)
	}
	return &app, nil
}

func (r *Registry) FindActiveAppByGlobalID(globalID string) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "global_id = ? AND status = ?", globalID, models.AppStatusActive).Error; err != nil {
		return nil, fmt.Errorf("application with global ID %s not found: %w", globalID, err)
	}
	return &app, nil
}

func (r *Registry) FindAllApps() ([]*models.Application, error) {
	var apps []*models.Application
	if err := r.dbHandle.Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to find all applications: %w", err)
	}
	return apps, nil
}

func (r *Registry) FindLocalAppGroupByGlobalID(globalAppID string) (*models.AppGroup, error) {
	// As the app group model does not have global_id directly, we obtain it by joining with the applications table.
	var group models.AppGroup
	if err := r.dbHandle.Joins("JOIN applications ON applications.id = app_groups.app_id").Where("applications.global_id = ?", globalAppID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("local app group with global ID %s not found: %w", globalAppID, err)
	}
	return &group, nil
}

func (r *Registry) FindAppGroupByID(id uuid.UUID) (*models.AppGroup, error) {
	var group models.AppGroup
	if err := r.dbHandle.First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("app group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *Registry) FindAppInstanceByContainerID(containerID string) (*models.AppInstance, error) {
	var instance models.AppInstance
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("app instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *Registry) FindAppInstancesByAppID(appID string) ([]*models.AppInstance, error) {
	var instances []*models.AppInstance
	if err := r.dbHandle.Find(&instances, "app_id = ?", appID).Error; err != nil {
		return nil, fmt.Errorf("failed to find app instances for app ID %s: %w", appID, err)
	}
	return instances, nil
}

func (r *Registry) FindAppInstancesByGroupID(groupID uuid.UUID) ([]*models.AppInstance, error) {
	var instances []*models.AppInstance
	if err := r.dbHandle.Find(&instances, "group_id = ?", groupID).Error; err != nil {
		return nil, fmt.Errorf("failed to find app instances for group ID %s: %w", groupID, err)
	}
	return instances, nil
}

func (r *Registry) FindAllNetworkFunctions() ([]*models.VirtualNetworkFunction, error) {
	var functions []*models.VirtualNetworkFunction
	if err := r.dbHandle.Find(&functions).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network functions: %w", err)
	}
	return functions, nil
}

func (r *Registry) FindNetworkFunctionByID(id uuid.UUID) (*models.VirtualNetworkFunction, error) {
	var function models.VirtualNetworkFunction
	if err := r.dbHandle.First(&function, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("network function with id %s not found: %w", id, err)
	}
	return &function, nil
}

func (r *Registry) FindActiveNetworkFunctionByLocalID(localID uuid.UUID) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.First(&vnf, "id = ? AND status = ?", localID, models.VNFStatusActive).Error; err != nil {
		return nil, fmt.Errorf("VNF with local ID %s not found: %w", localID, err)
	}
	return &vnf, nil
}

func (r *Registry) FindActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.First(&vnf, "global_id = ? AND status = ?", globalID, models.VNFStatusActive).Error; err != nil {
		return nil, fmt.Errorf("VNF with global ID %s not found: %w", globalID, err)
	}
	return &vnf, nil
}

func (r *Registry) FindVnfInstancesByVNFID(vnfID uint) ([]*models.VnfInstance, error) {
	var instances []*models.VnfInstance
	if err := r.dbHandle.Find(&instances, "vnf_id = ?", vnfID).Error; err != nil {
		return nil, fmt.Errorf("failed to find VNF instances for VNF ID %d: %w", vnfID, err)
	}
	return instances, nil
}

func (r *Registry) FindVnfInstanceByContainerID(containerID string) (*models.VnfInstance, error) {
	var instance models.VnfInstance
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("VNF instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *Registry) FindVnfGroupByID(id uuid.UUID) (*models.VnfGroup, error) {
	var group models.VnfGroup
	if err := r.dbHandle.First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("VNF group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *Registry) FindVnfGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.VnfGroup, error) {
	var group models.VnfGroup
	if err := r.dbHandle.Preload("Instances").First(&group, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("VNF group with id %s not found: %w", id, err)
	}
	return &group, nil
}

func (r *Registry) FindLocalVnfGroupByVnfID(globalVnfID string) (*models.VnfGroup, error) {
	var group models.VnfGroup
	if err := r.dbHandle.Joins("JOIN virtual_network_functions ON virtual_network_functions.id = vnf_groups.vnf_id").Where("virtual_network_functions.global_id = ?", globalVnfID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("local VNF group with VNF ID %s not found: %w", globalVnfID, err)
	}
	return &group, nil
}

func (r *Registry) FindAllNetworkServiceChains() ([]models.ServiceChain, error) {
	var chains []models.ServiceChain
	if err := r.dbHandle.Preload("Elements").Find(&chains).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network service chains: %w", err)
	}
	return chains, nil
}

func (r *Registry) FindActiveNetworkServiceChainByGlobalID(globalChainID string) (*models.ServiceChain, error) {
	var chain models.ServiceChain
	if err := r.dbHandle.First(&chain, "global_id = ? AND status = ?", globalChainID, models.ServiceChainStatusActive).Error; err != nil {
		return nil, fmt.Errorf("network service chain with global ID %s not found: %w", globalChainID, err)
	}
	return &chain, nil
}

func (r *Registry) FindActiveNodeByGlobalID(globalNodeID string) (*models.Worker, error) {
	var node models.Worker
	if err := r.dbHandle.First(&node, "global_id = ? AND status = ?", globalNodeID, models.WorkerStatusActive).Error; err != nil {
		return nil, fmt.Errorf("node with global ID %s not found: %w", globalNodeID, err)
	}
	return &node, nil
}

func (r *Registry) FindRemoteAppGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteAppGroup, error) {
	var group models.RemoteAppGroup
	if err := r.dbHandle.First(&group, "node_id = ? AND external_group_id = ?", nodeID, groupID).Error; err != nil {
		return nil, fmt.Errorf("remote app group with node ID %s and external ID %s not found: %w", nodeID, groupID, err)
	}
	return &group, nil
}

func (r *Registry) FindRemoteVnfGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteVnfGroup, error) {
	var group models.RemoteVnfGroup
	if err := r.dbHandle.First(&group, "node_id = ? AND external_group_id = ?", nodeID, groupID).Error; err != nil {
		return nil, fmt.Errorf("remote VNF group with node ID %s and external ID %s not found: %w", nodeID, groupID, err)
	}
	return &group, nil
}

func (r *Registry) SaveApp(app *models.Application) error {
	if err := r.dbHandle.Save(app).Error; err != nil {
		return fmt.Errorf("failed to save application: %w", err)
	}
	return nil
}

func (r *Registry) SaveAppGroup(group *models.AppGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save app group: %w", err)
	}
	return nil
}

func (r *Registry) SaveAppInstance(instance *models.AppInstance) error {
	if err := r.dbHandle.Save(instance).Error; err != nil {
		return fmt.Errorf("failed to save app instance: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnf(function *models.VirtualNetworkFunction) error {
	if err := r.dbHandle.Save(function).Error; err != nil {
		return fmt.Errorf("failed to save virtual network function: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnfGroup(group *models.VnfGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save VNF group: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnfInstance(instance *models.VnfInstance) error {
	if err := r.dbHandle.Save(instance).Error; err != nil {
		return fmt.Errorf("failed to save VNF instance: %w", err)
	}
	return nil
}

func (r *Registry) SaveNetworkServiceChain(chain *models.ServiceChain) error {
	if err := r.dbHandle.Save(chain).Error; err != nil {
		return fmt.Errorf("failed to save network service chain: %w", err)
	}
	return nil
}

func (r *Registry) SaveChainElement(element *models.ServiceChainVnfs) error {
	if err := r.dbHandle.Save(element).Error; err != nil {
		return fmt.Errorf("failed to save chain element: %w", err)
	}
	return nil
}

func (r *Registry) SaveRoute(route *models.Route) error {
	if err := r.dbHandle.Save(route).Error; err != nil {
		return fmt.Errorf("failed to save route: %w", err)
	}
	return nil
}

func (r *Registry) SaveRemoteAppInstances(instances []*models.RemoteAppInstance) error {
	for _, instance := range instances {
		if err := r.dbHandle.Save(instance).Error; err != nil {
			return fmt.Errorf("failed to save remote app instance: %w", err)
		}
	}
	return nil
}

func (r *Registry) RemoveAppInstanceByContainerID(containerID string) error {
	if err := r.dbHandle.Delete(&models.AppInstance{}, "container_id = ?", containerID).Error; err != nil {
		return fmt.Errorf("failed to remove app instance with container ID %s: %w", containerID, err)
	}
	return nil
}

func (r *Registry) RemoveAppInstance(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.AppInstance{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove app instance with ID %s: %w", id, err)
	}
	return nil
}

func (r *Registry) RemoveAppGroup(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.AppGroup{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove app group with ID %s: %w", id, err)
	}
	return nil
}

func (r *Registry) RemoveVnfInstance(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.VnfInstance{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete VNF instance %s: %w", id, err)
	}
	return nil
}

func (r *Registry) RemoveVnfGroup(id uuid.UUID) error {
	if err := r.dbHandle.Delete(&models.VnfGroup{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to remove VNF group with ID %s: %w", id, err)
	}
	return nil
}

func (r *Registry) RemoveVnfInstanceByContainerID(containerID string) error {
	if err := r.dbHandle.Delete(&models.VnfInstance{}, "container_id = ?", containerID).Error; err != nil {
		return fmt.Errorf("failed to remove VNF instance with container ID %s: %w", containerID, err)
	}
	return nil
}

func (r *Registry) FindAllRoutes() ([]*models.Route, error) {
	var routes []*models.Route
	if err := r.dbHandle.Preload("Stages").Find(&routes).Error; err != nil {
		return nil, fmt.Errorf("failed to find all routes: %w", err)
	}
	return routes, nil
}

func (r *Registry) RemoveAllRoutes() error {
	if err := r.dbHandle.Delete(&models.Route{}, "1 = 1").Error; err != nil {
		return fmt.Errorf("failed to remove all routes: %w", err)
	}
	return nil
}

func (r *Registry) RemoveRemoteAppGroupsByID(ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.dbHandle.Delete(&models.RemoteAppGroup{}, "id IN ?", ids).Error; err != nil {
		return fmt.Errorf("failed to remove remote app groups by IDs %v: %w", ids, err)
	}
	return nil
}

func (r *Registry) SaveRoutes(routes []*models.Route) error {
	for _, route := range routes {
		if err := r.SaveRoute(route); err != nil {
			return fmt.Errorf("failed to save route: %w", err)
		}
	}
	return nil
}

func (r *Registry) SaveRouteStages(routeStages []*models.RouteStage) error {
	for _, stage := range routeStages {
		if err := r.dbHandle.Save(stage).Error; err != nil {
			return fmt.Errorf("failed to save route stage: %w", err)
		}
	}
	return nil
}

func (r *Registry) SaveRemoteAppGroup(group *models.RemoteAppGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save remote app group: %w", err)
	}
	return nil
}

func (r *Registry) SaveRemoteVnfGroup(group *models.RemoteVnfGroup) error {
	if err := r.dbHandle.Save(group).Error; err != nil {
		return fmt.Errorf("failed to save remote VNF group: %w", err)
	}
	return nil
}

func (r *Registry) SaveNode(node *models.Worker) error {
	if err := r.dbHandle.Save(node).Error; err != nil {
		return fmt.Errorf("failed to save node: %w", err)
	}
	return nil
}

func (r *Registry) MarkAppAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.Application{}).Where("global_id = ? AND status = ?", globalID, models.AppStatusActive).Update("status", models.AppStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark application %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *Registry) MarkVnfAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.VirtualNetworkFunction{}).Where("global_id = ? AND status = ?", globalID, models.VNFStatusActive).Update("status", models.VNFStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark VNF %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *Registry) MarkServiceChainAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.ServiceChain{}).Where("global_id = ? AND status = ?", globalID, models.ServiceChainStatusActive).Update("status", models.ServiceChainStatusDeletionPending).Error; err != nil {
		return fmt.Errorf("failed to mark service chain %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *Registry) MarkNodeAsDeleted(globalID string) error {
	if err := r.dbHandle.Model(&models.Worker{}).Where("global_id = ? AND status = ?", globalID, models.WorkerStatusActive).Update("status", models.WorkerStatusInactive).Error; err != nil {
		return fmt.Errorf("failed to mark node %s as deleted: %w", globalID, err)
	}
	return nil
}

func (r *Registry) RemoveRemoteAppInstancesByIP(ips []string, groupID uuid.UUID) error {
	if len(ips) == 0 {
		return nil
	}
	if err := r.dbHandle.Delete(&models.RemoteAppInstance{}, "group_id = ? AND ip IN ?", groupID, ips).Error; err != nil {
		return fmt.Errorf("failed to remove remote app instances by IPs %v: %w", ips, err)
	}
	return nil
}

func (r *Registry) RemoveRemoteAppGroupsByGlobalAppID(globalAppID string) error {
	if err := r.dbHandle.Delete(&models.RemoteAppGroup{}, "app_id = ?", globalAppID).Error; err != nil {
		return fmt.Errorf("failed to remove remote app groups for App ID %s: %w", globalAppID, err)
	}
	return nil
}

func (r *Registry) RemoveRemoteVnfGroupsByGlobalVnfID(globalVnfID string) error {
	if err := r.dbHandle.Delete(&models.RemoteVnfGroup{}, "vnf_id = ?", globalVnfID).Error; err != nil {
		return fmt.Errorf("failed to remove remote VNF groups for VNF ID %s: %w", globalVnfID, err)
	}
	return nil
}