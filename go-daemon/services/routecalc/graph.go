package routecalc

import (
	"container/heap"
	"fmt"
	"iml-daemon/models"

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
		srcNode: srcNodeID,
	}
	srcNode := WorkerNode{
		id:        srcNodeID,
	}
	graph.nodes[srcNodeID] = srcNode

	return graph, nil
}

func (g *Graph) AddVNFGroup(vnfGroup *models.VnfGroup) error {
	if vnfGroup == nil {
		return fmt.Errorf("vnfGroup is nil")
	}
	if vnfGroup.ID == uuid.Nil {
		return fmt.Errorf("vnfGroup ID is nil")
	}
	hID := g.srcNode
	if vnfGroup.WorkerID != nil {
		hID = *vnfGroup.WorkerID
	}
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

func (g *Graph) AddAppGroup(appGroup *models.AppGroup) error {
	if appGroup == nil {
		return fmt.Errorf("appGroup is nil")
	}
	if appGroup.ID == uuid.Nil {
		return fmt.Errorf("appGroup ID is nil")
	}
	hID := g.srcNode
	if appGroup.WorkerID != nil {
		hID = *appGroup.WorkerID
	}
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
	srcNode := WorkerNode{
		id:        worker.ID,
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
	for _, node := range g.nodes {
		appNode, ok := node.(AppNode)
		if !ok || appNode.appID != appID {continue}
		hubNode, exists := g.hub[appNode.id]
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
	// Set the source node
	srcID := g.srcNode

	// Initialize bests and prev maps
	bests := make(map[pathIndex]int)
	prev := make(map[uuid.UUID]uuid.UUID)
	for node := range g.nodes {
		for vnfIndex := range vnfs {
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
		if u.node == dstID && u.catIndex == len(vnfs) { break }

		// Check if we need to move to the next category
		if vnf, ok := g.nodes[u.node].(VnfNode); ok && u.catIndex < len(vnfs) && vnf.VnfID == vnfs[u.catIndex] {
			u.catIndex++ // Move to the next category
		}
		
		// If we have already found a better path to this node, skip it
		pIndx := pathIndex{node: u.node, vnfIndex: u.catIndex}
		if u.dist > bests[pIndx] { continue }

		bests[pIndx] = u.dist

		// Explore neighbors
		for _, edge := range g.adj[u.node] {
			alt := u.dist + edge.Cost
			if alt < bests[pathIndex{node: edge.To, vnfIndex: u.catIndex}] {
				bests[pathIndex{node: edge.To, vnfIndex: u.catIndex}] = alt
				prev[edge.To] = u.node
				heap.Push(pq, &item{node: edge.To, dist: alt, catIndex: u.catIndex})
			}
		}
	}
	
	if u.node != dstID {
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
