package routecalc

import (
	"iml-daemon/logger"
	"net"

	"github.com/google/uuid"
)

type FunctionSelector struct {
	FunctionID    uuid.UUID
	SubfunctionID *uint32 // nil if not applicable
}

type GraphNode interface {
	ID() uuid.UUID // Returns the unique identifier of the node
}

type WorkerNode struct {
	id       uuid.UUID // Unique identifier for the worker node
	DecapSID net.IP
}

func (w WorkerNode) ID() uuid.UUID {
	return w.id
}

type AppNode struct {
	id    uuid.UUID // Unique identifier for the application node
	appID uuid.UUID // Application ID
}

func (a AppNode) ID() uuid.UUID {
	return a.id
}

type VnfNode interface {
	GraphNode
	// Returns the SID that satisfies the given function selector, or nil if it doesn't satisfy
	GetSIDThatSatisfies(funcSel FunctionSelector) *net.IPNet
}

type SimpleVnfNode struct {
	id    uuid.UUID // Unique identifier for the VNF node
	VnfID uuid.UUID // VNF ID
	SID   net.IPNet // in "IP/prefix" format
}

func (v *SimpleVnfNode) ID() uuid.UUID {
	return v.id
}
func (v *SimpleVnfNode) GetSIDThatSatisfies(funcSel FunctionSelector) *net.IPNet {
	logger.DebugLogger().Printf("Checking if VNF node %s satisfies function selector %+v", v.id, funcSel)
	if funcSel.SubfunctionID != nil || v.VnfID != funcSel.FunctionID {
		logger.DebugLogger().Printf("VNF node %s does not satisfy function selector %+v", v.id, funcSel)
		return nil
	}
	logger.DebugLogger().Printf("VNF node %s satisfies function selector %+v", v.id, funcSel)
	return &v.SID
}

type MultiplexedVnfNode struct {
	id              uuid.UUID // Unique identifier for the VNF node
	VnfID           uuid.UUID // VNF ID
	SubfunctionSids map[uint32]net.IPNet
}

func (v *MultiplexedVnfNode) ID() uuid.UUID {
	return v.id
}
func (v *MultiplexedVnfNode) GetSIDThatSatisfies(funcSel FunctionSelector) *net.IPNet {
	logger.DebugLogger().Printf("Checking if multiplexed VNF node %s satisfies function selector %+v", v.id, funcSel)
	if funcSel.SubfunctionID == nil || funcSel.FunctionID != v.VnfID {
		logger.DebugLogger().Printf("Multiplexed VNF node %s does not satisfy function selector %+v", v.id, funcSel)
		return nil
	}
	sid, exists := v.SubfunctionSids[*funcSel.SubfunctionID]
	if !exists {
		logger.DebugLogger().Printf("Multiplexed VNF node %s does not have SID for subfunction ID %d", v.id, *funcSel.SubfunctionID)
		return nil
	}
	logger.DebugLogger().Printf("Multiplexed VNF node %s satisfies function selector %+v", v.id, funcSel)
	return &sid
}

type GraphEdge struct {
	To   uuid.UUID
	Cost int
}

type Route struct {
	ChainID       uuid.UUID
	SrcAppGroupID uuid.UUID
	DstAppGroupID uuid.UUID
	VnfSIDs       []net.IPNet
}
