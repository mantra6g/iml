package routecalc

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/eventbus"
	"sync"
)

// RouteCalcService listens for topology events and recalculates routes.
type RouteCalcService struct {
	eb       *eventbus.EventBus
	registry *db.Registry
	graph    *Graph
	mu       sync.Mutex
}

// NewRouteCalcService constructs the service and subscribes to events.
func NewRouteCalcService(eb *eventbus.EventBus, registry *db.Registry) (*RouteCalcService, error) {
	// Validate the event bus and registry
	if eb == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	g, err := NewGraph()
	if err != nil {
		return nil, fmt.Errorf("failed to create new graph: %w", err)
	}
	rc := &RouteCalcService{
		eb:       eb,
		registry: registry,
		graph:    g,
	}

	// subscribe to instance lifecycle events
	eb.Subscribe("AppGroupCreated", rc.handleEvent)
	eb.Subscribe("AppGroupRemoved", rc.handleEvent)
	eb.Subscribe("VnfGroupCreated", rc.handleEvent)
	eb.Subscribe("VnfGroupRemoved", rc.handleEvent)

	return rc, nil
}

// handleEvent processes incoming events and triggers recalculation.
func (rc *RouteCalcService) handleEvent(evt eventbus.Event) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	switch evt.Name {
	case "VnfGroupCreated":
		inst := evt.Payload.(models.VnfGroup)
		rc.graph.AddVNFGroup(&inst)
	case "AppGroupCreated":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.AddAppGroup(&inst)
	case "WorkerNodeCreated":
		inst := evt.Payload.(models.Worker)
		rc.graph.AddWorker(&inst)
	case "VnfGroupRemoved":
		inst := evt.Payload.(models.VnfGroup)
		rc.graph.RemoveVNFGroup(&inst)
	case "AppGroupRemoved":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.RemoveAppGroup(&inst)
	case "WorkerNodeRemoved":
		inst := evt.Payload.(models.Worker)
		rc.graph.RemoveWorker(&inst)
	}

	// recalculate all routes after update
	if err := rc.recalculateAll(); err != nil {
		logger.ErrorLogger().Printf("failed to recalculate routes: %v", err)
	}
}

// recalculateAll retrieves all chains and recomputes their routes.
func (rc *RouteCalcService) recalculateAll() error {
	// Get all network service chains
	chains, err := rc.registry.FindAllNetworkServiceChains()
	if err != nil {
		return fmt.Errorf("failed to list chains: %v", err)
	}

	// Remove all old routes
	if err := rc.registry.RemoveAllRoutes(); err != nil {
		return fmt.Errorf("failed to remove old routes: %v", err)
	}

	// Notify that routes are being recalculated
	rc.eb.Publish(eventbus.Event{
		Name:    "RoutesRecalculating",
		Payload: nil,
	})

	// Recompute routes for each chain
	for _, chain := range chains {
		routes, err := rc.computeRoutes(chain)
		if err != nil {
			logger.InfoLogger().Printf("failed to compute route for chain %d: %v", chain.ID, err)
			continue
		}
		if err := rc.registry.SaveRoutes(routes); err != nil {
			return fmt.Errorf("failed to save routes for chain %d: %v", chain.ID, err)
		}
	}
	return nil
}

// computeRoute finds shortest path for a single chain.
func (rc *RouteCalcService) computeRoutes(chain *models.NetworkServiceChain) ([]*models.Route, error) {

	srcAppNode := rc.graph.FindLocalAppGroupNode(chain.SrcAppID)
	if srcAppNode == nil {
		return nil, fmt.Errorf("no source app group found for chain %s", chain.ID)
	}

	dstAppNodes := rc.graph.FindNodesByAppID(chain.DstAppID)
	if len(dstAppNodes) == 0 {
		return nil, fmt.Errorf("no destination app groups found for chain %s", chain.ID)
	}

	var vnfIDChain []string
	for _, elem := range chain.Elements {
		vnfIDChain = append(vnfIDChain, elem.VnfID)
	}

	var routes []*models.Route
	for _, dstAppNode := range dstAppNodes {

		path, err := rc.graph.ShortestPath(srcAppNode, dstAppNode, vnfIDChain)
		if err != nil {
			logger.InfoLogger().Printf("failed to compute shortest path for chain %s: %v", chain.ID, err)
		}

		// Build Route
		route := &models.Route{
			ChainID: chain.ID,
			SrcID:   srcAppNode.ID,
			DstID:   dstAppNode.ID,
		}

		// Save route
		if err := rc.registry.SaveRoute(route); err != nil {
			return nil, fmt.Errorf("failed to save route for chain %s: %v", chain.ID, err)
		}

		// Build segments from the full path
		var segments []*models.RouteSegment
		for i := 0; i < len(path)-1; i++ {
			seg := &models.RouteSegment{
				RouteID:  route.ID,
				VnfGroupID: path[i].ID,
				Position:  uint8(i),
			}
			segments = append(segments, seg)
		}

		// Save segments
		if err := rc.registry.SaveRouteSegments(segments); err != nil {
			return nil, fmt.Errorf("failed to save route segments for chain %s: %v", chain.ID, err)
		}

		// Add the route to the list
		routes = append(routes, route)
	}

	return routes, nil
}
