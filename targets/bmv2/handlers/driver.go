package handlers

import (
	"bmv2-driver/api"
	p4switch "bmv2-driver/pkg/p4switch"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
)

// Driver holds the P4Runtime client and gRPC connection.
type Driver struct {
	Switch         *p4switch.SwitchClient
	Conn           *grpc.ClientConn
	CurrentProgram *api.P4Program
	Log            logr.Logger
}
