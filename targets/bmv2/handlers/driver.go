package handlers

import (
	"bmv2-driver/api"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

// Driver holds the P4Runtime client and gRPC connection.
type Driver struct {
	Client         v1.P4RuntimeClient
	Conn           *grpc.ClientConn
	CurrentProgram *api.P4Program
	DeviceID       uint64
	ElectionIDHigh uint64
	ElectionIDLow  uint64
}
