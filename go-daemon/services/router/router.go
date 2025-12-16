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
	"context"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/services/events"
	"iml-daemon/services/routecalc"
	"iml-daemon/services/router/dataplane"
	"net"
)

type RouterService struct {
	registry  db.Registry
	eventBus  events.EventBus
	dataplane dataplane.Manager
}

func New(registry db.Registry, eventBus events.EventBus, dataplane dataplane.Manager) (*RouterService, error) {
	// Validate the registry and event bus
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if eventBus == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}
	if dataplane == nil {
		return nil, fmt.Errorf("dataplane cannot be nil")
	}

	router := &RouterService{
		registry:  registry,
		eventBus:  eventBus,
		dataplane: dataplane,
	}
	// eventBus.Subscribe("RouteUpdated", router.handleRouteUpdated)
	eventBus.Subscribe(events.EventRouteRecalculationFinished, router.handlerRecalculationFinished)

	return router, nil
}

func (r *RouterService) handlerRecalculationFinished(evt events.Event) {
	routes, ok := evt.Payload.([]*routecalc.Route)
	if !ok {
		logger.ErrorLogger().Printf("handlerRecalculationFinished: error casting event payload to []*routecalc.Route")
		return
	}

	for _, route := range routes {
		err := r.Add(route)
		if err != nil {
			logger.ErrorLogger().Printf("handlerRecalculationFinished: error adding route %v: %v", route, err)
		}
	}
}

func (r *RouterService) Add(route *routecalc.Route) error {
	// Search for instances in the registry
	srcAppGroup, err := r.registry.FindAppGroupByID(route.SrcAppGroupID)
	if err != nil {
		return fmt.Errorf("error finding source app group: %v", err)
	}
	dstAppGroup, err := r.registry.FindAppGroupByID(route.DstAppGroupID)
	if err != nil {
		return fmt.Errorf("error finding destination app group: %v", err)
	}
	// TODO: Differentiate between local and remote Groups
	// For now, we assume all are local and reachable directly.
	// As such, we will use the DecapSID of the worker node hosting the destination app group.
	globalConfig, err := env.Config()
	if err != nil {
		return fmt.Errorf("failed to get global config: %w", err)
	}

	var sids []net.IP
	for _, sid := range route.VnfSIDs {
		sids = append([]net.IP{sid.IP}, sids...)
	}
	sids = append([]net.IP{globalConfig.DecapSID.IP}, sids...)

	err = r.dataplane.AddRoute(srcAppGroup.ID, dstAppGroup.GetSubnet(), sids)
	if err != nil {
		return fmt.Errorf("error adding route from group %s to group %s: %v", srcAppGroup.ID, dstAppGroup.ID, err)
	}
	return nil
}

func (r *RouterService) Shutdown(ctx context.Context) error {
	// Place any necessary cleanup logic here
	if r.dataplane == nil {
		return nil
	}
	// tear the dataplane down
	if err := r.dataplane.Close(); err != nil {
		logger.ErrorLogger().Printf("RouterService shutdown error: %v", err)
		return fmt.Errorf("failed to shutdown Dataplane: %w", err)
	}
	return nil
}
