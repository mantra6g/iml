package dpcsclient

import (
	"bytes"
	"context"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type fakeP4Runtime struct {
	p4v1.UnimplementedP4RuntimeServer
	arbitrationCode codes.Code
	silent          bool
	firstResponse   *p4v1.StreamMessageResponse
	breakStream     chan struct{}
	mutex           sync.Mutex
	tableEntries    []*p4v1.TableEntry
}

func (s *fakeP4Runtime) StreamChannel(stream p4v1.P4Runtime_StreamChannelServer) error {
	request, err := stream.Recv()
	if err != nil {
		return err
	}
	if !s.silent {
		firstResponse := s.firstResponse
		if firstResponse == nil {
			firstResponse = &p4v1.StreamMessageResponse{Update: &p4v1.StreamMessageResponse_Arbitration{
				Arbitration: &p4v1.MasterArbitrationUpdate{
					DeviceId: request.GetArbitration().GetDeviceId(),
					Status:   &rpcstatus.Status{Code: int32(s.arbitrationCode)},
				},
			}}
		}
		if err := stream.Send(firstResponse); err != nil {
			return err
		}
	}
	select {
	case <-stream.Context().Done():
		return nil
	case <-s.breakStream:
		return status.Error(codes.Unavailable, "stream closed by server")
	}
}

func (s *fakeP4Runtime) GetForwardingPipelineConfig(
	context.Context, *p4v1.GetForwardingPipelineConfigRequest,
) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	return &p4v1.GetForwardingPipelineConfigResponse{
		Config: &p4v1.ForwardingPipelineConfig{P4Info: testP4Info()},
	}, nil
}

func (s *fakeP4Runtime) Write(_ context.Context, request *p4v1.WriteRequest) (*p4v1.WriteResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, update := range request.GetUpdates() {
		if update.GetType() == p4v1.Update_INSERT {
			s.tableEntries = append(s.tableEntries, update.GetEntity().GetTableEntry())
		}
	}
	return &p4v1.WriteResponse{}, nil
}

func (s *fakeP4Runtime) Read(_ *p4v1.ReadRequest, stream p4v1.P4Runtime_ReadServer) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	response := &p4v1.ReadResponse{}
	for _, tableEntry := range s.tableEntries {
		response.Entities = append(response.Entities, &p4v1.Entity{
			Entity: &p4v1.Entity_TableEntry{TableEntry: tableEntry},
		})
	}
	return stream.Send(response)
}

func startFakeP4Runtime(t *testing.T, server *fakeP4Runtime) Config {
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	p4v1.RegisterP4RuntimeServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	return Config{
		Address:  "passthrough:///bufnet",
		DeviceID: 1,
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return listener.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	}
}

