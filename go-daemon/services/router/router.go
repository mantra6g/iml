// The routing package provides APIs for managing SRv6 routes
// between applications and VNFs.
//
// It includes functionalities for creating, updating, and deleting routes,
// as well as handling route configurations and monitoring their status.
//
// The underlying routes are implemented by using Linux's SRv6 routing
// capabilities, allowing for efficient and flexible routing in the network.
package router

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/services/eventbus"
)

type RouterService struct {
	registry *db.Registry
	eventBus *eventbus.EventBus
}

func New(registry *db.Registry, eventBus *eventbus.EventBus) (*RouterService, error) {
	// Validate the registry and event bus
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if eventBus == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}

	router := &RouterService{
		registry: registry,
		eventBus: eventBus,
	}
	eventBus.Subscribe("RouteUpdated", router.handleRouteUpdated)
	eventBus.Subscribe("RouteDeleted", router.handleRouteDeleted)

	return router, nil
}

func (r *RouterService) handleRouteUpdated(evt eventbus.Event) {
	panic("handleRouteUpdated not implemented")
}

func (r *RouterService) handleRouteDeleted(evt eventbus.Event) {
	panic("handleRouteDeleted not implemented")
}