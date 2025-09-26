// The tunneling package provides APIs for managing network tunnels
// between worker nodes.
//
// It includes functionalities for creating, updating, and deleting tunnels,
// as well as handling tunnel configurations and monitoring their status.
//
// VXLAN is used to create the tunnels in the underlying network.
//
// IMPORTANT: This package is still a work in progress and is not yet fully implemented.
package tunnel

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/services/events"
)

type TunnelService struct {
	registry *db.Registry
	eventBus *events.EventBus
}

func New(registry *db.Registry, eventBus *events.EventBus) (*TunnelService, error) {
	// Validate the registry and event bus
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if eventBus == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}

	tunnelService := &TunnelService{
		registry: registry,
		eventBus: eventBus,
	}
	
	// Subscribe to worker-related events
	eventBus.Subscribe("WorkerAdded", tunnelService.handleWorkerAdded)
	eventBus.Subscribe("WorkerRemoved", tunnelService.handleWorkerRemoved)

	return tunnelService, nil
}

func (t *TunnelService) handleWorkerAdded(evt events.Event) {
	panic("handleWorkerAdded not implemented")
}

func (t *TunnelService) handleWorkerRemoved(evt events.Event) {
	panic("handleWorkerRemoved not implemented")
}
