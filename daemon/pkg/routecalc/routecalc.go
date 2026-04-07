package routecalc

//
//import (
//	"context"
//	"fmt"
//	corev1alpha1 "iml-daemon/api/core/v1alpha1"
//	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
//	"iml-daemon/env"
//	"iml-daemon/logger"
//	"iml-daemon/models"
//	"iml-daemon/services/events"
//	"net"
//	"runtime/debug"
//	"sync"
//
//	"k8s.io/apimachinery/pkg/api/equality"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/labels"
//	"k8s.io/apimachinery/pkg/types"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//)
//
//// Route Calculation Service listens for topology events and recalculates routes.
//type Service interface {
//	Shutdown(ctx context.Context) error
//}
//
//type InMemoryService struct {
//	graph *Graph
//	mutex sync.Mutex
//
//	Config       *env.GlobalConfig
//	nodeData     map[types.NamespacedName]*NodeData
//	p4TargetData map[types.NamespacedName]*P4TargetData
//	chainData    map[types.NamespacedName]*ServiceChainData
//	appData      map[types.NamespacedName]*ApplicationData
//	nfData       map[types.NamespacedName]*NetworkFunctionData
//}
//
//// NewInMemoryService constructs the service and subscribes to events.
//func NewInMemoryService(cfg *env.GlobalConfig) (Service, error) {
//	g, err := NewGraph()
//	if err != nil {
//		return nil, fmt.Errorf("failed to create new graph: %w", err)
//	}
//	rc := &InMemoryService{
//		Config:       cfg,
//		nodeData:     make(map[types.NamespacedName]*NodeData),
//		p4TargetData: make(map[types.NamespacedName]*P4TargetData),
//		chainData:    make(map[types.NamespacedName]*ServiceChainData),
//		appData:      make(map[types.NamespacedName]*ApplicationData),
//		nfData:       make(map[types.NamespacedName]*NetworkFunctionData),
//		graph:        g,
//	}
//	return rc, nil
//}
//
//func (s *InMemoryService) UpsertServiceChain(chain *corev1alpha1.ServiceChain) error {
//	chainData, exists := s.chainData[client.ObjectKeyFromObject(chain)]
//	if !exists {
//		chainData = &ServiceChainData{}
//	}
//	if equality.Semantic.DeepEqual(chainData, chain) {
//		return nil
//	}
//	chainData.From = chain.Spec.From.ToNamespacedName()
//	chainData.To = chain.Spec.To.ToNamespacedName()
//	chainData.NFSelectors = chain.Spec.Functions
//	err := s.recalculateChains([]ServiceChainData{*chainData})
//	if err != nil {
//		return fmt.Errorf("failed to recalculate chains: %w", err)
//	}
//	s.chainData[client.ObjectKeyFromObject(chain)] = chainData
//	return nil
//}
//
//func (s *InMemoryService) RemoveServiceChain(chainKey types.NamespacedName) error {
//	chainData, exists := s.chainData[chainKey]
//	if !exists {
//		return nil
//	}
//	err := s.removeServiceChainRoutes(chainData)
//	if err != nil {
//		return fmt.Errorf("failed to remove service chain routes: %w", err)
//	}
//	delete(s.chainData, chainKey)
//	return nil
//}
//
//func (s *InMemoryService) UpsertApplication(app *corev1alpha1.Application) error {
//	appData, exists := s.appData[client.ObjectKeyFromObject(app)]
//	if !exists {
//		appData = &ApplicationData{}
//	}
//	if equality.Semantic.DeepEqual(appData, appData) {
//		return nil
//	}
//	chains, err := s.getServiceChainsOfApplication(client.ObjectKeyFromObject(app))
//	if err != nil {
//		return fmt.Errorf("failed to get service chains of application: %w", err)
//	}
//	err = s.recalculateChains(chains)
//	if err != nil {
//		return fmt.Errorf("failed to recalculate chains of application: %w", err)
//	}
//	return nil
//}
//
//func (s *InMemoryService) RemoveApplication(appKey types.NamespacedName) error {
//	_, exists := s.appData[appKey]
//	if !exists {
//		return nil
//	}
//	delete(s.appData, appKey)
//	return nil
//}
//
//func (s *InMemoryService) UpsertNetworkFunction(nf *corev1alpha1.NetworkFunction) error {
//	nfData, exists := s.nfData[client.ObjectKeyFromObject(nf)]
//	if !exists {
//		nfData = &NetworkFunctionData{}
//	}
//	if equality.Semantic.DeepEqual(nfData, nf) {
//		return nil
//	}
//
//}
//
//func (s *InMemoryService) RemoveNetworkFunction(nfKey types.NamespacedName) error {
//	_, exists := s.nfData[nfKey]
//	if !exists {
//		return nil
//	}
//	delete(s.nfData, nfKey)
//	return nil
//}
//
//func (s *InMemoryService) UpsertP4Target(target *corev1alpha1.P4Target) error {
//
//}
//
//func (s *InMemoryService) RemoveP4Target(targetKey types.NamespacedName) error {
//
//}
//
//func (s *InMemoryService) UpsertNode(node *infrav1alpha1.LoomNode) error {
//
//}
//
//func (s *InMemoryService) RemoveNode(nodeKey types.NamespacedName) error {
//
//}
//
//func (s *InMemoryService) calculateChainRoutes(chainKey types.NamespacedName) error {
//
//}
//
//func (s *InMemoryService) getServiceChainsOfApplication(appKey types.NamespacedName) ([]types.NamespacedName, error) {
//	_, exists := s.appData[appKey]
//	if !exists {
//		return nil, fmt.Errorf("application %s does not exist", appKey.String())
//	}
//	var matchingChains = make([]types.NamespacedName, 0)
//	for chainKey, chain := range s.chainData {
//		if chain.From == appKey || chain.To == appKey {
//			matchingChains = append(matchingChains, chainKey)
//		}
//	}
//	return matchingChains, nil
//}
//
//func (s *InMemoryService) getServiceChainsOfNetworkFunction(networkFunctionKey types.NamespacedName,
//) ([]types.NamespacedName, error) {
//	nf, exists := s.nfData[networkFunctionKey]
//	if !exists {
//		return nil, fmt.Errorf("network function %s does not exist", networkFunctionKey.String())
//	}
//	var matchingChains = make([]types.NamespacedName, 0)
//	for chainKey, chain := range s.chainData {
//		for _, funcSelector := range chain.NFSelectors {
//			selector, err := metav1.LabelSelectorAsSelector(&funcSelector)
//			if err != nil {
//				return nil, err
//			}
//			if selector.Matches(labels.Set(nf.Labels)) {
//				matchingChains = append(matchingChains, chainKey)
//			}
//		}
//	}
//	return matchingChains, nil
//}
//
//// handleEvent processes incoming events and triggers recalculation.
//func (s *InMemoryService) handleEvent(evt events.Event) {
//	logger.InfoLogger().Printf("RouteCalcService received event: %s", evt.Name)
//	defer func() {
//		if r := recover(); r != nil {
//			logger.ErrorLogger().Printf("panic in handleEvent: %v", r)
//			debug.PrintStack()
//		}
//	}()
//
//	s.mutex.Lock()
//	defer s.mutex.Unlock()
//	logger.InfoLogger().Printf("RouteCalcService got lock for event: %s", evt.Name)
//
//	var err error
//	switch evt.Name {
//	case events.EventLocalSimpleVnfGroupCreated:
//		inst, ok := evt.Payload.(models.SimpleVnfGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupCreated")
//			break
//		}
//		err = s.graph.AddSimpleVNFGroup(&inst)
//	case events.EventLocalVnfMultiplexedGroupCreated:
//		inst, ok := evt.Payload.(models.MultiplexedVnfGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupCreated")
//			break
//		}
//		var group *models.MultiplexedVnfGroup
//		group, err = s.registry.FindSubfunctionPreloadedVnfMultiplexedGroupByID(inst.ID)
//		if err != nil {
//			err = fmt.Errorf("failed to retrieve multiplexed VNF group from registry: %v", err)
//			break
//		}
//		err = s.graph.AddMultiplexedVnfGroup(group)
//		// rc.recalculateRoutesWithVnf(inst.ID)
//	// case events.EventRemoteVnfGroupCreated:
//	// 	inst, ok := evt.Payload.(models.RemoteVnfGroup)
//	// 	if !ok {
//	// 		err = fmt.Errorf("invalid payload type for EventRemoteVnfGroupCreated")
//	// 		break
//	// 	}
//	// 	err = rc.graph.AddRemoteVNFGroup(&inst)
//	// 	// rc.recalculateRoutesWithVnf(inst.ID)
//	case events.EventLocalAppGroupCreated:
//		inst, ok := evt.Payload.(models.AppGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventLocalAppGroupCreated")
//			break
//		}
//		err = s.graph.AddLocalAppGroup(&inst)
//		// rc.recalculateRoutesWhereAppGroupIsSrc(inst.ID)
//	case events.EventRemoteAppGroupCreated:
//		inst, ok := evt.Payload.(models.RemoteAppGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventRemoteAppGroupCreated")
//			break
//		}
//		err = s.graph.AddRemoteAppGroup(&inst)
//		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
//	case events.EventChainCreated, events.EventChainUpdated:
//		// Chains do not affect the graph structure directly
//		// so we do not need to update the graph here.
//		break
//	case events.EventNodeCreated:
//		inst, ok := evt.Payload.(models.Worker)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventNodeCreated")
//			break
//		}
//		err = s.graph.AddWorker(&inst)
//	case events.EventLocalSimpleVnfGroupRemoved:
//		inst, ok := evt.Payload.(models.SimpleVnfGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventLocalVnfGroupRemoved")
//			break
//		}
//		s.graph.RemoveNode(inst.ID)
//		// rc.recalculateRoutesWithVnf(inst.ID)
//	case events.EventRemoteVnfGroupRemoved:
//		inst, ok := evt.Payload.(models.RemoteVnfGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventRemoteVnfGroupRemoved")
//			break
//		}
//		s.graph.RemoveNode(inst.ID)
//		// rc.recalculateRoutesWithVnf(inst.ID)
//	case events.EventLocalAppGroupRemoved:
//		inst, ok := evt.Payload.(models.AppGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventLocalAppGroupRemoved")
//			break
//		}
//		s.graph.RemoveNode(inst.ID)
//		// When removing a local app group,
//		// this means that there is no longer a source for routes
//		// rc.invalidateRoutesWhereAppGroupIsSrc(inst.ID)
//	case events.EventRemoteAppGroupRemoved:
//		inst, ok := evt.Payload.(models.RemoteAppGroup)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventRemoteAppGroupRemoved")
//			break
//		}
//		s.graph.RemoveNode(inst.ID)
//		// rc.recalculateRoutesWhereAppGroupIsDst(inst.ID)
//	case events.EventChainRemoved:
//		// Chains do not affect the graph structure directly
//		// so we do not need to update the graph here.
//		break
//	case events.EventNodeRemoved:
//		inst, ok := evt.Payload.(models.Worker)
//		if !ok {
//			err = fmt.Errorf("invalid payload type for EventNodeRemoved")
//			break
//		}
//		s.graph.RemoveNode(inst.ID)
//		// When removing a worker, we need to recalculate
//		// all routes that might have used this worker
//		// This is a more complex operation, so we will just recalculate all routes
//		// In the future, we might want to optimize this
//		// by only recalculating routes that were directly affected.
//		// rc.recalculateAll()
//	}
//	if err != nil {
//		logger.ErrorLogger().Printf("failed to update graph for event %s: %v", evt.Name, err)
//		return
//	}
//
//	logger.InfoLogger().Printf("RouteCalcService starting route recalculation")
//	// recalculate all routes after update
//	if err := s.recalculateAll(); err != nil {
//		logger.ErrorLogger().Printf("failed to recalculate routes: %v", err)
//	}
//}
//
//// recalculateAll retrieves all chains and recomputes their routes.
//func (s *InMemoryService) recalculateAll() error {
//	// Get all network service chains
//	chains, err := s.registry.FindAllNetworkServiceChains()
//	if err != nil {
//		return fmt.Errorf("failed to list chains: %v", err)
//	}
//	logger.DebugLogger().Printf("Got the following service chains: %+v", chains)
//
//	// // Remove all old routes
//	// if err := rc.registry.RemoveAllRoutes(); err != nil {
//	// 	return fmt.Errorf("failed to remove old routes: %v", err)
//	// }
//
//	// Notify that routes are being recalculated
//	s.eventBus.Publish(events.Event{
//		Name:    events.EventRouteRecalculationStarted,
//		Payload: nil,
//	})
//
//	// Recompute routes for each chain
//	logger.DebugLogger().Printf("Recomputing routes for %d chains", len(chains))
//	for _, chain := range chains {
//		routes, err := s.computeRoutes(&chain)
//		if err != nil {
//			logger.ErrorLogger().Printf("failed to compute route for chain %d: %v", chain.ID, err)
//			continue
//		}
//		// Notify that routes finished recalculation
//		s.eventBus.Publish(events.Event{
//			Name:    events.EventRouteRecalculationFinished,
//			Payload: routes,
//		})
//	}
//	logger.DebugLogger().Printf("Route recomputation completed")
//
//	return nil
//}
//
//// computeRoute finds shortest path for a single chain.
//func (s *InMemoryService) computeRoutes(chain *models.ServiceChain) ([]*Route, error) {
//
//	srcAppNode := s.graph.FindLocalAppGroupNode(chain.SrcAppID)
//	if srcAppNode == nil {
//		return nil, fmt.Errorf("no source app group with id %s found for chain %s", chain.SrcAppID, chain.ID)
//	}
//
//	dstAppNodes := s.graph.FindAllAppGroupNodes(chain.DstAppID)
//	if len(dstAppNodes) == 0 {
//		return nil, fmt.Errorf("no destination app groups with id %s found for chain %s", chain.DstAppID, chain.ID)
//	}
//
//	var nfs []FunctionSelector
//	for _, elem := range chain.Elements {
//		nfs = append(nfs, FunctionSelector{
//			FunctionID:    elem.VnfID,
//			SubfunctionID: elem.SubfunctionID,
//		})
//	}
//
//	var routes []*Route
//	for _, dstAppNode := range dstAppNodes {
//		path, err := s.graph.ShortestPath(dstAppNode.ID(), nfs)
//		if err != nil {
//			logger.InfoLogger().Printf("failed to compute shortest path for chain %s: %v", chain.ID, err)
//			continue
//		}
//
//		// Save route
//		route := &Route{
//			ChainID:       chain.ID,
//			SrcAppGroupID: srcAppNode.ID(),
//			DstAppGroupID: dstAppNode.ID(),
//		}
//
//		// Set the stages for the route
//		var vnfSids []net.IPNet
//		vnfIndex := 0
//		for _, node := range path {
//			switch n := node.(type) {
//			case VnfNode:
//				vnfSids = append(vnfSids, *n.GetSIDThatSatisfies(nfs[vnfIndex]))
//				vnfIndex++
//			default:
//				continue
//			}
//		}
//		route.VnfSIDs = vnfSids
//
//		// Add the route to the list
//		routes = append(routes, route)
//	}
//
//	return routes, nil
//}
//
//func (s *InMemoryService) Shutdown(ctx context.Context) error {
//	// Place any necessary cleanup logic here
//	// Maybe cancel any ongoing calculations or close resources
//	// For now, this is a no-op
//	return nil
//}
