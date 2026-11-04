package server

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/mantra6g/iml/dpcs/internal/devicemap"

	"github.com/go-logr/logr"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
)

type DriverAddressResolver interface {
	Resolve(ctx context.Context, p4targetName string) (string, error)
}

type Server struct {
	p4v1.UnimplementedP4RuntimeServer

	deviceMapFile         string
	driverAddressResolver DriverAddressResolver
	log                   logr.Logger

	mu                              sync.Mutex
	deviceIDToTargetNames           map[uint64][]string
	connections                     map[string]driverConnection
	loadedDeviceMapModificationTime time.Time
	lastDeviceMapCheckTime          time.Time
}

type driverConnection struct {
	address    string
	connection *grpc.ClientConn
}

const deviceMapCheckInterval = time.Second

func New(deviceMapFile string, deviceIDToTargetNames map[uint64][]string, driverAddressResolver DriverAddressResolver, log logr.Logger) *Server {
	return &Server{
		deviceMapFile:                   deviceMapFile,
		deviceIDToTargetNames:           deviceIDToTargetNames,
		driverAddressResolver:           driverAddressResolver,
		log:                             log,
		connections:                     make(map[string]driverConnection),
		loadedDeviceMapModificationTime: deviceMapModificationTime(deviceMapFile),
	}
}

func deviceMapModificationTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (s *Server) getTargetNamesByDeviceID(deviceID uint64) ([]string, bool) {
	s.reloadDeviceMapIfChanged()
	s.mu.Lock()
	defer s.mu.Unlock()
	names, ok := s.deviceIDToTargetNames[deviceID]
	return names, ok
}

func (s *Server) reloadDeviceMapIfChanged() {
	s.mu.Lock()
	if time.Since(s.lastDeviceMapCheckTime) < deviceMapCheckInterval {
		s.mu.Unlock()
		return
	}
	s.lastDeviceMapCheckTime = time.Now()
	previousModificationTime := s.loadedDeviceMapModificationTime
	s.mu.Unlock()

	modificationTime := deviceMapModificationTime(s.deviceMapFile)
	if modificationTime.Equal(previousModificationTime) {
		return
	}
	fresh, err := devicemap.ParseIfExists(s.deviceMapFile)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadedDeviceMapModificationTime = modificationTime
	if err != nil {
		s.log.Error(err, "failed to re-read device-map file, keeping the previous map", "path", s.deviceMapFile)
		return
	}
	s.deviceIDToTargetNames = fresh
	for p4targetName, cached := range s.connections {
		if !referencesTarget(fresh, p4targetName) {
			_ = cached.connection.Close()
			delete(s.connections, p4targetName)
		}
	}
	s.log.Info("device-map reloaded", "numDevices", len(fresh))
}

func referencesTarget(deviceIDToTargetNames map[uint64][]string, p4targetName string) bool {
	for _, names := range deviceIDToTargetNames {
		for _, name := range names {
			if name == p4targetName {
				return true
			}
		}
	}
	return false
}

func (s *Server) targetNamesOrNotFound(deviceID uint64) ([]string, error) {
	names, ok := s.getTargetNamesByDeviceID(deviceID)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown device_id %d", deviceID)
	}
	return names, nil
}

