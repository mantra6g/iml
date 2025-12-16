package routecalc

import (
	"context"
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/services/events"
	"net"
	"runtime/debug"
	"sync"
)

// RouteCalcService listens for topology events and recalculates routes.
type RouteCalcService struct {
	eventBus events.EventBus
	registry db.Registry
	graph    *Graph
	mutex    sync.Mutex
}

// NewRouteCalcService constructs the service and subscribes to events.
func NewRouteCalcService(registry db.Registry, eb events.EventBus) (*RouteCalcService, error) {
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
		eventBus: eb,
		registry: registry,
		graph:    g,
	}

	// subscribe to instance lifecycle events
	eb.Subscribe(events.EventLocalAppGroupCreated, rc.handleEvent)
	eb.Subscribe(events.EventRemoteAppGroupCreated, rc.handleEvent)
	eb.Subscribe(events.EventLocalSimpleVnfGroupCreated, rc.handleEvent)
	eb.Subscribe(events.EventLocalVnfMultiplexedGroupCreated, rc.handleEvent)
	eb.Subscribe(events.EventRemoteVnfGroupCreated, rc.handleEvent)
	eb.Subscribe(events.EventChainCreated, rc.handleEvent)
	eb.Subscribe(events.EventChainUpdated, rc.handleEvent)
	eb.Subscribe(events.EventNodeCreated, rc.handleEvent)

	eb.Subscribe(events.EventLocalAppGroupRemoved, rc.handleEvent)
	eb.Subscribe(events.EventRemoteAppGroupRemoved, rc.handleEvent)
	eb.Subscribe(events.EventLocalSimpleVnfGroupRemoved, rc.handleEvent)
	eb.Subscribe(events.EventLocalVnfMultiplexedGroupRemoved, rc.handleEvent)
	eb.Subscribe(events.EventRemoteVnfGroupRemoved, rc.handleEvent)
	eb.Subscribe(events.EventChainRemoved, rc.handleEvent)
	eb.Subscribe(events.EventNodeRemoved, rc.handleEvent)

	return rc, nil
}

