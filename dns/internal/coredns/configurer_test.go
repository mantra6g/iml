package coredns

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	configMapKey = types.NamespacedName{Namespace: "kube-system", Name: "coredns"}
	serviceKey   = types.NamespacedName{Namespace: "loom-system", Name: "loom-dns"}
)

func newConfigurer(t *testing.T, objects ...client.Object) *Configurer {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return &Configurer{
		Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build(),
		ConfigMap: configMapKey,
		Service:   serviceKey,
		Zone:      "loom.local.",
		TTL:       5,
		Log:       logr.Discard(),
	}
}

func newService(clusterIP string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: serviceKey.Namespace, Name: serviceKey.Name},
		Spec:       corev1.ServiceSpec{ClusterIP: clusterIP},
	}
}

func newConfigMap(data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: configMapKey.Namespace, Name: configMapKey.Name},
		Data:       data,
	}
}

func getCorefile(t *testing.T, c *Configurer) (string, string) {
	t.Helper()
	configMap := &corev1.ConfigMap{}
	if err := c.Client.Get(context.Background(), configMapKey, configMap); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return configMap.Data[corefileKey], configMap.ResourceVersion
}

func TestEnsure(t *testing.T) {
	c := newConfigurer(t, newService("10.96.0.53"), newConfigMap(map[string]string{corefileKey: kubeadmCorefile}))

	if err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	corefile, version := getCorefile(t, c)
	if want := kubeadmCorefile + Block("loom.local", "10.96.0.53", 5); corefile != want {
		t.Errorf("Corefile =\n%s\nwant\n%s", corefile, want)
	}

	if err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, again := getCorefile(t, c); again != version {
		t.Errorf("up-to-date Corefile was updated: resourceVersion %s -> %s", version, again)
	}
}

func TestEnsureFollowsClusterIP(t *testing.T) {
	stale := kubeadmCorefile + Block("loom.local", "10.96.0.99", 5)
	c := newConfigurer(t, newService("10.96.0.53"), newConfigMap(map[string]string{corefileKey: stale}))

	if err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if corefile, _ := getCorefile(t, c); corefile != kubeadmCorefile+Block("loom.local", "10.96.0.53", 5) {
		t.Errorf("stale block was not replaced:\n%s", corefile)
	}
}

func TestEnsureErrors(t *testing.T) {
	stock := newConfigMap(map[string]string{corefileKey: kubeadmCorefile})
	tests := []struct {
		name    string
		objects []client.Object
	}{
		{"missing Service", []client.Object{stock}},
		{"headless Service", []client.Object{newService(corev1.ClusterIPNone), stock}},
		{"Service without cluster IP", []client.Object{newService(""), stock}},
		{"missing ConfigMap", []client.Object{newService("10.96.0.53")}},
		{"ConfigMap without Corefile", []client.Object{newService("10.96.0.53"), newConfigMap(nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := newConfigurer(t, tt.objects...).Ensure(context.Background()); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
