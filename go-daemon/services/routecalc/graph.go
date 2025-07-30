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
	// apps    map[uuid.UUID][]GraphNode
	// vnfs    map[uuid.UUID][]GraphNode
	// Source node, typically the local hub
	// This is used to calculate the shortest path from the local hub to other nodes.
	srcNode uuid.UUID
}

// GraphNode represents a node in the graph with its location and group.
// type GraphNode struct {
// 	CategoryID uuid.UUID    // e.g. VNF ID or Application ID
// 	Category   NodeCategory // e.g. VNF, App, Hub
// }

type GraphNode interface {
	sID() *string // Returns the Segment ID (SID) of the node, if applicable
}

type WorkerNode struct {
	ID        uuid.UUID // Unique identifier for the worker node
	DecapNode uuid.UUID // Decapsulation node ID for the worker
}

func (w WorkerNode) sID() *string {
	// Worker nodes do not have a Segment ID (SID) in this context.
	return nil
}

type DecapsulationNode struct {
	ID  uuid.UUID // Unique identifier for the decapsulation node
	SID string
}

func (d DecapsulationNode) sID() *string {
	return &d.SID
}

type AppNode struct {
	ID uuid.UUID // Unique identifier for the application node
}
func (a AppNode) sID() *string { return nil }

type VnfNode struct {
	ID     uuid.UUID // Unique identifier for the VNF node
	VnfID  uuid.UUID // VNF ID
	SID    string    // Segment ID (SID) for the VNF
}
func (v VnfNode) sID() *string {
	return &v.SID
}

type GraphEdge struct {
	To   uuid.UUID
	Cost int
}

type NodeCategory uint8
const (
	// Group of Virtual Network Function instances.
	// They all share the same VNF ID and Segment ID (SID).
	NODE_CAT_VNF NodeCategory = iota

	// Group of Application instances.
	// They all share the same Application ID.
	// Each of the instances has its own IP address.
	NODE_CAT_APP

	// A remote worker node.
	NODE_CAT_HUB

	// This node. It is the source of all calculated routes.
	NODE_CAT_SOURCE

	// Decapsulation node.
	NODE_CAT_DECAPSULATION
)

func NewGraph() (*Graph, error) {
	// Create source hub ID
	srcNodeID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to create source hub ID: %w", err)
	}

	// Create decapsulation node of the source hub
	decapNodeID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to create decapsulation node ID: %w", err)
	}

	// Initialize the graph
	graph := &Graph{
		nodes: make(map[uuid.UUID]GraphNode),
		adj:   make(map[uuid.UUID][]GraphEdge),
		srcNode: srcNodeID,
	}

	// Create worker node
	srcNode := WorkerNode{
		ID:        srcNodeID,
		DecapNode: decapNodeID,
	}

	// Create decapsulation node for the worker
	decapNode := DecapsulationNode{
		ID: decapNodeID,
	}

	// Add the nodes to the graph
	graph.nodes[srcNodeID] = srcNode
	graph.nodes[decapNodeID] = decapNode

	// Add edges from the worker node to the decapsulation node
	graph.addEdge(srcNodeID, decapNodeID, 1) // Cost is always 1	

	return graph, nil
}

func (g *Graph) AddVNFGroup(vnfGroup *models.VnfGroup) error {
	if vnfGroup == nil {
		return fmt.Errorf("vnfGroup is nil")
	}

	// Make sure the VNF group ID is valid
	if vnfGroup.ID == uuid.Nil {
		return fmt.Errorf("vnfGroup ID is nil")
	}

	// If the VNF group has a worker ID, use it; otherwise, use the source node.
	hub := g.srcNode
	if vnfGroup.WorkerID != nil {
		hub = *vnfGroup.WorkerID
	}

	// Check if the worker node exists
	if _, exists := g.nodes[hub]; !exists {
		return fmt.Errorf("worker node %s does not exist", hub)
	}

	// Add the VNF group node to the graph
	g.nodes[vnfGroup.ID] = VnfNode{
		ID: vnfGroup.ID,
	}

	// Add edges from the VNF group to the worker node
	g.addEdge(vnfGroup.ID, hub, 1)

	return nil
}

func (g *Graph) AddAppGroup(appGroup *models.AppGroup) error {
	if appGroup == nil {
		return fmt.Errorf("appGroup is nil")
	}

	// Make sure the appGroup ID is valid
	if appGroup.ID == uuid.Nil {
		return fmt.Errorf("appGroup ID is nil")
	}

	// If the VNF group has a worker ID, use it; otherwise, use the source node.
	hub := g.srcNode
	if appGroup.WorkerID != nil {
		hub = *appGroup.WorkerID
	}

	// Check if the worker node exists
	hNode, exists := g.nodes[hub]
	if !exists {
		return fmt.Errorf("worker node %s does not exist", hub)
	}

	// Cast the hubNode variable from GraphNode to WorkerNode
	hubNode, ok := hNode.(WorkerNode)
	if !ok {
		return fmt.Errorf("failed to cast hubNode to WorkerNode")
	}

	// Add the VNF group node to the graph
	g.nodes[appGroup.ID] = AppNode{
		ID: appGroup.ID,
	}

	// Add edges from the VNF group to the worker node's decapsulation node
	g.addEdge(appGroup.ID, hubNode.DecapNode, 1) // Cost is always 1

	return nil
}

func (g *Graph) AddWorker(worker *models.Worker) error {
	if worker == nil {
		return fmt.Errorf("worker is nil")
	}

	// Make sure the worker ID is valid
	if worker.ID == uuid.Nil {
		return fmt.Errorf("worker ID is nil")
	}

	// Create decap node for the worker
	decapNodeID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to create decap node ID for worker %s: %w", worker.ID, err)
	}

	// Create worker node
	srcNode := WorkerNode{
		ID:        worker.ID,
		DecapNode: decapNodeID,
	}

	// Create decapsulation node for the worker
	decapNode := DecapsulationNode{
		ID: decapNodeID,
	}

	// Add the nodes to the graph
	g.nodes[worker.ID] = srcNode
	g.nodes[decapNodeID] = decapNode

	// Add edges
	g.addEdge(worker.ID, decapNodeID, 1) // From worker to decap node
	g.addEdge(g.srcNode, worker.ID, 100) // From worker to source node

	return nil
}

func (g *Graph) RemoveNode(id uuid.UUID) {
	delete(g.nodes, id)
	delete(g.adj, id)
	for _, edges := range g.adj {
		for i, e := range edges {
			if e.To == id {
				edges = append(edges[:i], edges[i+1:]...)
			}
		}
	}
}

func (g *Graph) addEdge(nodeA, nodeB uuid.UUID, cost int) {
	// As links between containers are non-directional, add both directions
	g.adj[nodeA] = append(g.adj[nodeA], GraphEdge{To: nodeB, Cost: cost})
	g.adj[nodeB] = append(g.adj[nodeB], GraphEdge{To: nodeA, Cost: cost})
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
func (g *Graph) ShortestPath(dstID uuid.UUID, vnfs []uuid.UUID) ([]string, error) {
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
	var path []string
	for u, ok := prev[dstID]; ok; u, ok = prev[u] {
		if sid := g.nodes[u].sID(); sid != nil {
			path = append([]string{*sid}, path...)
		}
	}
	return path, nil
}
