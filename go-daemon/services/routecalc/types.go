package routecalc

import (
	"net"

	"github.com/google/uuid"
)

type GraphNode interface {
	ID() uuid.UUID // Returns the unique identifier of the node
	Type() string  // Returns the type of the node
}

type WorkerNode struct {
	id        uuid.UUID // Unique identifier for the worker node
	DecapSID  net.IP
}
func (w WorkerNode) ID() uuid.UUID {
	return w.id
}
func (WorkerNode) Type() string {
	return "worker"
}

type AppNode struct {
	id    uuid.UUID // Unique identifier for the application node
	appID uuid.UUID // Application ID
}
func (a AppNode) ID() uuid.UUID {
	return a.id
}
func (a AppNode) Type() string {
	return "app"
}

type VnfNode struct {
	id     uuid.UUID // Unique identifier for the VNF node
	VnfID  uuid.UUID // VNF ID
}
func (v VnfNode) ID() uuid.UUID {
	return v.id
}
func (v VnfNode) Type() string {
	return "VNF"
}

type GraphEdge struct {
	To   uuid.UUID
	Cost int
}