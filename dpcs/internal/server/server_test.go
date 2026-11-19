package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mantra6g/iml/dpcs/internal/devicemap"

	"github.com/go-logr/logr"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type fakeDriver struct {
	p4v1.UnimplementedP4RuntimeServer
	id       uint64
	writeErr error

	mutex               sync.Mutex
	writes              []*p4v1.WriteRequest
	reads               int
	pipelineConfigReads int
}

func (d *fakeDriver) Write(_ context.Context, request *p4v1.WriteRequest) (*p4v1.WriteResponse, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.writes = append(d.writes, request)
	if d.writeErr != nil {
		return nil, d.writeErr
	}
	return &p4v1.WriteResponse{}, nil
}

func (d *fakeDriver) Read(_ *p4v1.ReadRequest, stream p4v1.P4Runtime_ReadServer) error {
	d.mutex.Lock()
	d.reads++
	d.mutex.Unlock()
	return stream.Send(&p4v1.ReadResponse{Entities: []*p4v1.Entity{{
		Entity: &p4v1.Entity_TableEntry{TableEntry: &p4v1.TableEntry{TableId: uint32(d.id)}},
	}}})
}

func (d *fakeDriver) GetForwardingPipelineConfig(
	context.Context, *p4v1.GetForwardingPipelineConfigRequest,
) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	d.mutex.Lock()
	d.pipelineConfigReads++
	d.mutex.Unlock()
	return &p4v1.GetForwardingPipelineConfigResponse{Config: &p4v1.ForwardingPipelineConfig{
		Cookie: &p4v1.ForwardingPipelineConfig_Cookie{Cookie: d.id},
	}}, nil
}

func (d *fakeDriver) receivedWrites() []*p4v1.WriteRequest {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return append([]*p4v1.WriteRequest(nil), d.writes...)
}

type fakeResolver map[string]string

func (r fakeResolver) Resolve(_ context.Context, p4targetName string) (string, error) {
	address, ok := r[p4targetName]
	if !ok {
		return "", fmt.Errorf("P4Target %q not found", p4targetName)
	}
	return address, nil
}

func serveP4Runtime(t *testing.T, implementation p4v1.P4RuntimeServer) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	p4v1.RegisterP4RuntimeServer(grpcServer, implementation)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	return listener.Addr().String()
}

func startDPCS(t *testing.T, deviceMap string, drivers map[string]*fakeDriver) (*Server, p4v1.P4RuntimeClient) {
	t.Helper()
	deviceMapFile := filepath.Join(t.TempDir(), "device-map.env")
	if err := os.WriteFile(deviceMapFile, []byte(deviceMap), 0o600); err != nil {
		t.Fatalf("write device-map file: %v", err)
	}
	deviceIDToTargetNames, err := devicemap.Parse(deviceMapFile)
	if err != nil {
		t.Fatalf("parse device-map file: %v", err)
	}
	driverAddresses := fakeResolver{}
	for name, driver := range drivers {
		driverAddresses[name] = serveP4Runtime(t, driver)
	}
	dpcs := New(deviceMapFile, deviceIDToTargetNames, driverAddresses, logr.Discard())

	connection, err := grpc.NewClient(serveP4Runtime(t, dpcs), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial DPCS: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return dpcs, p4v1.NewP4RuntimeClient(connection)
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func writeRequest(deviceID uint64) *p4v1.WriteRequest {
	return &p4v1.WriteRequest{
		DeviceId: deviceID,
		Updates: []*p4v1.Update{{
			Type:   p4v1.Update_INSERT,
			Entity: &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: &p4v1.TableEntry{TableId: 42}}},
		}},
	}
}

func assertSingleWrite(t *testing.T, name string, driver *fakeDriver, deviceID uint64) {
	t.Helper()
	writes := driver.receivedWrites()
	if len(writes) != 1 {
		t.Fatalf("driver %s received %d writes, want 1", name, len(writes))
	}
	if writes[0].GetDeviceId() != deviceID || len(writes[0].GetUpdates()) != 1 {
		t.Errorf("driver %s received %v, want device_id %d with 1 update", name, writes[0], deviceID)
	}
}

func TestWriteFansOutToAllTargets(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}, "lb-c": {id: 3}}
	_, client := startDPCS(t, "1=lb-a,lb-b,lb-c\n", drivers)

	if _, err := client.Write(testContext(t), writeRequest(1)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	for name, driver := range drivers {
		assertSingleWrite(t, name, driver, 1)
	}
}

func TestWriteReturnsFirstErrorAfterWritingAllTargets(t *testing.T) {
	rejected, err := status.New(codes.InvalidArgument, "rejected by driver").WithDetails(
		&p4v1.Error{CanonicalCode: int32(codes.AlreadyExists), Message: "entry exists"})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	for _, failing := range []string{"lb-a", "lb-b"} {
		t.Run(failing+" fails", func(t *testing.T) {
			drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}}
			drivers[failing].writeErr = rejected.Err()
			_, client := startDPCS(t, "1=lb-a,lb-b\n", drivers)

			_, err := client.Write(testContext(t), writeRequest(1))
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("Write returned %v, want InvalidArgument", err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("Write error carries %d details, want the driver's p4 error", len(details))
			}
			if p4Error, ok := details[0].(*p4v1.Error); !ok || p4Error.GetMessage() != "entry exists" {
				t.Errorf("Write error detail is %v, want the driver's p4 error", details[0])
			}
			for name, driver := range drivers {
				assertSingleWrite(t, name, driver, 1)
			}
		})
	}
}

