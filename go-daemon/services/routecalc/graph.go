package routecalc

import (
	"container/heap"
	"fmt"
	"iml-daemon/env"
	"iml-daemon/logger"
	"iml-daemon/models"
	"net"

	"github.com/google/uuid"
)

// Graph represents the network as adjacency lists for Dijkstra.
type Graph struct {
	nodes   map[uuid.UUID]GraphNode
	adj     map[uuid.UUID][]GraphEdge
	hub     map[uuid.UUID]WorkerNode
	srcNode uuid.UUID
}

func NewGraph() (*Graph, error) {
	srcNodeID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to create source hub ID: %w", err)
	}
	graph := &Graph{
		nodes: make(map[uuid.UUID]GraphNode),
		adj:   make(map[uuid.UUID][]GraphEdge),
		hub:   make(map[uuid.UUID]WorkerNode),
		srcNode: srcNodeID,
	}
	globalConfig, err := env.Config()
	if err != nil {
		return nil, fmt.Errorf("failed to get global config: %w", err)
	}
	srcNode := WorkerNode{
		id:       srcNodeID,
		DecapSID: globalConfig.DecapSID.IP,
	}
	graph.nodes[srcNodeID] = srcNode

	return graph, nil
}

func (g *Graph) AddLocalVNFGroup(vnfGroup *models.VnfGroup) error {
	if vnfGroup == nil {
		return fmt.Errorf("vnfGroup is nil")
	}
	if vnfGroup.ID == uuid.Nil {
		return fmt.Errorf("vnfGroup ID is nil")
	}
	hID := g.srcNode
	hNode, ok := g.nodes[hID].(WorkerNode)
	if !ok {
		return fmt.Errorf("worker node %s does not exist", hID)
	}
	g.nodes[vnfGroup.ID] = VnfNode{
		id:    vnfGroup.ID,
		VnfID: vnfGroup.VnfID,
	}
	g.hub[vnfGroup.ID] = hNode
	g.addEdge(vnfGroup.ID, hID, 1)

	return nil
}

func (g *Graph) AddRemoteVNFGroup(vnfGroup *models.RemoteVnfGroup) error {
	if vnfGroup == nil {
		return fmt.Errorf("vnfGroup is nil")
	}
	if vnfGroup.ID == uuid.Nil {
		return fmt.Errorf("vnfGroup ID is nil")
	}
	hID := vnfGroup.WorkerID
	hNode, ok := g.nodes[hID].(WorkerNode)
	if !ok {
		return fmt.Errorf("worker node %s does not exist", hID)
	}
	g.nodes[vnfGroup.ID] = VnfNode{
		id:    vnfGroup.ID,
		VnfID: vnfGroup.VnfID,
	}
	g.hub[vnfGroup.ID] = hNode
	g.addEdge(vnfGroup.ID, hID, 1)

	return nil
}

func (g *Graph) AddLocalAppGroup(appGroup *models.AppGroup) error {
	if appGroup == nil {
		return fmt.Errorf("appGroup is nil")
	}
	if appGroup.ID == uuid.Nil {
		return fmt.Errorf("appGroup ID is nil")
	}
	hID := g.srcNode
	hNode, ok := g.nodes[hID].(WorkerNode)
	if !ok {
		return fmt.Errorf("worker node %s does not exist", hID)
	}
	g.nodes[appGroup.ID] = AppNode{
		id:    appGroup.ID,
		appID: appGroup.AppID,
	}
	g.hub[appGroup.ID] = hNode
	g.addEdge(appGroup.ID, hID, 1) // Cost is always 1
	return nil
}

func (g *Graph) AddRemoteAppGroup(appGroup *models.RemoteAppGroup) error {
	if appGroup == nil {
		return fmt.Errorf("appGroup is nil")
	}
	if appGroup.ID == uuid.Nil {
		return fmt.Errorf("appGroup ID is nil")
	}
	hID := appGroup.NodeID
	hNode, ok := g.nodes[hID].(WorkerNode)
	if !ok {
		return fmt.Errorf("worker node %s does not exist", hID)
	}
	g.nodes[appGroup.ID] = AppNode{
		id:    appGroup.ID,
		appID: appGroup.AppID,
	}
	g.hub[appGroup.ID] = hNode
	g.addEdge(appGroup.ID, hID, 1) // Cost is always 1
	return nil
}

