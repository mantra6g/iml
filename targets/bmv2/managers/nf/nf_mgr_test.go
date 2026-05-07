package nf

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	p4targetpkg "bmv2-driver/managers/p4target"
	p4switch "bmv2-driver/pkg/p4switch"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/client"
)

// ---------------------------------------------------------------------------
// Mock P4Runtime client
// ---------------------------------------------------------------------------

type mockP4Client struct {
	mu sync.Mutex

	getFwdPipelineFn func(*p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error)
	setFwdPipelineFn func(*p4v1.SetForwardingPipelineConfigRequest) (*p4v1.SetForwardingPipelineConfigResponse, error)

	setCalls []*p4v1.SetForwardingPipelineConfigRequest
}

func (m *mockP4Client) GetForwardingPipelineConfig(_ context.Context, in *p4v1.GetForwardingPipelineConfigRequest, _ ...grpc.CallOption) (*p4v1.GetForwardingPipelineConfigResponse, error) {
	if m.getFwdPipelineFn != nil {
		return m.getFwdPipelineFn(in)
	}
	return &p4v1.GetForwardingPipelineConfigResponse{}, nil
}

func (m *mockP4Client) SetForwardingPipelineConfig(_ context.Context, in *p4v1.SetForwardingPipelineConfigRequest, _ ...grpc.CallOption) (*p4v1.SetForwardingPipelineConfigResponse, error) {
	m.mu.Lock()
	m.setCalls = append(m.setCalls, in)
	m.mu.Unlock()
	if m.setFwdPipelineFn != nil {
		return m.setFwdPipelineFn(in)
	}
	return &p4v1.SetForwardingPipelineConfigResponse{}, nil
}

func (m *mockP4Client) Read(_ context.Context, _ *p4v1.ReadRequest, _ ...grpc.CallOption) (p4v1.P4Runtime_ReadClient, error) {
	return &mockReadStream{}, nil
}
func (m *mockP4Client) Write(_ context.Context, _ *p4v1.WriteRequest, _ ...grpc.CallOption) (*p4v1.WriteResponse, error) {
	return nil, nil
}
func (m *mockP4Client) StreamChannel(_ context.Context, _ ...grpc.CallOption) (p4v1.P4Runtime_StreamChannelClient, error) {
	return nil, nil
}
func (m *mockP4Client) Capabilities(_ context.Context, _ *p4v1.CapabilitiesRequest, _ ...grpc.CallOption) (*p4v1.CapabilitiesResponse, error) {
	return nil, nil
}

func (m *mockP4Client) setCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.setCalls)
}

type mockReadStream struct{}

func (m *mockReadStream) Recv() (*p4v1.ReadResponse, error) { return nil, io.EOF }
func (m *mockReadStream) Header() (metadata.MD, error)      { return nil, nil }
func (m *mockReadStream) Trailer() metadata.MD              { return nil }
func (m *mockReadStream) CloseSend() error                  { return nil }
func (m *mockReadStream) Context() context.Context          { return context.Background() }
func (m *mockReadStream) SendMsg(_ any) error               { return nil }
func (m *mockReadStream) RecvMsg(_ any) error               { return nil }

// ---------------------------------------------------------------------------
// Mock P4Target manager
// ---------------------------------------------------------------------------

type mockP4TargetManager struct{}