func TestWriteStillReachesTargetsListedAfterAnUnresolvableOne(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-b": {id: 2}}
	_, client := startDPCS(t, "1=missing,lb-b\n", drivers)

	_, err := client.Write(testContext(t), writeRequest(1))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Write returned %v, want Unavailable", err)
	}
	assertSingleWrite(t, "lb-b", drivers["lb-b"], 1)
}

func TestReadAndPipelineConfigUseFirstTarget(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}}
	_, client := startDPCS(t, "1=lb-a,lb-b\n", drivers)
	ctx := testContext(t)

	config, err := client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{DeviceId: 1})
	if err != nil {
		t.Fatalf("GetForwardingPipelineConfig failed: %v", err)
	}
	if cookie := config.GetConfig().GetCookie().GetCookie(); cookie != 1 {
		t.Errorf("pipeline config came from driver %d, want lb-a (1)", cookie)
	}

	stream, err := client.Read(ctx, &p4v1.ReadRequest{DeviceId: 1})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	response, err := stream.Recv()
	if err != nil {
		t.Fatalf("Read stream failed: %v", err)
	}
	if tableID := response.GetEntities()[0].GetTableEntry().GetTableId(); tableID != 1 {
		t.Errorf("read response came from driver %d, want lb-a (1)", tableID)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Errorf("Read stream ended with %v, want EOF", err)
	}

	if drivers["lb-b"].reads != 0 || drivers["lb-b"].pipelineConfigReads != 0 {
		t.Errorf("lb-b served %d reads and %d pipeline config reads, want none",
			drivers["lb-b"].reads, drivers["lb-b"].pipelineConfigReads)
	}
}

func TestUnknownDeviceIDIsNotFound(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}}
	_, client := startDPCS(t, "1=lb-a\n", drivers)

	_, err := client.Write(testContext(t), writeRequest(2))
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Write returned %v, want NotFound", err)
	}
	if len(drivers["lb-a"].receivedWrites()) != 0 {
		t.Errorf("lb-a received a write for an unknown device")
	}
}

func TestDriverConnectionIsSharedBetweenDevicesOfTheSameTarget(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}}
	dpcs, client := startDPCS(t, "1=lb-a,lb-b\n2=lb-a\n", drivers)
	ctx := testContext(t)

	for _, deviceID := range []uint64{1, 2} {
		if _, err := client.Write(ctx, writeRequest(deviceID)); err != nil {
			t.Fatalf("Write for device %d failed: %v", deviceID, err)
		}
	}
	dpcs.mu.Lock()
	numConnections := len(dpcs.connections)
	dpcs.mu.Unlock()
	if numConnections != 2 {
		t.Errorf("DPCS caches %d driver connections, want one per P4Target (2)", numConnections)
	}
	if writes := drivers["lb-a"].receivedWrites(); len(writes) != 2 {
		t.Errorf("lb-a received %d writes, want 2", len(writes))
	}
}

func TestConcurrentWritesShareOneConnectionPerTarget(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}}
	dpcs, client := startDPCS(t, "1=lb-a,lb-b\n", drivers)
	ctx := testContext(t)

	const writers, writesPerWriter = 16, 10
	errs := make(chan error, writers*writesPerWriter)
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range writesPerWriter {
				if _, err := client.Write(ctx, writeRequest(1)); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Write failed: %v", err)
	}

	for name, driver := range drivers {
		if got := len(driver.receivedWrites()); got != writers*writesPerWriter {
			t.Errorf("%s received %d writes, want %d", name, got, writers*writesPerWriter)
		}
	}
	dpcs.mu.Lock()
	defer dpcs.mu.Unlock()
	if len(dpcs.connections) != 2 {
		t.Errorf("cached connections: %d, want 2", len(dpcs.connections))
	}
}

func TestDeviceMapChangeIsPickedUpWithoutRestart(t *testing.T) {
	drivers := map[string]*fakeDriver{"lb-a": {id: 1}, "lb-b": {id: 2}}
	dpcs, client := startDPCS(t, "1=lb-a\n", drivers)
	ctx := testContext(t)

	if _, err := client.Write(ctx, writeRequest(1)); err != nil {
		t.Fatalf("Write before the change: %v", err)
	}
	rewriteDeviceMap(t, dpcs, "1=lb-b\n")
	if _, err := client.Write(ctx, writeRequest(1)); err != nil {
		t.Fatalf("Write after the change: %v", err)
	}

	assertSingleWrite(t, "lb-a", drivers["lb-a"], 1)
	assertSingleWrite(t, "lb-b", drivers["lb-b"], 1)
	dpcs.mu.Lock()
	defer dpcs.mu.Unlock()
	if _, cached := dpcs.connections["lb-a"]; cached {
		t.Errorf("connection to the removed target lb-a is still cached")
	}
	if len(dpcs.connections) != 1 {
		t.Errorf("cached connections: %d, want 1", len(dpcs.connections))
	}
}

func rewriteDeviceMap(t *testing.T, dpcs *Server, content string) {
	t.Helper()
	if err := os.WriteFile(dpcs.deviceMapFile, []byte(content), 0o600); err != nil {
		t.Fatalf("rewrite device-map file: %v", err)
	}
	modificationTime := time.Now().Add(time.Hour)
	if err := os.Chtimes(dpcs.deviceMapFile, modificationTime, modificationTime); err != nil {
		t.Fatalf("touch device-map file: %v", err)
	}
	dpcs.mu.Lock()
	dpcs.lastDeviceMapCheckTime = time.Time{}
	dpcs.mu.Unlock()
}
