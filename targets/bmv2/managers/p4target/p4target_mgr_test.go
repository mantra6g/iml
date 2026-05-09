package p4target

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"testing"

	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockP4Client struct {
	getFwdPipelineFn func(*p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error)
	readFn           func(*p4v1.ReadRequest) (p4v1.P4Runtime_ReadClient, error)
}

func (m *mockP4Client) GetForwardingPipelineConfig(_ context.Context, in *p4v1.GetForwardingPipelineConfigRequest, _ ...grpc.CallOption) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	if m.getFwdPipelineFn != nil {
		return m.getFwdPipelineFn(in)
	}
	return &p4v1.GetForwardingPipelineConfigResponse{}, nil
}

func (m *mockP4Client) Read(_ context.Context, in *p4v1.ReadRequest, _ ...grpc.CallOption) (p4v1.P4Runtime_ReadClient, error) {
	if m.readFn != nil {
		return m.readFn(in)
	}
	return &mockReadStream{}, nil
}

func (m *mockP4Client) Write(_ context.Context, _ *p4v1.WriteRequest, _ ...grpc.CallOption) (*p4v1.WriteResponse, error) {
	return nil, nil
}
func (m *mockP4Client) SetForwardingPipelineConfig(_ context.Context, _ *p4v1.SetForwardingPipelineConfigRequest, _ ...grpc.CallOption) (*p4v1.SetForwardingPipelineConfigResponse, error) {
	return nil, nil
}
func (m *mockP4Client) StreamChannel(_ context.Context, _ ...grpc.CallOption) (p4v1.P4Runtime_StreamChannelClient, error) {
	return nil, nil
}
func (m *mockP4Client) Capabilities(_ context.Context, _ *p4v1.CapabilitiesRequest, _ ...grpc.CallOption) (*p4v1.CapabilitiesResponse, error) {
	return nil, nil
}

type mockReadStream struct {
	responses []*p4v1.ReadResponse
	index     int
}

func (m *mockReadStream) Recv() (*p4v1.ReadResponse, error) {
	if m.index >= len(m.responses) {
		return nil, io.EOF
	}
	r := m.responses[m.index]
	m.index++
	return r, nil
}

func (m *mockReadStream) Header() (metadata.MD, error) { return nil, nil }
func (m *mockReadStream) Trailer() metadata.MD         { return nil }
func (m *mockReadStream) CloseSend() error             { return nil }
func (m *mockReadStream) Context() context.Context     { return context.Background() }
func (m *mockReadStream) SendMsg(_ any) error          { return nil }
func (m *mockReadStream) RecvMsg(_ any) error          { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestManager(client p4v1.P4RuntimeClient, maxSlots int) *RealManager {
	return &RealManager{
		name:         "test-target",
		p4client:     client,
		maxNFSlots:   maxSlots,
		allocatedIPs: make(map[netip.Addr]struct{}),
	}
}

func p4infoWithTables(sizes ...int64) *p4configv1.P4Info {
	tables := make([]*p4configv1.Table, len(sizes))
	for i, s := range sizes {
		tables[i] = &p4configv1.Table{
			Preamble: &p4configv1.Preamble{Id: uint32(i + 1), Name: "table"},
			Size:     s,
		}
	}
	return &p4configv1.P4Info{Tables: tables}
}

func readStreamWithEntries(n int) *mockReadStream {
	entities := make([]*p4v1.Entity, n)
	for i := range entities {
		entities[i] = &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: &p4v1.TableEntry{}}}
	}
	return &mockReadStream{
		responses: []*p4v1.ReadResponse{{Entities: entities}},
	}
}

// ---------------------------------------------------------------------------
// EnsureNetworkConfiguration
// ---------------------------------------------------------------------------

func TestEnsureNetworkConfiguration_ValidCIDR(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/24"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.cidr.IsValid() {
		t.Fatal("cidr should be valid after configuration")
	}
}