func (m *mockP4TargetManager) GetName() string { return "test-target" }
func (m *mockP4TargetManager) GetAllocatable() corev1.ResourceList {
	return corev1.ResourceList{
		p4targetpkg.ResourceNFSlots:      resource.MustParse("10"),
		p4targetpkg.ResourceTableEntries: resource.MustParse("1000"),
	}
}
func (m *mockP4TargetManager) GetCapacity() corev1.ResourceList { return nil }
func (m *mockP4TargetManager) GetHealthyCondition() corev1alpha1.P4TargetCondition {
	return corev1alpha1.P4TargetCondition{}
}
func (m *mockP4TargetManager) GetReadyCondition() corev1alpha1.P4TargetCondition {
	return corev1alpha1.P4TargetCondition{}
}
func (m *mockP4TargetManager) GetNetworkConfiguredCondition() corev1alpha1.P4TargetCondition {
	return corev1alpha1.P4TargetCondition{}
}
func (m *mockP4TargetManager) GetOccupiedCondition() corev1alpha1.P4TargetCondition {
	return corev1alpha1.P4TargetCondition{}
}
func (m *mockP4TargetManager) GetTargetIPs() []net.IP { return nil }
func (m *mockP4TargetManager) GetDriverIP() net.IP    { return nil }
func (m *mockP4TargetManager) EnsureNetworkConfiguration(_ p4targetpkg.NetConfig) error {
	return nil
}
func (m *mockP4TargetManager) AllocateNetworkFunctionIP() (net.IP, error) {
	return net.ParseIP("10.0.0.1"), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var testP4Bytes = []byte(`{"test": "program"}`)

func newTestNF(name, namespace string) *corev1alpha1.NetworkFunction {
	return &corev1alpha1.NetworkFunction{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1alpha1.NetworkFunctionSpec{
			P4File:     base64.StdEncoding.EncodeToString(testP4Bytes),
			TargetName: "test-target",
		},
	}
}

func newTestManagerWith(p4c p4v1.P4RuntimeClient, tgtMgr p4targetpkg.Manager) Manager {
	sc := p4switch.NewSwitchClient(p4c, 0, 1, 0)
	m, err := NewManager(ManagerConfig{Switch: sc, P4TargetManager: tgtMgr})
	if err != nil {
		panic(err)
	}
	return m
}

func waitDone(t *testing.T, h DeploymentHandle) {
	t.Helper()
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("deployment handle did not reach terminal state within 5s (phase=%s)", h.Status().Phase)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestEnsurePresent_DrivesToPhaseReady verifies that applying a NetworkFunction
// drives the NF manager through compile/preCheck/deploy and reaches PhaseReady.
// func TestEnsurePresent_DrivesToPhaseReady(t *testing.T) {
// 	mock := &mockP4Client{}
// 	mgr := newTestManagerWith(mock, &mockP4TargetManager{})
// 	nf := newTestNF("test-nf", "test-ns")

// 	handle := mgr.EnsurePresent(context.Background(), nf, net.ParseIP("10.100.0.2"))
// 	waitDone(t, handle)

// 	if handle.Status().Phase != PhaseReady {
// 		t.Fatalf("expected PhaseReady, got %s: %v", handle.Status().Phase, handle.Err())
// 	}
// 	if handle.Err() != nil {
// 		t.Fatalf("unexpected error: %v", handle.Err())
// 	}
// 	if mock.setCallCount() != 1 {
// 		t.Fatalf("expected 1 SetForwardingPipelineConfig call, got %d", mock.setCallCount())
// 	}
// }

// TestEnsureAbsent_ResetsForwardingPipeline verifies that deleting a NetworkFunction
// causes deleteResources to call ResetPipeline (SetForwardingPipelineConfig with empty config).
// func TestEnsureAbsent_ResetsForwardingPipeline(t *testing.T) {
// 	mock := &mockP4Client{}
// 	mgr := newTestManagerWith(mock, &mockP4TargetManager{})
// 	nf := newTestNF("test-nf", "test-ns")

// 	// Deploy first.
// 	deployHandle := mgr.EnsurePresent(context.Background(), nf, net.ParseIP("10.100.0.2"))
// 	waitDone(t, deployHandle)
// 	if deployHandle.Status().Phase != PhaseReady {
// 		t.Fatalf("setup: expected PhaseReady, got %s", deployHandle.Status().Phase)
// 	}

// 	// Now delete.
// 	deleteHandle := mgr.EnsureAbsent(context.Background(), nf)
// 	waitDone(t, deleteHandle)

// 	if deleteHandle.Status().Phase != PhaseDeleted {
// 		t.Fatalf("expected PhaseDeleted, got %s: %v", deleteHandle.Status().Phase, deleteHandle.Err())
// 	}

// 	// Verify that a reset call (empty P4DeviceConfig) was made.
// 	mock.mu.Lock()
// 	calls := mock.setCalls
// 	mock.mu.Unlock()
// 	resetFound := false
// 	for _, req := range calls {
// 		if req.Config != nil && len(req.Config.P4DeviceConfig) == 0 {
// 			resetFound = true
// 			break
// 		}
// 	}
// 	if !resetFound {
// 		t.Fatal("expected a SetForwardingPipelineConfig call with empty P4DeviceConfig (ResetPipeline), none found")
// 	}
// }

// TestGetDeployedNetworkFunctions_AfterRestart verifies that after a driver restart,
// when the switch already has a program loaded, EnsurePresent detects the existing
// state and fast-paths to PhaseReady without re-deploying. GetDeployedNetworkFunctions
// then correctly reflects the switch state.
// func TestGetDeployedNetworkFunctions_AfterRestart(t *testing.T) {
// 	// Simulate the switch already having the program loaded.
// 	mock := &mockP4Client{
// 		getFwdPipelineFn: func(_ *p4v1.GetForwardingPipelineConfigRequest) (*p4v1.GetForwardingPipelineConfigResponse, error) {
// 			return &p4v1.GetForwardingPipelineConfigResponse{
// 				Config: &p4v1.ForwardingPipelineConfig{
// 					P4DeviceConfig: testP4Bytes,
// 				},
// 			}, nil
// 		},
// 	}

// 	// Fresh manager — simulates driver restart (empty ops/programs maps).
// 	mgr := newTestManagerWith(mock, &mockP4TargetManager{})
// 	nf := newTestNF("test-nf", "test-ns")

// 	handle := mgr.EnsurePresent(context.Background(), nf, net.ParseIP("10.100.0.2"))
// 	waitDone(t, handle)

// 	if handle.Status().Phase != PhaseReady {
// 		t.Fatalf("expected PhaseReady after restart, got %s: %v", handle.Status().Phase, handle.Err())
// 	}
// 	if mock.setCallCount() != 0 {
// 		t.Fatalf("expected no SetForwardingPipelineConfig calls (program already loaded), got %d", mock.setCallCount())
// 	}

// 	deployed, err := mgr.GetDeployedNetworkFunctions(context.Background())
// 	if err != nil {
// 		t.Fatalf("GetDeployedNetworkFunctions: %v", err)
// 	}
// 	want := []client.ObjectKey{{Name: "test-nf", Namespace: "test-ns"}}
// 	if len(deployed) != 1 || deployed[0] != want[0] {
// 		t.Fatalf("expected %v, got %v", want, deployed)
// 	}
// }
