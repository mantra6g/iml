package registry

import (
	"fmt"
	"net"

	"iml-daemon/config"
	"iml-daemon/dataplane"
	"iml-daemon/helpers"
)

func (r *Registry) RegisterApp(request AppRegistrationRequest) (*AppDetails, error) {
	// Check if the application ID is valid
	if !r.IsValidAppID(request.ApplicationID) {
		return nil, fmt.Errorf("invalid application ID: %s", request.ApplicationID)
	}

	// Check if the instance is already registered
	// If it is, return its details
	if r.IsAppInstanceRegistered(request.ContainerID) {
		return nil, nil // Placeholder for actual logic to return existing app details
	}

	// Create the application details
	details := &AppDetails{
		ContainerID: request.ContainerID,
		AppIP: config.GenerateAppIP(request.ApplicationID),
	}

	// Register the container details in the APPLICATION registry
	r.appRegistry[request.ApplicationID] = &AppDetails{
		ContainerID: request.ContainerID,
		AppIP:       appIP,
	}

	// Create necessary routes in the nfrouter
	if err := dataplane.CreateRoutesForApp(request.ApplicationID, request.ContainerID, appIP); err != nil {
		return nil, fmt.Errorf("failed to create routes for app %s: %w", request.ApplicationID, err)
	}

	return &AppDetails{
		ContainerID: request.ContainerID,
		AppIP:       appIP,
	}, nil
}

func (r *Registry) TeardownApp(containerID string) error {
	// Find the application ID associated with the container
	appID, err := r.findAppByContainerID(containerID)
	if err != nil {
		return fmt.Errorf("container %s is not registered: %w", containerID, err)
	}

	// Remove the application from the registry
	delete(r.appRegistry, appID)

	// Remove routes from the nfrouter
	if err := dataplane.RemoveRoutesForApp(appID); err != nil {
		return fmt.Errorf("failed to remove routes for app %s: %w", appID, err)
	}

	return nil
}

func (r *Registry) IsValidAppID(appID string) bool {
	// Implement logic to check if the application ID is valid
	// This could involve checking against a list of known app IDs or a regex pattern
	return true // Placeholder for actual validation logic
}

func (r *Registry) IsAppInstanceRegistered(containerID string) bool {
	// Check if the container ID is already registered in the application registry
	for _, appDetails := range r.appRegistry {
		if appDetails.ContainerID == containerID {
			return true
		}
	}
	return false
}

func (r *Registry) findAppByContainerID(containerID string) (string, error) {
	// Iterate through the app registry to find the application ID by container ID
	for appID, appDetails := range r.appRegistry {
		if appDetails.ContainerID == containerID {
			return appID, nil
		}
	}
	return "", fmt.Errorf("no application found for container ID: %s", containerID)
}
