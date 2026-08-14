package grpcserver

import (
	"context"
	"errors"
	"net"

	p4switch "github.com/mantra6g/iml/targets/bmv2/pkg/p4switch"

	"github.com/go-logr/logr"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

type Server struct {
	Log           logr.Logger
	listenAddress string
	grpcServer    *grpc.Server
}

type p4RuntimeHandler struct {
	p4v1.UnimplementedP4RuntimeServer
	switchClient *p4switch.SwitchClient
	log          logr.Logger
}

func (h *p4RuntimeHandler) Write(ctx context.Context, request *p4v1.WriteRequest) (*p4v1.WriteResponse, error) {
	response, err := h.switchClient.ApplyUpdates(ctx, request.GetUpdates())
	if err != nil {
		h.log.Error(err, "failed to apply updates to switch", "numUpdates", len(request.GetUpdates()))
		return nil, err
	}
	return response, nil
}

func NewServer(listenAddress string, switchClient *p4switch.SwitchClient, log logr.Logger) *Server {
	grpcServer := grpc.NewServer()
	p4v1.RegisterP4RuntimeServer(grpcServer, &p4RuntimeHandler{
		switchClient: switchClient,
		log:          log,
	})

	return &Server{
		Log:           log,
		listenAddress: listenAddress,
		grpcServer:    grpcServer,
	}
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.listenAddress)
	if err != nil {
		return err
	}

	s.Log.Info("Starting P4Runtime gRPC server", "listenAddress", s.listenAddress)

	stopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	if err := s.grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		s.Log.Error(err, "Encountered error while running P4Runtime gRPC server")
		return err
	}
	<-stopped
	return nil
}
