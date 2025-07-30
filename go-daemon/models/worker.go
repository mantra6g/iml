package models

import "github.com/google/uuid"

// Worker represents a worker node in the Kubernetes cluster.
// This is used to associate application and VNF groups with specific workers.
// The WorkerID is nullable to represent an App or VNF group from THIS node.
type Worker struct {
	ID            uuid.UUID `gorm:"primaryKey"`
	ClusterNodeID string    `gorm:"unique_index:cluster_node_id"`
	IP            string // in "IP" format (without prefix)
	HubSID        string // in "IP/prefix" format
}