func (g *Graph) AddWorker(worker *models.Worker) error {
	if worker == nil {
		return fmt.Errorf("worker is nil")
	}
	if worker.ID == uuid.Nil {
		return fmt.Errorf("worker ID is nil")
	}
	// TODO: This receives a string in the ip/net format, but this parses the ip only.
	// We need to adjust this if we want to use the subnet mask later.
	sid := net.ParseIP(worker.DecapSID)
	if sid == nil {
		return fmt.Errorf("invalid DecapSID IP address: %s", worker.DecapSID)
	}
	srcNode := WorkerNode{
		id:        worker.ID,
		DecapSID:  sid,
	}
	g.nodes[worker.ID] = srcNode
	g.addEdge(g.srcNode, worker.ID, 100) // From worker to source node
	return nil
}

func (g *Graph) RemoveNode(id uuid.UUID) {
	if _, exists := g.nodes[id]; !exists { return }

	// Remove the adjacencies that point to this node
	for _, edge := range g.adj[id] {
		g.adj[edge.To] = removeValue(g.adj[edge.To], id) // Remove the edge from the other node
	}

	// Remove the node from the nodes and adjacencies
	delete(g.nodes, id)
	delete(g.adj, id)
}

func (g *Graph) RemoveWorkerNode(id uuid.UUID) {
	if _, exists := g.nodes[id]; !exists { return }
	// Remove all nodes in the hub that point to this worker
	for _, node := range g.adj[id] {
		if hubNode, exists := g.hub[node.To]; exists && hubNode.id == id {
			g.RemoveNode(node.To)
		}
	}
}

func removeValue(slice []GraphEdge, id uuid.UUID) []GraphEdge {
	result := make([]GraphEdge, 0, len(slice))
	for _, v := range slice {
		if v.To != id {
			result = append(result, v)
		}
	}
	return result
}

func (g *Graph) addEdge(nodeA, nodeB uuid.UUID, cost int) {
	// As links between containers are non-directional, add both directions
	g.adj[nodeA] = append(g.adj[nodeA], GraphEdge{To: nodeB, Cost: cost})
	g.adj[nodeB] = append(g.adj[nodeB], GraphEdge{To: nodeA, Cost: cost})
}

func (g *Graph) FindLocalAppGroupNode(appID uuid.UUID) *AppNode {
	logger.DebugLogger().Printf("Finding local AppGroupNode for AppID: %s", appID)
	logger.DebugLogger().Printf("Graph state: %+v", g)
	for _, node := range g.nodes {
		logger.DebugLogger().Printf("Checking node: %+v", node)
		appNode, ok := node.(AppNode)
		if !ok || appNode.appID != appID {continue}
		hubNode, exists := g.hub[appNode.id]
		logger.DebugLogger().Printf("Hub node for AppNode %s: %+v, exists: %v", appNode.id, hubNode, exists)
		if exists && hubNode.id == g.srcNode {
			return &appNode
		}
	}
	return nil
}

func (g *Graph) FindAllAppGroupNodes(appID uuid.UUID) []*AppNode {
	var nodes []*AppNode
	for _, node := range g.nodes {
		appNode, ok := node.(AppNode)
		if !ok || appNode.appID != appID {continue}
		if _, exists := g.hub[appNode.id]; exists {
			nodes = append(nodes, &appNode)
		}
	}
	return nodes
}

// func (g *Graph) FindNodesByAppID(appID uuid.UUID) []uuid.UUID {
// 	var nodes []uuid.UUID
// 	for id, node := range g.nodes {
// 		if node.Category == NODE_CAT_APP && node.CategoryID == appID {
// 			nodes = append(nodes, id)
// 		}
// 	}
// 	return nodes
// }

