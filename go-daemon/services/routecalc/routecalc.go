package routecalc

import (
	"context"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/eventbus"
	"sync"

	"github.com/google/uuid"
)

// RouteCalcService listens for topology events and recalculates routes.
type RouteCalcService struct {
	eventBus *eventbus.EventBus
	registry *db.Registry
	graph    *Graph
	mutex    sync.Mutex
}

// NewRouteCalcService constructs the service and subscribes to events.
func NewRouteCalcService(registry *db.Registry, eb *eventbus.EventBus) (*RouteCalcService, error) {
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
		eventBus:       eb,
		registry: registry,
		graph:    g,
	}

	// subscribe to instance lifecycle events
	eb.Subscribe("AppGroupCreated", rc.handleEvent)
	eb.Subscribe("VnfGroupCreated", rc.handleEvent)
	eb.Subscribe("WorkerNodeCreated", rc.handleEvent)
	eb.Subscribe("AppGroupRemoved", rc.handleEvent)
	eb.Subscribe("VnfGroupRemoved", rc.handleEvent)
	eb.Subscribe("WorkerNodeRemoved", rc.handleEvent)

	return rc, nil
}

// handleEvent processes incoming events and triggers recalculation.
func (rc *RouteCalcService) handleEvent(evt eventbus.Event) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	switch evt.Name {
	case "VnfGroupCreated":
		inst := evt.Payload.(models.VnfGroup)
		rc.graph.AddVNFGroup(&inst)
		// rc.recalculateRoutesWithVnf(inst.ID)
	case "LocalAppGroupCreated":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.AddAppGroup(&inst)
		// rc.recalculateRoutesWhereAppGroupIsSrc(inst.ID)
	case "RemoteAppGroupCreated":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.AddAppGroup(&inst)
		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
	case "WorkerNodeCreated":
		inst := evt.Payload.(models.Worker)
		rc.graph.AddWorker(&inst)
	case "VnfGroupRemoved":
		inst := evt.Payload.(models.VnfGroup)
		rc.graph.RemoveNode(inst.ID)
		// rc.recalculateRoutesWithVnf(inst.ID)
	case "LocalAppGroupRemoved":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.RemoveNode(inst.ID)
		// When removing a local app group, 
		// this means that there is no longer a source for routes
		// rc.invalidateRoutesWhereAppGroupIsSrc(inst.ID)
	case "RemoteAppGroupRemoved":
		inst := evt.Payload.(models.AppGroup)
		rc.graph.RemoveNode(inst.ID)
		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
	case "WorkerNodeRemoved":
		inst := evt.Payload.(models.Worker)
		rc.graph.RemoveNode(inst.ID)
		// When removing a worker, we need to recalculate 
		// all routes that might have used this worker
		// This is a more complex operation, so we will just recalculate all routes
		// In the future, we might want to optimize this
		// by only recalculating routes that were directly affected.
		// rc.recalculateAll()
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
	rc.eventBus.Publish(eventbus.Event{
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
func (rc *RouteCalcService) computeRoutes(chain *models.ServiceChain) ([]*models.Route, error) {

	srcAppNode := rc.graph.FindLocalAppGroupNode(chain.SrcAppID)
	if srcAppNode == nil {
		return nil, fmt.Errorf("no source app group found for chain %s", chain.ID)
	}

	dstAppNodes := rc.graph.FindAllAppGroupNodes(chain.DstAppID)
	if len(dstAppNodes) == 0 {
		return nil, fmt.Errorf("no destination app groups found for chain %s", chain.ID)
	}

	var vnfIDChain []uuid.UUID
	for _, elem := range chain.Elements {
		vnfIDChain = append(vnfIDChain, elem.VnfID)
	}

	var routes []*models.Route
	for _, dstAppNode := range dstAppNodes {

		path, err := rc.graph.ShortestPath(dstAppNode.ID(), vnfIDChain)
		if err != nil {
			logger.InfoLogger().Printf("failed to compute shortest path for chain %s: %v", chain.ID, err)
			continue
		}

		// Save route
		route := &models.Route{
			ChainID:       chain.ID,
			SrcAppGroupID: srcAppNode.ID(),
			DstAppGroupID: dstAppNode.ID(),
		}
		rc.registry.SaveRoute(route)

		// Set the stages for the route
		var stages []*models.RouteStage
		pos := uint8(0)
		for _, node := range path {
			if node.Type() != "VNF" {continue}
			stages = append(stages, &models.RouteStage{
				RouteID: 	route.ID,
				VnfGroupID: node.ID(),
				Position:   pos,
			})
			pos++
		}
		rc.registry.SaveRouteStages(stages)

		// Add the route to the list
		routes = append(routes, route)
	}

	return routes, nil
}

func (rc *RouteCalcService) Shutdown(ctx context.Context) error {
	// Place any necessary cleanup logic here
	// Maybe cancel any ongoing calculations or close resources
	// For now, this is a no-op
	return nil
}
