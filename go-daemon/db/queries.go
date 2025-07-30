package db

import (
	"fmt"
	"iml-daemon/models"
)

func (r *Registry) FindAppById(id string) (*models.Application, error) {
	var app models.Application
	if err := r.dbHandle.First(&app, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("application with id %s not found: %w", id, err)
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

func (r *Registry) FindAllNetworkFunctions() ([]*models.VirtualNetworkFunction, error) {
	var functions []*models.VirtualNetworkFunction
	if err := r.dbHandle.Find(&functions).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network functions: %w", err)
	}
	return functions, nil
}

func (r *Registry) FindNetworkFunctionByID(id string) (*models.VirtualNetworkFunction, error) {
	var function models.VirtualNetworkFunction
	if err := r.dbHandle.First(&function, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("network function with id %s not found: %w", id, err)
	}
	return &function, nil
}

func (r *Registry) FindVnfInstancesByVNFID(vnfID uint) ([]*models.VnfGroup, error) {
	var instances []*models.VnfGroup
	if err := r.dbHandle.Find(&instances, "vnf_id = ?", vnfID).Error; err != nil {
		return nil, fmt.Errorf("failed to find VNF instances for VNF ID %d: %w", vnfID, err)
	}
	return instances, nil
}

func (r *Registry) FindVnfInstanceByContainerID(containerID string) (*models.VnfGroup, error) {
	var instance models.VnfGroup
	if err := r.dbHandle.First(&instance, "container_id = ?", containerID).Error; err != nil {
		return nil, fmt.Errorf("VNF instance with container ID %s not found: %w", containerID, err)
	}
	return &instance, nil
}

func (r *Registry) FindAllNetworkServiceChains() ([]*models.NetworkServiceChain, error) {
	var chains []*models.NetworkServiceChain
	if err := r.dbHandle.Find(&chains).Error; err != nil {
		return nil, fmt.Errorf("failed to find all network service chains: %w", err)
	}
	return chains, nil
}

func (r *Registry) FindNetworkServiceChainByID(chainID string) (*models.NetworkServiceChain, error) {
	var chain models.NetworkServiceChain
	if err := r.dbHandle.First(&chain, "id = ?", chainID).Error; err != nil {
		return nil, fmt.Errorf("network service chain with id %s not found: %w", chainID, err)
	}
	return &chain, nil
}

func (r *Registry) SaveApp(app *models.Application) error {
	if err := r.dbHandle.Create(app).Error; err != nil {
		return fmt.Errorf("failed to save application: %w", err)
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

func (r *Registry) SaveVnfInstance(instance *models.VnfGroup) error {
	if err := r.dbHandle.Create(instance).Error; err != nil {
		return fmt.Errorf("failed to save VNF instance: %w", err)
	}
	return nil
}

func (r *Registry) SaveNetworkServiceChain(chain *models.NetworkServiceChain) error {
	if err := r.dbHandle.Create(chain).Error; err != nil {
		return fmt.Errorf("failed to save network service chain: %w", err)
	}
	return nil
}

func (r *Registry) SaveChainElement(element *models.ChainElement) error {
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
	if err := r.dbHandle.Delete(&models.VnfGroup{}, "container_id = ?", containerID).Error; err != nil {
		return fmt.Errorf("failed to remove VNF instance with container ID %s: %w", containerID, err)
	}
	return nil
}