func TestEnsureNetworkConfiguration_InvalidCIDR(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "not-a-cidr"}); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestEnsureNetworkConfiguration_UnmaskedPrefix(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	// 10.0.0.5/24 should be normalised to 10.0.0.0/24
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.5/24"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.cidr.String() != "10.0.0.0/24" {
		t.Fatalf("expected masked CIDR 10.0.0.0/24, got %s", m.cidr)
	}
}

func TestEnsureNetworkConfiguration_Idempotent(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	cfg := NetConfig{TargetCIDR: "10.0.0.0/24"}
	if err := m.EnsureNetworkConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AllocateNetworkFunctionIP(); err != nil {
		t.Fatal(err)
	}
	// Second call with same CIDR must not reset the allocator.
	if err := m.EnsureNetworkConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	if len(m.allocatedIPs) != 1 {
		t.Fatal("idempotent call should not reset allocated IPs")
	}
}

func TestEnsureNetworkConfiguration_CIDRChange_ResetsAllocator(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AllocateNetworkFunctionIP(); err != nil {
		t.Fatal(err)
	}
	// Switch to a different CIDR — should clear previous allocations.
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
	if len(m.allocatedIPs) != 0 {
		t.Fatal("changing CIDR should reset allocated IPs")
	}
	if m.cidr.String() != "192.168.1.0/24" {
		t.Fatalf("expected new CIDR, got %s", m.cidr)
	}
}

// ---------------------------------------------------------------------------
// AllocateNetworkFunctionIP
// ---------------------------------------------------------------------------

func TestAllocateNetworkFunctionIP_BeforeConfiguration(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if _, err := m.AllocateNetworkFunctionIP(); err == nil {
		t.Fatal("expected error when network not configured")
	}
}

// func TestAllocateNetworkFunctionIP_Sequential(t *testing.T) {
// 	m := newTestManager(&mockP4Client{}, 8)
// 	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/30"}); err != nil {
// 		t.Fatal(err)
// 	}
// 	ip1, err := m.AllocateNetworkFunctionIP()
// 	if err != nil {
// 		t.Fatalf("first allocation failed: %v", err)
// 	}
// 	ip2, err := m.AllocateNetworkFunctionIP()
// 	if err != nil {
// 		t.Fatalf("second allocation failed: %v", err)
// 	}
// 	if ip1.Equal(ip2) {
// 		t.Fatalf("expected different IPs, both got %v", ip1)
// 	}
// 	if !net.ParseIP("10.0.0.1").Equal(ip1) {
// 		t.Errorf("expected 10.0.0.1, got %v", ip1)
// 	}
// 	if !net.ParseIP("10.0.0.2").Equal(ip2) {
// 		t.Errorf("expected 10.0.0.2, got %v", ip2)
// 	}
// }

func TestAllocateNetworkFunctionIP_Exhaustion(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	// /30 has exactly 2 usable host addresses
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/30"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AllocateNetworkFunctionIP(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AllocateNetworkFunctionIP(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.AllocateNetworkFunctionIP(); err == nil {
		t.Fatal("expected error on exhausted pool")
	}
}

func TestAllocateNetworkFunctionIP_TracksCount(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if _, err := m.AllocateNetworkFunctionIP(); err != nil {
			t.Fatalf("allocation %d failed: %v", i+1, err)
		}
	}
	if len(m.allocatedIPs) != 3 {
		t.Fatalf("expected 3 tracked IPs, got %d", len(m.allocatedIPs))
	}
}

// ---------------------------------------------------------------------------
// GetCapacity
// ---------------------------------------------------------------------------

func TestGetCapacity_NFSlots(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 5)
	slots, ok := m.GetCapacity()[ResourceNFSlots]
	if !ok {
		t.Fatal("expected loom.io/nf-slots in capacity")
	}
	if slots.Value() != 5 {
		t.Errorf("expected 5 slots, got %d", slots.Value())
	}
}