func (s *Server) Write(ctx context.Context, request *p4v1.WriteRequest) (*p4v1.WriteResponse, error) {
	p4targetNames, err := s.targetNamesOrNotFound(request.GetDeviceId())
	if err != nil {
		return nil, err
	}

	errs := make([]error, len(p4targetNames))
	var wg sync.WaitGroup
	for i, p4targetName := range p4targetNames {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.writeToDriver(ctx, p4targetName, request)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return &p4v1.WriteResponse{}, nil
}

func (s *Server) writeToDriver(ctx context.Context, p4targetName string, request *p4v1.WriteRequest) error {
	connection, err := s.getDriverConnection(ctx, p4targetName)
	if err != nil {
		return err
	}

	if _, err := p4v1.NewP4RuntimeClient(connection).Write(ctx, request); err != nil {
		s.log.Error(err, "driver Write failed",
			"deviceId", request.GetDeviceId(), "p4target", p4targetName, "numUpdates", len(request.GetUpdates()))
		s.logUnwrappedP4Errors(err, request.GetDeviceId(), p4targetName)
		s.evictConnectionOnTransportError(err, p4targetName)
		return err
	}
	s.log.V(1).Info("forwarded Write to driver",
		"deviceId", request.GetDeviceId(), "p4target", p4targetName, "numUpdates", len(request.GetUpdates()))
	return nil
}

func (s *Server) GetForwardingPipelineConfig(ctx context.Context, request *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	p4targetNames, err := s.targetNamesOrNotFound(request.GetDeviceId())
	if err != nil {
		return nil, err
	}
	p4targetName := p4targetNames[0]
	connection, err := s.getDriverConnection(ctx, p4targetName)
	if err != nil {
		return nil, err
	}

	response, err := p4v1.NewP4RuntimeClient(connection).GetForwardingPipelineConfig(ctx, request)
	if err != nil {
		s.log.Error(err, "driver GetForwardingPipelineConfig failed", "deviceId", request.GetDeviceId(), "p4target", p4targetName)
		s.evictConnectionOnTransportError(err, p4targetName)
		return nil, err
	}
	s.log.V(1).Info("forwarded GetForwardingPipelineConfig to driver", "deviceId", request.GetDeviceId(), "p4target", p4targetName)
	return response, nil
}

func (s *Server) Read(request *p4v1.ReadRequest, stream p4v1.P4Runtime_ReadServer) error {
	ctx := stream.Context()
	p4targetNames, err := s.targetNamesOrNotFound(request.GetDeviceId())
	if err != nil {
		return err
	}
	p4targetName := p4targetNames[0]
	connection, err := s.getDriverConnection(ctx, p4targetName)
	if err != nil {
		return err
	}

	driverStream, err := p4v1.NewP4RuntimeClient(connection).Read(ctx, request)
	if err != nil {
		s.log.Error(err, "driver Read failed", "deviceId", request.GetDeviceId(), "p4target", p4targetName)
		s.evictConnectionOnTransportError(err, p4targetName)
		return err
	}
	numResponses := 0
	for {
		response, err := driverStream.Recv()
		if errors.Is(err, io.EOF) {
			s.log.V(1).Info("forwarded Read to driver",
				"deviceId", request.GetDeviceId(), "p4target", p4targetName, "numResponses", numResponses)
			return nil
		}
		if err != nil {
			s.log.Error(err, "driver Read stream failed", "deviceId", request.GetDeviceId(), "p4target", p4targetName)
			s.evictConnectionOnTransportError(err, p4targetName)
			return err
		}
		if err := stream.Send(response); err != nil {
			return err
		}
		numResponses++
	}
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

func (s *Server) getDriverConnection(ctx context.Context, p4targetName string) (*grpc.ClientConn, error) {
	address, err := s.driverAddressResolver.Resolve(ctx, p4targetName)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to resolve driver for P4Target %q: %v", p4targetName, err)
	}

	s.mu.Lock()
	cached, ok := s.connections[p4targetName]
	s.mu.Unlock()
	if ok && cached.address == address {
		return cached.connection, nil
	}
	if ok {
		s.log.Info("driver address changed, dropping cached connection",
			"p4target", p4targetName, "oldAddress", cached.address, "newAddress", address)
		s.evictConnection(p4targetName)
	}

	newConnection, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "failed to dial driver at %s for P4Target %q: %v",
			address, p4targetName, err)
	}

	s.mu.Lock()
	existing, ok := s.connections[p4targetName]
	if ok && existing.address == address {
		s.mu.Unlock()
		_ = newConnection.Close()
		return existing.connection, nil
	}
	if ok {
		_ = existing.connection.Close()
	}
	s.connections[p4targetName] = driverConnection{address: address, connection: newConnection}
	s.mu.Unlock()
	return newConnection, nil
}

func (s *Server) logUnwrappedP4Errors(err error, deviceID uint64, p4targetName string) {
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
			"deviceId", deviceID, "p4target", p4targetName, "updateIndex", index,
			"code", codes.Code(p4Error.GetCanonicalCode()).String(),
			"space", p4Error.GetSpace(), "message", p4Error.GetMessage())
	}
}

func (s *Server) evictConnectionOnTransportError(err error, p4targetName string) {
	if code := status.Code(err); code == codes.Unavailable || code == codes.DeadlineExceeded {
		s.log.Info("dropping cached driver connection", "p4target", p4targetName, "code", code.String())
		s.evictConnection(p4targetName)
	}
}

func (s *Server) evictConnection(p4targetName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.connections[p4targetName]; ok {
		_ = cached.connection.Close()
		delete(s.connections, p4targetName)
	}
}
