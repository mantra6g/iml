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

func (r *Registry) FindAppByGlobalID(globalID string) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "global_id = ?", globalID).Error; err != nil {
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
	var group models.AppGroup
	if err := r.dbHandle.First(&group, "global_id = ? AND worker_id = null", globalAppID).Error; err != nil {
		return nil, fmt.Errorf("local app group with global ID %s not found: %w", globalAppID, err)
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

func (r *Registry) FindNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	var vnf models.VirtualNetworkFunction
	if err := r.dbHandle.First(&vnf, "global_id = ?", globalID).Error; err != nil {
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

func (r *Registry) FindLocalVnfGroupByVnfID(globalVnfID string) (*models.VnfGroup, error) {
	var group models.VnfGroup
	if err := r.dbHandle.First(&group, "global_id = ? AND worker_id = null", globalVnfID).Error; err != nil {
		return nil, fmt.Errorf("local VNF group with VNF ID %s not found: %w", globalVnfID, err)
	}
	return &group, nil
}

func (r *Registry) FindAllNetworkServiceChains() ([]*models.ServiceChain, error) {
	var chains []*models.ServiceChain
	if err := r.dbHandle.Find(&chains).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network service chains: %w", err)
	}
	return chains, nil
}

func (r *Registry) FindNetworkServiceChainByGlobalID(globalChainID string) (*models.ServiceChain, error) {
	var chain models.ServiceChain
	if err := r.dbHandle.First(&chain, "global_id = ?", globalChainID).Error; err != nil {
		return nil, fmt.Errorf("network service chain with global ID %s not found: %w", globalChainID, err)
	}
	return &chain, nil
}

func (r *Registry) SaveApp(app *models.Application) error {
	if err := r.dbHandle.Create(app).Error; err != nil {
		return fmt.Errorf("failed to save application: %w", err)
	}
	return nil
}

func (r *Registry) SaveAppGroup(group *models.AppGroup) error {
	if err := r.dbHandle.Create(group).Error; err != nil {
		return fmt.Errorf("failed to save app group: %w", err)
	}
	return nil
}

func (r *Registry) SaveAppInstance(instance *models.AppInstance) error {
	if err := r.dbHandle.Create(instance).Error; err != nil {
		return fmt.Errorf("failed to save app instance: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnf(function *models.VirtualNetworkFunction) error {
	if err := r.dbHandle.Create(function).Error; err != nil {
		return fmt.Errorf("failed to save virtual network function: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnfGroup(group *models.VnfGroup) error {
	if err := r.dbHandle.Create(group).Error; err != nil {
		return fmt.Errorf("failed to save VNF group: %w", err)
	}
	return nil
}

func (r *Registry) SaveVnfInstance(instance *models.VnfInstance) error {
	if err := r.dbHandle.Create(instance).Error; err != nil {
		return fmt.Errorf("failed to save VNF instance: %w", err)
	}
	return nil
}

func (r *Registry) SaveNetworkServiceChain(chain *models.ServiceChain) error {
	if err := r.dbHandle.Create(chain).Error; err != nil {
		return fmt.Errorf("failed to save network service chain: %w", err)
	}
	return nil
}

func (r *Registry) SaveChainElement(element *models.ServiceChainVnfs) error {
	if err := r.dbHandle.Create(element).Error; err != nil {
		return fmt.Errorf("failed to save chain element: %w", err)
	}
	return nil
}

func (r *Registry) SaveRoute(route *models.Route) error {
	if err := r.dbHandle.Create(route).Error; err != nil {
		return fmt.Errorf("failed to save route: %w", err)
	}
	return nil
}

func (r *Registry) RemoveAppInstanceByContainerID(containerID string) error {
	if err := r.dbHandle.Delete(&models.AppInstance{}, "container_id = ?", containerID).Error; err != nil {
		return fmt.Errorf("failed to remove app instance with container ID %s: %w", containerID, err)
	}
	return nil
}

func (r *Registry) RemoveVnfInstanceByContainerID(containerID string) error {
	if err := r.dbHandle.Delete(&models.VnfInstance{}, "container_id = ?", containerID).Error; err != nil {
		return fmt.Errorf("failed to remove VNF instance with container ID %s: %w", containerID, err)
	}
	return nil
}

func (r *Registry) RemoveAllRoutes() error {
	if err := r.dbHandle.Delete(&models.Route{}).Error; err != nil {
		return fmt.Errorf("failed to remove all routes: %w", err)
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
		if err := r.dbHandle.Create(stage).Error; err != nil {
			return fmt.Errorf("failed to save route stage: %w", err)
		}
	}
	return nil
}