func TestGetCapacity_TableEntries_NoProgram(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return &p4v1.GetForwardingPipelineConfigResponse{Config: &p4v1.ForwardingPipelineConfig{}}, nil
		},
	}
	m := newTestManager(client, 8)
	if _, ok := m.GetCapacity()[ResourceTableEntries]; ok {
		t.Fatal("expected no table-entries key when no program is loaded")
	}
}

func TestGetCapacity_TableEntries_SumsAllTables(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return &p4v1.GetForwardingPipelineConfigResponse{
				Config: &p4v1.ForwardingPipelineConfig{P4Info: p4infoWithTables(512, 1024)},
			}, nil
		},
	}
	m := newTestManager(client, 8)
	entries, ok := m.GetCapacity()[ResourceTableEntries]
	if !ok {
		t.Fatal("expected loom.io/table-entries in capacity")
	}
	if entries.Value() != 1536 {
		t.Errorf("expected 1536 (512+1024), got %d", entries.Value())
	}
}

func TestGetCapacity_SwitchUnreachable_NFSlotsStillPresent(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	m := newTestManager(client, 8)
	cap := m.GetCapacity()
	if _, ok := cap[ResourceNFSlots]; !ok {
		t.Fatal("nf-slots must be present even when switch is unreachable")
	}
	if _, ok := cap[ResourceTableEntries]; ok {
		t.Fatal("table-entries should be absent when switch is unreachable")
	}
}

// ---------------------------------------------------------------------------
// GetAllocatable
// ---------------------------------------------------------------------------

func TestGetAllocatable_NFSlots_DecreasesWithAllocations(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 4)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}

	q := m.GetAllocatable()[ResourceNFSlots]
	if got := q.Value(); got != 4 {
		t.Errorf("expected 4 free slots, got %d", got)
	}

	if _, err := m.AllocateNetworkFunctionIP(); err != nil {
		t.Fatal(err)
	}

	q = m.GetAllocatable()[ResourceNFSlots]
	if got := q.Value(); got != 3 {
		t.Errorf("expected 3 free slots after one allocation, got %d", got)
	}
}

func TestGetAllocatable_TableEntries(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return &p4v1.GetForwardingPipelineConfigResponse{
				Config: &p4v1.ForwardingPipelineConfig{P4Info: p4infoWithTables(1000)},
			}, nil
		},
		readFn: func(_ *p4v1.ReadRequest) (p4v1.P4Runtime_ReadClient, error) {
			return readStreamWithEntries(200), nil
		},
	}
	m := newTestManager(client, 8)
	free, ok := m.GetAllocatable()[ResourceTableEntries]
	if !ok {
		t.Fatal("expected table-entries in allocatable")
	}
	if free.Value() != 800 {
		t.Errorf("expected 800 free entries (1000-200), got %d", free.Value())
	}
}