// func (g *Graph) FindNodesByVnfID(vnfID uuid.UUID) []uuid.UUID {
// 	var nodes []uuid.UUID
// 	for id, node := range g.nodes {
// 		if node.Category == NODE_CAT_VNF && node.CategoryID == vnfID {
// 			nodes = append(nodes, id)
// 		}
// 	}
// 	return nodes
// }

// Cost returns edge cost.
// func (g *Graph) Cost(from, to uuid.UUID) int {
// 	for _, e := range g.adj[from] {
// 		if e.To == to {
// 			return e.Cost
// 		}
// 	}
// 	return 0
// }

type pathIndex struct {
	node uuid.UUID
	vnfIndex int // Index of the category in the categoryIDs slice
}

// Dijkstra's implementation using heap
func (g *Graph) ShortestPath(dstID uuid.UUID, vnfs []uuid.UUID) ([]GraphNode, error) {
	logger.DebugLogger().Printf("Starting ShortestPath to %s with VNFs: %+v", dstID, vnfs)
	logger.DebugLogger().Printf("Graph state: %+v", g)
	// Set the source node
	srcID := g.srcNode

	// Initialize bests and prev maps
	bests := make(map[pathIndex]int)
	prev := make(map[uuid.UUID]uuid.UUID)
	for node := range g.nodes {
		for vnfIndex := 0; vnfIndex < len(vnfs)+1; vnfIndex++ {
			bests[pathIndex{node: node, vnfIndex: vnfIndex}] = int(^uint(0) >> 1) // max int
		}
	}
	bests[pathIndex{node: srcID, vnfIndex: 0}] = 0

	// Initialize the priority queue
	pq := &priorityQueue{}
	heap.Init(pq)
	heap.Push(pq, &item{node: srcID, dist: 0, catIndex: 0})

	var u *item
	for pq.Len() > 0 {
		u = heap.Pop(pq).(*item)

		// Stop if we reached the destination node and have traversed all categories
		if u.node == dstID && u.catIndex == len(vnfs) {
			logger.DebugLogger().Printf("Reached destination %s with all VNFs traversed.", dstID)
			break 
		}

		// Check if we need to move to the next category
		if vnf, ok := g.nodes[u.node].(VnfNode); ok && u.catIndex < len(vnfs) && vnf.VnfID == vnfs[u.catIndex] {
			u.catIndex++ // Move to the next category
		}
		
		// If we have already found a better path to this node, skip it
		pIndx := pathIndex{node: u.node, vnfIndex: u.catIndex}
		if u.dist > bests[pIndx] {
			logger.DebugLogger().Printf("Skipping node %s with category index %d as a better path already exists.", u.node, u.catIndex)
			continue
		}

		bests[pIndx] = u.dist

		// Explore neighbors
		logger.DebugLogger().Printf("Exploring neighbors of node %s with category index %d.", u.node, u.catIndex)
		for _, edge := range g.adj[u.node] {
			logger.DebugLogger().Printf("Checking edge from %s to %s", u.node, edge.To)
			alt := u.dist + edge.Cost
			if alt < bests[pathIndex{node: edge.To, vnfIndex: u.catIndex}] {
				logger.DebugLogger().Printf("Found better path to %s with category index %d: %d", edge.To, u.catIndex, alt)
				bests[pathIndex{node: edge.To, vnfIndex: u.catIndex}] = alt
				prev[edge.To] = u.node
				heap.Push(pq, &item{node: edge.To, dist: alt, catIndex: u.catIndex})
			}
		}
	}
	
	if u.node != dstID {
		logger.DebugLogger().Printf("No path found to %s after exploring all nodes.", dstID)
		logger.DebugLogger().Printf("Best distances: %+v", bests)
		logger.DebugLogger().Printf("Previous nodes: %+v", prev)
		logger.DebugLogger().Printf("Final node reached: %s with category index %d", u.node, u.catIndex)
		return nil, fmt.Errorf("no path found to %s", dstID)
	}

	// reconstruct path
	var path []GraphNode
	for u, ok := prev[dstID]; ok; u, ok = prev[u] {
		if node, exists := g.nodes[u]; exists {
			path = append([]GraphNode{node}, path...)
		}
	}
	return path, nil
}