// handleEvent processes incoming events and triggers recalculation.
func (rc *RouteCalcService) handleEvent(evt events.Event) {
	logger.InfoLogger().Printf("RouteCalcService received event: %s", evt.Name)
	defer func() {
		if r := recover(); r != nil {
			logger.ErrorLogger().Printf("panic in handleEvent: %v", r)
			debug.PrintStack()
		}
	}()

	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	logger.InfoLogger().Printf("RouteCalcService got lock for event: %s", evt.Name)

	var err error
	switch evt.Name {
	case events.EventLocalSimpleVnfGroupCreated:
		inst, ok := evt.Payload.(models.SimpleVnfGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupCreated")
			break
		}
		err = rc.graph.AddSimpleVNFGroup(&inst)
	case events.EventLocalVnfMultiplexedGroupCreated:
		inst, ok := evt.Payload.(models.MultiplexedVnfGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupCreated")
			break
		}
		var group *models.MultiplexedVnfGroup
		group, err = rc.registry.FindSubfunctionPreloadedVnfMultiplexedGroupByID(inst.ID)
		if err != nil {
			err = fmt.Errorf("failed to retrieve multiplexed VNF group from registry: %v", err)
			break
		}
		err = rc.graph.AddMultiplexedVnfGroup(group)
		// rc.recalculateRoutesWithVnf(inst.ID)
	// case events.EventRemoteVnfGroupCreated:
	// 	inst, ok := evt.Payload.(models.RemoteVnfGroup)
	// 	if !ok {
	// 		err = fmt.Errorf("invalid payload type for EventRemoteVnfGroupCreated")
	// 		break
	// 	}
	// 	err = rc.graph.AddRemoteVNFGroup(&inst)
	// 	// rc.recalculateRoutesWithVnf(inst.ID)
	case events.EventLocalAppGroupCreated:
		inst, ok := evt.Payload.(models.AppGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventLocalAppGroupCreated")
			break
		}
		err = rc.graph.AddLocalAppGroup(&inst)
		// rc.recalculateRoutesWhereAppGroupIsSrc(inst.ID)
	case events.EventRemoteAppGroupCreated:
		inst, ok := evt.Payload.(models.RemoteAppGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventRemoteAppGroupCreated")
			break
		}
		err = rc.graph.AddRemoteAppGroup(&inst)
		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
	case events.EventChainCreated, events.EventChainUpdated:
		// Chains do not affect the graph structure directly
		// so we do not need to update the graph here.
		break
	case events.EventNodeCreated:
		inst, ok := evt.Payload.(models.Worker)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventNodeCreated")
			break
		}
		err = rc.graph.AddWorker(&inst)
	case events.EventLocalSimpleVnfGroupRemoved:
		inst, ok := evt.Payload.(models.SimpleVnfGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupRemoved")
			break
		}
		rc.graph.RemoveNode(inst.ID)
		// rc.recalculateRoutesWithVnf(inst.ID)
	case events.EventRemoteVnfGroupRemoved:
		inst, ok := evt.Payload.(models.RemoteVnfGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventRemoteVnfGroupRemoved")
			break
		}
		rc.graph.RemoveNode(inst.ID)
		// rc.recalculateRoutesWithVnf(inst.ID)
	case events.EventLocalAppGroupRemoved:
		inst, ok := evt.Payload.(models.AppGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventLocalAppGroupRemoved")
			break
		}
		rc.graph.RemoveNode(inst.ID)
		// When removing a local app group,
		// this means that there is no longer a source for routes
		// rc.invalidateRoutesWhereAppGroupIsSrc(inst.ID)
	case events.EventRemoteAppGroupRemoved:
		inst, ok := evt.Payload.(models.RemoteAppGroup)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventRemoteAppGroupRemoved")
			break
		}
		rc.graph.RemoveNode(inst.ID)
		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
	case events.EventChainRemoved:
		// Chains do not affect the graph structure directly
		// so we do not need to update the graph here.
		break
	case events.EventNodeRemoved:
		inst, ok := evt.Payload.(models.Worker)
		if !ok {
			err = fmt.Errorf("invalid payload type for EventNodeRemoved")
			break
		}
		rc.graph.RemoveNode(inst.ID)
		// When removing a worker, we need to recalculate
		// all routes that might have used this worker
		// This is a more complex operation, so we will just recalculate all routes
		// In the future, we might want to optimize this
		// by only recalculating routes that were directly affected.
		// rc.recalculateAll()
	}
	if err != nil {
		logger.ErrorLogger().Printf("failed to update graph for event %s: %v", evt.Name, err)
		return
	}

	logger.InfoLogger().Printf("RouteCalcService starting route recalculation")
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
	logger.DebugLogger().Printf("Got the following service chains: %+v", chains)

	// // Remove all old routes
	// if err := rc.registry.RemoveAllRoutes(); err != nil {
	// 	return fmt.Errorf("failed to remove old routes: %v", err)
	// }

	// Notify that routes are being recalculated
	rc.eventBus.Publish(events.Event{
		Name:    events.EventRouteRecalculationStarted,
		Payload: nil,
	})

	// Recompute routes for each chain
	logger.DebugLogger().Printf("Recomputing routes for %d chains", len(chains))
	for _, chain := range chains {
		routes, err := rc.computeRoutes(&chain)
		if err != nil {
			logger.ErrorLogger().Printf("failed to compute route for chain %d: %v", chain.ID, err)
			continue
		}
		// Notify that routes finished recalculation
		rc.eventBus.Publish(events.Event{
			Name:    events.EventRouteRecalculationFinished,
			Payload: routes,
		})
	}
	logger.DebugLogger().Printf("Route recomputation completed")

	return nil
}

// computeRoute finds shortest path for a single chain.
func (rc *RouteCalcService) computeRoutes(chain *models.ServiceChain) ([]*Route, error) {

	srcAppNode := rc.graph.FindLocalAppGroupNode(chain.SrcAppID)
	if srcAppNode == nil {
		return nil, fmt.Errorf("no source app group with id %s found for chain %s", chain.SrcAppID, chain.ID)
	}

	dstAppNodes := rc.graph.FindAllAppGroupNodes(chain.DstAppID)
	if len(dstAppNodes) == 0 {
		return nil, fmt.Errorf("no destination app groups with id %s found for chain %s", chain.DstAppID, chain.ID)
	}

	var nfs []FunctionSelector
	for _, elem := range chain.Elements {
		nfs = append(nfs, FunctionSelector{
			FunctionID:    elem.VnfID,
			SubfunctionID: elem.SubfunctionID,
		})
	}

	var routes []*Route
	for _, dstAppNode := range dstAppNodes {
		path, err := rc.graph.ShortestPath(dstAppNode.ID(), nfs)
		if err != nil {
			logger.InfoLogger().Printf("failed to compute shortest path for chain %s: %v", chain.ID, err)
			continue
		}

		// Save route
		route := &Route{
			ChainID:       chain.ID,
			SrcAppGroupID: srcAppNode.ID(),
			DstAppGroupID: dstAppNode.ID(),
		}

		// Set the stages for the route
		var vnfSids []net.IPNet
		vnfIndex := 0
		for _, node := range path {
			switch n := node.(type) {
			case VnfNode:
				vnfSids = append(vnfSids, *n.GetSIDThatSatisfies(nfs[vnfIndex]))
				vnfIndex++
			default:
				continue
			}
		}
		route.VnfSIDs = vnfSids

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
