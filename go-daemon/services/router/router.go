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
	"iml-daemon/models"
	"iml-daemon/services/events"
	"net"
)

type RouterService struct {
	registry *db.Registry
	eventBus *events.EventBus
	nfrouter *NFRouter
}

func New(registry *db.Registry, eventBus *events.EventBus) (*RouterService, error) {
	// Validate the registry and event bus
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if eventBus == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}

	config, err := env.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to get global config: %w", err)
	}

	nfrInstance, err := newNFRouter(config.NFRouterAppIP, config.NFRouterVNFIP)
	if err != nil {
		logger.ErrorLogger().Printf("Failed to configure NFRouter: %v", err)
		return nil, fmt.Errorf("failed to configure NFRouter: %w", err)
	}

	router := &RouterService{
		registry: registry,
		eventBus: eventBus,
		nfrouter: nfrInstance,
	}
	// eventBus.Subscribe("RouteUpdated", router.handleRouteUpdated)
	eventBus.Subscribe(events.EventRouteRecalculationFinished, router.handlerRecalculationFinished)

	return router, nil
}

func (r *RouterService) handlerRecalculationFinished(evt events.Event) {
	// After route recalculation, update all routes in the NFRouter
	routes, err := r.registry.FindAllRoutes()
	if err != nil {
		logger.ErrorLogger().Printf("handlerRecalculationFinished: error retrieving routes: %v", err)
		return
	}

	for _, route := range routes {
		err := r.Add(route)
		if err != nil {
			logger.ErrorLogger().Printf("handlerRecalculationFinished: error adding route %v: %v", route, err)
		}
	}
}

func (r *RouterService) Add(route *models.Route) error {
	// Search for instances in the registry
	srcAppInstances, err := r.registry.FindAppInstancesByGroupID(route.SrcAppGroupID)
	if err != nil {
		return fmt.Errorf("error finding source app instances: %v", err)
	}
	dstAppInstances, err := r.registry.FindAppInstancesByGroupID(route.DstAppGroupID)
	if err != nil {
		return fmt.Errorf("error finding destination app instances: %v", err)
	}
	// TODO: Differentiate between local and remote Groups
	// For now, we assume all are local and reachable directly.
	// As such, we will use the DecapSID of the worker node hosting the destination app group.
	globalConfig, err := env.Config()
	if err != nil {
		return fmt.Errorf("failed to get global config: %w", err)
	}

	var sids []net.IP
	for _, vnfGroup := range route.Stages {
		sid := net.ParseIP(vnfGroup.SID)
		if sid == nil {
			return fmt.Errorf("error parsing SID %s: %v", vnfGroup.SID, err)
		}
		sids = append(sids, sid)
	}
	sids = append(sids, globalConfig.NFRouterVNFIP.IP)

	for _, srcAppInstance := range srcAppInstances {
		for _, dstAppInstance := range dstAppInstances {
			err = r.nfrouter.AddRoute(srcAppInstance.IP, dstAppInstance.IP, sids)
			if err != nil {
				logger.ErrorLogger().Printf("handleRouteUpdated: error adding route from %s to %s: %v", srcAppInstance.IP, dstAppInstance.IP, err)
			}
		}
	}
	return nil
}

func (r *RouterService) handleRouteUpdated(evt events.Event) {
	route, ok := evt.Payload.(models.Route)
	if !ok {
		logger.ErrorLogger().Printf("handleRouteUpdated: error casting event payload to Route")
		return
	}

	if err := r.Add(&route); err != nil {
		logger.ErrorLogger().Printf("handleRouteUpdated: error adding route: %v", err)
	}
}

func (r *RouterService) Shutdown(ctx context.Context) error {
	// Place any necessary cleanup logic here
	if r.nfrouter == nil {
		return nil
	}
	// tear the nfrouter interface down
	if err := r.nfrouter.Teardown(); err != nil {
		logger.ErrorLogger().Printf("RouterService shutdown error: %v", err)
		return fmt.Errorf("failed to shutdown NFRouter: %w", err)
	}
	return nil
}
