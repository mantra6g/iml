package env

import (
	"context"
	"testing"
	"time"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNodeName = "node-1"

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := infrav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func setCIDRWaitTimeout(t *testing.T, timeout time.Duration) {
	t.Helper()
	old := CIDRWaitTimeout
	CIDRWaitTimeout = timeout
	t.Cleanup(func() { CIDRWaitTimeout = old })
}

func TestEnsureLoomNodeCIDRs_AlreadyAssigned(t *testing.T) {
	k8sClient := newFakeClient(t, &infrav1alpha1.LoomNode{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName},
		Spec: infrav1alpha1.LoomNodeSpec{
			NodeCIDRs:   []string{"fd00:1::/64"},
			TunnelCIDRs: []string{"fd00:2::/64"},
		},
	})

	loomNode, err := ensureLoomNodeCIDRs(context.Background(), logr.Discard(), k8sClient, testNodeName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loomNode.Spec.NodeCIDRs[0] != "fd00:1::/64" || loomNode.Spec.TunnelCIDRs[0] != "fd00:2::/64" {
		t.Errorf("unexpected CIDRs: %v %v", loomNode.Spec.NodeCIDRs, loomNode.Spec.TunnelCIDRs)
	}
}

// The LoomNode does not exist yet and the operator assigns its CIDRs only after a while,
// as happens on a fresh install. The returned LoomNode must carry the assigned CIDRs.
func TestEnsureLoomNodeCIDRs_CreatesAndWaitsForOperator(t *testing.T) {
	k8sClient := newFakeClient(t)

	go func() {
		ctx := context.Background()
		loomNode := &infrav1alpha1.LoomNode{}
		for {
			time.Sleep(500 * time.Millisecond)
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: testNodeName}, loomNode); err == nil {
				break
			}
		}
		time.Sleep(3 * time.Second)
		loomNode.Spec.NodeCIDRs = []string{"fd00:1::/64"}
		loomNode.Spec.TunnelCIDRs = []string{"fd00:2::/64"}
		if err := k8sClient.Update(ctx, loomNode); err != nil {
			t.Errorf("failed to assign CIDRs: %v", err)
		}
	}()

	loomNode, err := ensureLoomNodeCIDRs(context.Background(), logr.Discard(), k8sClient, testNodeName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCIDRs(loomNode) {
		t.Errorf("returned LoomNode has no CIDRs: %+v", loomNode.Spec)
	}
}

func TestEnsureLoomNodeCIDRs_TimesOut(t *testing.T) {
	setCIDRWaitTimeout(t, 3*time.Second)
	k8sClient := newFakeClient(t, &infrav1alpha1.LoomNode{
		ObjectMeta: metav1.ObjectMeta{Name: testNodeName},
	})

	if _, err := ensureLoomNodeCIDRs(context.Background(), logr.Discard(), k8sClient, testNodeName); err == nil {
		t.Fatal("expected a timeout error")
	}
}