func connectFake(t *testing.T, server *fakeP4Runtime) *Client {
	client, err := Connect(context.Background(), startFakeP4Runtime(t, server))
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestConnectBecomesPrimary(t *testing.T) {
	client := connectFake(t, &fakeP4Runtime{})
	if findTableInfo(client.P4Info(), "MyIngress.log_table") == nil {
		t.Error("P4Info was not loaded")
	}
}

func TestConnectNotPrimary(t *testing.T) {
	config := startFakeP4Runtime(t, &fakeP4Runtime{arbitrationCode: codes.AlreadyExists})
	_, err := Connect(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "not primary") {
		t.Errorf("expected a not primary error, got %v", err)
	}
}

func TestConnectRejectsNonArbitrationFirstMessage(t *testing.T) {
	config := startFakeP4Runtime(t, &fakeP4Runtime{firstResponse: &p4v1.StreamMessageResponse{
		Update: &p4v1.StreamMessageResponse_Packet{Packet: &p4v1.PacketIn{Payload: []byte{1}}},
	}})
	_, err := Connect(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "unexpected first stream message") {
		t.Errorf("expected an unexpected first stream message error, got %v", err)
	}
}

func TestConnectRejectsArbitrationWithoutStatus(t *testing.T) {
	config := startFakeP4Runtime(t, &fakeP4Runtime{firstResponse: &p4v1.StreamMessageResponse{
		Update: &p4v1.StreamMessageResponse_Arbitration{Arbitration: &p4v1.MasterArbitrationUpdate{DeviceId: 1}},
	}})
	_, err := Connect(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "without status") {
		t.Errorf("expected a response without status error, got %v", err)
	}
}

func TestConnectArbitrationTimeout(t *testing.T) {
	config := startFakeP4Runtime(t, &fakeP4Runtime{silent: true})
	config.ArbitrationTimeout = 100 * time.Millisecond
	_, err := Connect(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "no response") {
		t.Errorf("expected an arbitration timeout error, got %v", err)
	}
}

func TestConnectHonorsContextWhileDialing(t *testing.T) {
	blockingDialer := grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	config := Config{
		Address:     "passthrough:///unreachable",
		DeviceID:    1,
		DialOptions: []grpc.DialOption{blockingDialer, grpc.WithTransportCredentials(insecure.NewCredentials())},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := Connect(ctx, config)
	if err == nil || time.Since(started) > 2*time.Second {
		t.Errorf("expected a context error within the deadline, got %v after %s", err, time.Since(started))
	}

	config.ArbitrationTimeout = 100 * time.Millisecond
	_, err = Connect(context.Background(), config)
	if err == nil || !strings.Contains(err.Error(), "no response") {
		t.Errorf("expected an arbitration timeout error, got %v", err)
	}
}

func TestCloseStopsSessionAndIsIdempotent(t *testing.T) {
	config := startFakeP4Runtime(t, &fakeP4Runtime{})
	goroutinesBefore := runtime.NumGoroutine()
	client, err := Connect(context.Background(), config)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("first close failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("second close should return nil, got %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > goroutinesBefore && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if goroutines := runtime.NumGoroutine(); goroutines > goroutinesBefore {
		t.Errorf("goroutines leaked: %d before connect, %d after close", goroutinesBefore, goroutines)
	}
}

func TestSessionLostIsReturnedByNextOperation(t *testing.T) {
	server := &fakeP4Runtime{breakStream: make(chan struct{})}
	client := connectFake(t, server)
	close(server.breakStream)
	entry := Entry{
		Table:  "MyIngress.log_table",
		Match:  map[string]FieldMatch{"hdr.inner_ipv4.src_addr": ExactIP(net.ParseIP("10.0.0.1"))},
		Action: "MyIngress.log",
	}
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for err == nil && time.Now().Before(deadline) {
		err = client.Insert(context.Background(), entry)
		time.Sleep(10 * time.Millisecond)
	}
	if err == nil || !strings.Contains(err.Error(), "session lost") {
		t.Errorf("expected a session lost error, got %v", err)
	}
}

func TestLPMMasksCopyOfValue(t *testing.T) {
	value := []byte{10, 1, 2, 3}
	matchFieldInfo := findMatchFieldInfo(findTableInfo(testP4Info(), "MyIngress.ipv4_lpm"), "hdr.ipv4.dst_addr")
	p4rtMatch, err := p4rtFieldMatch(matchFieldInfo, LPM(value, 12))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if !bytes.Equal(p4rtMatch.GetLpm().GetValue(), []byte{10, 0, 0, 0}) {
		t.Errorf("unexpected masked value: %x", p4rtMatch.GetLpm().GetValue())
	}
	if !bytes.Equal(value, []byte{10, 1, 2, 3}) {
		t.Errorf("caller value was modified: %x", value)
	}
}

func TestReadBackCanonicalLPMIsWritable(t *testing.T) {
	client := &Client{p4Info: testP4Info()}
	entry, err := decodeTableEntry(client.p4Info, &p4v1.TableEntry{
		TableId: 101,
		Match: []*p4v1.FieldMatch{{
			FieldId:        1,
			FieldMatchType: &p4v1.FieldMatch_Lpm{Lpm: &p4v1.FieldMatch_LPM{Value: []byte{1, 0}, PrefixLen: 24}},
		}},
		Action: &p4v1.TableAction{Type: &p4v1.TableAction_Action{Action: &p4v1.Action{ActionId: 200}}},
	})
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	tableEntry, err := client.p4rtTableEntry(entry)
	if err != nil {
		t.Fatalf("read-back entry rejected: %v", err)
	}
	if value := tableEntry.GetMatch()[0].GetLpm().GetValue(); !bytes.Equal(value, []byte{0, 0, 1, 0}) {
		t.Errorf("expected the value padded to the field width, got %x", value)
	}
}

func TestMatchValuesAreCheckedAgainstP4Info(t *testing.T) {
	client := &Client{p4Info: testP4Info()}
	lpmEntry := func(match FieldMatch) Entry {
		return Entry{Table: "MyIngress.ipv4_lpm", Match: map[string]FieldMatch{"hdr.ipv4.dst_addr": match}, Action: "MyIngress.log"}
	}
	exactEntry := func(match FieldMatch) Entry {
		return Entry{Table: "MyIngress.log_table", Match: map[string]FieldMatch{"hdr.inner_ipv4.src_addr": match}, Action: "MyIngress.log"}
	}
	prefixEntry := func(match FieldMatch) Entry {
		return Entry{Table: "MyIngress.port_prefix", Match: map[string]FieldMatch{"meta.prefix": match}, Action: "MyIngress.log"}
	}
	valid := map[string]Entry{
		"prefix 0":       lpmEntry(LPM([]byte{10, 0, 0, 0}, 0)),
		"full prefix":    lpmEntry(LPM([]byte{10, 0, 0, 1}, 32)),
		"empty LPM only": {Table: "MyIngress.ipv4_lpm", Action: "MyIngress.log"},
		"exact IP":       exactEntry(ExactIP(net.ParseIP("10.0.0.1"))),
		"12-bit LPM":     prefixEntry(LPM([]byte{0x0f, 0xff}, 12)),
	}
	for name, entry := range valid {
		if _, err := client.p4rtTableEntry(entry); err != nil {
			t.Errorf("entry with %s rejected: %v", name, err)
		}
	}
	invalid := map[string]Entry{
		"prefix too long":              lpmEntry(LPM([]byte{10, 0, 0, 0}, 33)),
		"negative prefix":              lpmEntry(LPM([]byte{10, 0, 0, 0}, -1)),
		"value too wide":               lpmEntry(LPM([]byte{1, 10, 0, 0, 0}, 8)),
		"invalid exact IP":             exactEntry(ExactIP(nil)),
		"nil match":                    exactEntry(nil),
		"pointer LPM":                  lpmEntry(&LPMMatch{Value: []byte{10, 0, 0, 0}, PrefixLength: 8}),
		"nil pointer LPM":              lpmEntry((*LPMMatch)(nil)),
		"pointer exact":                exactEntry(&ExactMatch{Value: []byte{10}}),
		"LPM on exact field":           exactEntry(LPM([]byte{10, 0, 0, 1}, 8)),
		"LPM 0 on exact field":         exactEntry(LPM([]byte{10, 0, 0, 1}, 0)),
		"exact on LPM field":           lpmEntry(Exact([]byte{10, 0, 0, 1})),
		"value over 12 bits":           prefixEntry(LPM([]byte{0x1f, 0xff}, 12)),
		"masked over 12 bits":          prefixEntry(LPM([]byte{0xff, 0xf0}, 4)),
		"only prefix 0 on mixed table": prefixEntry(LPM([]byte{0x0f, 0xff}, 0)),
		"no match on exact table":      {Table: "MyIngress.log_table", Action: "MyIngress.log"},
		"no match on keyless table":    {Table: "MyIngress.keyless", Action: "MyIngress.log"},
	}
	for name, entry := range invalid {
		if _, err := client.p4rtTableEntry(entry); err == nil {
			t.Errorf("entry with %s should be rejected", name)
		}
	}
}

func TestDefaultRouteRoundTrip(t *testing.T) {
	server := &fakeP4Runtime{}
	client := connectFake(t, server)
	ctx := context.Background()
	defaultRoute := Entry{
		Table:  "MyIngress.ipv4_lpm",
		Match:  map[string]FieldMatch{"hdr.ipv4.dst_addr": LPM([]byte{10, 0, 0, 0}, 0)},
		Action: "MyIngress.log",
	}
	if err := client.Insert(ctx, defaultRoute); err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	server.mutex.Lock()
	written := server.tableEntries
	server.mutex.Unlock()
	if len(written) != 1 || len(written[0].GetMatch()) != 0 {
		t.Fatalf("prefix 0 field should be omitted from the proto, got %v", written)
	}
	entries, err := client.ReadTable(ctx, "MyIngress.ipv4_lpm")
	if err != nil || len(entries) != 1 || len(entries[0].Match) != 0 {
		t.Fatalf("expected one entry with empty match, got %v (err %v)", entries, err)
	}
	result, err := client.SyncTable(ctx, "MyIngress.ipv4_lpm", []Entry{defaultRoute})
	if err != nil || result != (SyncResult{}) {
		t.Errorf("sync of an unchanged default route should be a no-op, got %+v (err %v)", result, err)
	}
}

func TestActionParamIDsComeFromP4Info(t *testing.T) {
	client := &Client{p4Info: testP4Info()}
	tableEntry, err := client.p4rtTableEntry(Entry{
		Table:  "MyIngress.log_table",
		Match:  map[string]FieldMatch{"hdr.inner_ipv4.src_addr": Exact([]byte{10})},
		Action: "MyIngress.set_tunnel",
		Params: [][]byte{{1}, {2}},
	})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	params := tableEntry.GetAction().GetAction().GetParams()
	if len(params) != 2 || params[0].GetParamId() != 4 || params[1].GetParamId() != 9 {
		t.Errorf("param ids should come from P4Info, got %v", params)
	}
}

func TestExactIPLength(t *testing.T) {
	for address, length := range map[string]int{"10.0.0.1": 4, "2001:db8::1": 16} {
		match, ok := ExactIP(net.ParseIP(address)).(ExactMatch)
		if !ok || len(match.Value) != length {
			t.Errorf("ExactIP(%s) should be %d bytes, got %v", address, length, match.Value)
		}
	}
}
