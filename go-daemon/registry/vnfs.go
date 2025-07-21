package registry

import (
	"fmt"
	"iml-daemon/dataplane"
	"net"
)

func (r *Registry) RegisterVnf(vnfID string, containerID string, vnfIP *net.IPNet) error {
	// Check if the VNF ID is valid
	if !r.IsValidVnfID(vnfID) {
		return fmt.Errorf("invalid VNF ID: %s", vnfID)
	}

	// Check if the instance is already registered
	// If it is, return its details
	if r.IsVnfInstanceRegistered(containerID) {
		return nil
	}

	// Register the container details in the VNF registry
	r.nfvRegistry[vnfID] = &VnfDetails{
		ContainerID: containerID,
		VnfIP:       vnfIP,
	}

	// Create necessary routes in the nfrouter
	if err := dataplane.CreateRoutesForVnf(vnfID, containerID, vnfIP); err != nil {
		return fmt.Errorf("failed to create routes for VNF %s: %w", vnfID, err)
	}

	return nil
}

func (r *Registry) TeardownVnf(containerID string) error {
	// Find the VNF ID associated with the container
	vnfID, err := r.findVnfByContainerID(containerID)
	if err != nil {
		return fmt.Errorf("container %s is not registered: %w", containerID, err)
	}

	// Remove the VNF from the registry
	delete(r.nfvRegistry, vnfID)

	// Remove routes from the nfrouter
	if err := dataplane.RemoveRoutesForVnf(vnfID); err != nil {
		return fmt.Errorf("failed to remove routes for VNF %s: %w", vnfID, err)
	}

	return nil
}

func (r *Registry) IsValidVnfID(vnfID string) bool {
	// Implement logic to check if the VNF ID is valid
	// This could involve checking against a list of known VNF IDs or a regex pattern
	return true // Placeholder for actual validation logic
}

func (r *Registry) IsVnfInstanceRegistered(containerID string) bool {
	// Check if the container ID is already registered in the VNF registry
	for _, vnfDetails := range r.nfvRegistry {
		if vnfDetails.ContainerID == containerID {
			return true
		}
	}
	return false
}

func (r *Registry) findVnfByContainerID(containerID string) (string, error) {
	// Find the VNF ID associated with the given container ID
	for vnfID, vnfDetails := range r.nfvRegistry {
		if vnfDetails.ContainerID == containerID {
			return vnfID, nil
		}
	}
	return "", nil
}