func TestGetAllocatable_TableEntries_NeverNegative(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return &p4v1.GetForwardingPipelineConfigResponse{
				Config: &p4v1.ForwardingPipelineConfig{P4Info: p4infoWithTables(100)},
			}, nil
		},
		readFn: func(_ *p4v1.ReadRequest) (p4v1.P4Runtime_ReadClient, error) {
			return readStreamWithEntries(200), nil // more used than capacity
		},
	}
	m := newTestManager(client, 8)
	q := m.GetAllocatable()[ResourceTableEntries]
	if got := q.Value(); got < 0 {
		t.Errorf("allocatable table-entries must not be negative, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// GetHealthyCondition
// ---------------------------------------------------------------------------

func TestGetHealthyCondition_Reachable(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	cond := m.GetHealthyCondition()
	if cond.Type != ConditionHealthy {
		t.Errorf("expected type %q, got %q", ConditionHealthy, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected True, got %q", cond.Status)
	}
	if cond.Reason != "SwitchReachable" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
}

func TestGetHealthyCondition_Unreachable(t *testing.T) {
	client := &mockP4Client{
		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	m := newTestManager(client, 8)
	cond := m.GetHealthyCondition()
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected False, got %q", cond.Status)
	}
	if cond.Reason != "SwitchUnreachable" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
	if cond.Message == "" {
		t.Error("expected error message in condition")
	}
}

// ---------------------------------------------------------------------------
// GetReadyCondition
// ---------------------------------------------------------------------------

// func TestGetReadyCondition_Unreachable(t *testing.T) {
// 	client := &mockP4Client{
// 		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
// 			return nil, errors.New("dial failed")
// 		},
// 	}
// 	m := newTestManager(client, 8)
// 	cond := m.GetReadyCondition()
// 	if cond.Status != metav1.ConditionFalse {
// 		t.Errorf("expected False when unreachable, got %q", cond.Status)
// 	}
// 	if cond.Reason != "SwitchUnreachable" {
// 		t.Errorf("unexpected reason: %q", cond.Reason)
// 	}
// }

// func TestGetReadyCondition_NoProgramLoaded(t *testing.T) {
// 	client := &mockP4Client{
// 		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
// 			return &p4v1.GetForwardingPipelineConfigResponse{
// 				Config: &p4v1.ForwardingPipelineConfig{},
// 			}, nil
// 		},
// 	}
// 	m := newTestManager(client, 8)
// 	cond := m.GetReadyCondition()
// 	if cond.Status != metav1.ConditionFalse {
// 		t.Errorf("expected False when no program loaded, got %q", cond.Status)
// 	}
// 	if cond.Reason != "NoProgramLoaded" {
// 		t.Errorf("unexpected reason: %q", cond.Reason)
// 	}
// }

// func TestGetReadyCondition_ProgramLoaded(t *testing.T) {
// 	client := &mockP4Client{
// 		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
// 			return &p4v1.GetForwardingPipelineConfigResponse{
// 				Config: &p4v1.ForwardingPipelineConfig{P4Info: p4infoWithTables(512)},
// 			}, nil
// 		},
// 	}
// 	m := newTestManager(client, 8)
// 	cond := m.GetReadyCondition()
// 	if cond.Status != metav1.ConditionTrue {
// 		t.Errorf("expected True when program loaded, got %q", cond.Status)
// 	}
// 	if cond.Reason != "ProgramLoaded" {
// 		t.Errorf("unexpected reason: %q", cond.Reason)
// 	}
// }

// ---------------------------------------------------------------------------
// GetNetworkConfiguredCondition
// ---------------------------------------------------------------------------

func TestGetNetworkConfiguredCondition_BeforeConfiguration(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	cond := m.GetNetworkConfiguredCondition()
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected False before configuration, got %q", cond.Status)
	}
	if cond.Reason != "NoCIDR" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
}

func TestGetNetworkConfiguredCondition_AfterConfiguration(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 8)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	cond := m.GetNetworkConfiguredCondition()
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected True after configuration, got %q", cond.Status)
	}
	if cond.Reason != "CIDRConfigured" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
	if cond.Message != "10.0.0.0/24" {
		t.Errorf("expected CIDR in message, got %q", cond.Message)
	}
}

// ---------------------------------------------------------------------------
// GetOccupiedCondition
// ---------------------------------------------------------------------------

func TestGetOccupiedCondition_SlotsAvailable(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 4)
	cond := m.GetOccupiedCondition()
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected False when slots available, got %q", cond.Status)
	}
	if cond.Reason != "SlotsAvailable" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
}

func TestGetOccupiedCondition_FullyOccupied(t *testing.T) {
	m := newTestManager(&mockP4Client{}, 2)
	if err := m.EnsureNetworkConfiguration(NetConfig{TargetCIDR: "10.0.0.0/29"}); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, err := m.AllocateNetworkFunctionIP(); err != nil {
			t.Fatalf("allocation %d failed: %v", i+1, err)
		}
	}
	cond := m.GetOccupiedCondition()
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected True when all slots occupied, got %q", cond.Status)
	}
	if cond.Reason != "NoSlotsAvailable" {
		t.Errorf("unexpected reason: %q", cond.Reason)
	}
}
