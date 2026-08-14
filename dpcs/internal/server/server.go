package server

import (
	"context"
	"sync"

	"github.com/mantra6g/iml/dpcs/internal/devicemap"
	"github.com/mantra6g/iml/dpcs/internal/resolver"

	"github.com/go-logr/logr"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
)

type Server struct {
	p4v1.UnimplementedP4RuntimeServer

	deviceMapFile         string
	driverAddressResolver *resolver.Resolver
	log                   logr.Logger

	mu                   sync.Mutex
	deviceIDToTargetName map[uint64]string
	connections          map[uint64]*grpc.ClientConn // device_id -> cached driver connection
}

func New(deviceMapFile string, deviceIDToTargetName map[uint64]string, driverAddressResolver *resolver.Resolver, log logr.Logger) *Server {
	return &Server{
		deviceMapFile:         deviceMapFile,
		deviceIDToTargetName:  deviceIDToTargetName,
		driverAddressResolver: driverAddressResolver,
		log:                   log,
		connections:           make(map[uint64]*grpc.ClientConn),
	}
}

// TODO: throttle the re-read — a flood of unknown device_ids currently triggers a file read per request.
func (s *Server) getTargetNameByDeviceID(deviceID uint64) (string, bool) {
	s.mu.Lock()
	name, ok := s.deviceIDToTargetName[deviceID]
	s.mu.Unlock()
	if ok {
		return name, true
	}

	fresh, err := devicemap.ParseIfExists(s.deviceMapFile)
	if err != nil {
		s.log.Error(err, "failed to re-read device-map file", "path", s.deviceMapFile)
		return "", false
	}
	s.mu.Lock()
	s.deviceIDToTargetName = fresh
	name, ok = s.deviceIDToTargetName[deviceID]
	s.mu.Unlock()
	if ok {
		s.log.Info("device-map reloaded", "numDevices", len(fresh))
	}
	return name, ok
}

func (s *Server) Write(ctx context.Context, request *p4v1.WriteRequest) (*p4v1.WriteResponse, error) {
	connection, err := s.getDriverConnection(ctx, request.GetDeviceId())
	if err != nil {
		return nil, err
	}

	response, err := p4v1.NewP4RuntimeClient(connection).Write(ctx, request)
	if err != nil {
		s.log.Error(err, "driver Write failed",
			"deviceId", request.GetDeviceId(), "numUpdates", len(request.GetUpdates()))
		s.logUnwrappedP4Errors(err, request.GetDeviceId())
		if code := status.Code(err); code == codes.Unavailable || code == codes.DeadlineExceeded {
			s.log.Info("dropping cached driver connection", "deviceId", request.GetDeviceId(), "code", code.String())
			s.evictConnection(request.GetDeviceId())
		}
		return nil, err
	}
	s.log.V(1).Info("forwarded Write to driver", "deviceId", request.GetDeviceId(), "numUpdates", len(request.GetUpdates()))
	return response, nil
}

func (s *Server) StreamChannel(stream p4v1.P4Runtime_StreamChannelServer) error {
	for {
		request, err := stream.Recv()
		if err != nil {
			return err
		}

		arbitration := request.GetArbitration()
		if arbitration == nil {
			s.log.Info("ignoring non-arbitration StreamChannel message")
			continue
		}

		ack := &p4v1.StreamMessageResponse{
			Update: &p4v1.StreamMessageResponse_Arbitration{
				Arbitration: &p4v1.MasterArbitrationUpdate{
					DeviceId:   arbitration.GetDeviceId(),
					ElectionId: arbitration.GetElectionId(),
					Status:     &rpcstatus.Status{Code: int32(codes.OK)},
				},
			},
		}
		if err := stream.Send(ack); err != nil {
			return err
		}
	}
}

func (s *Server) getDriverConnection(ctx context.Context, deviceID uint64) (*grpc.ClientConn, error) {
	s.mu.Lock()
	connection, ok := s.connections[deviceID]
	s.mu.Unlock()
	if ok {
		return connection, nil
	}

	p4targetName, ok := s.getTargetNameByDeviceID(deviceID)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown device_id %d", deviceID)
	}

	address, err := s.driverAddressResolver.Resolve(ctx, p4targetName)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to resolve driver for device_id %d (P4Target %q): %v",
			deviceID, p4targetName, err)
	}

	newConnection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to dial driver at %s for device_id %d: %v",
			address, deviceID, err)
	}

	s.mu.Lock()
	if existingConnection, ok := s.connections[deviceID]; ok {
		s.mu.Unlock()
		_ = newConnection.Close()
		return existingConnection, nil
	}
	s.connections[deviceID] = newConnection
	s.mu.Unlock()
	return newConnection, nil
}

func (s *Server) logUnwrappedP4Errors(err error, deviceID uint64) {
	grpcStatus, ok := status.FromError(err)
	if !ok {
		return
	}
	for index, detail := range grpcStatus.Details() {
		p4Error, ok := detail.(*p4v1.Error)
		if !ok || codes.Code(p4Error.GetCanonicalCode()) == codes.OK {
			continue
		}
		s.log.Error(nil, "update rejected by switch",
			"deviceId", deviceID, "updateIndex", index,
			"code", codes.Code(p4Error.GetCanonicalCode()).String(),
			"space", p4Error.GetSpace(), "message", p4Error.GetMessage())
	}
}

func (s *Server) evictConnection(deviceID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if connection, ok := s.connections[deviceID]; ok {
		_ = connection.Close()
		delete(s.connections, deviceID)
	}
}
