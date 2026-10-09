package coredns

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// corefileKey is the ConfigMap key that holds the Corefile.
const corefileKey = "Corefile"

// Configurer keeps the CoreDNS Corefile forwarding Zone to the cluster IP of Service.
type Configurer struct {
	Client client.Client
	// ConfigMap is the CoreDNS ConfigMap, usually kube-system/coredns.
	ConfigMap types.NamespacedName
	// Service is the Kubernetes Service in front of the loom DNS server.
	Service types.NamespacedName
	Zone    string
	TTL     uint32
	Log     logr.Logger
}

// Run calls Ensure immediately and then every period until ctx is done. Errors are logged and
// retried on the next period.
func (c *Configurer) Run(ctx context.Context, period time.Duration) {
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		if err := c.Ensure(ctx); err != nil {
			c.Log.Error(err, "failed to configure CoreDNS", "configMap", c.ConfigMap, "service", c.Service)
		}
	}, period)
}

// Ensure updates the CoreDNS ConfigMap so that its Corefile forwards Zone to the Service's
// cluster IP. It does nothing if the Corefile is already up to date.
func (c *Configurer) Ensure(ctx context.Context) error {
	service := &corev1.Service{}
	if err := c.Client.Get(ctx, c.Service, service); err != nil {
		return fmt.Errorf("get Service: %w", err)
	}
	ip := service.Spec.ClusterIP
	if ip == "" || ip == corev1.ClusterIPNone {
		return fmt.Errorf("service %s has no cluster IP", c.Service)
	}
	block := Block(c.Zone, ip, c.TTL)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		configMap := &corev1.ConfigMap{}
		if err := c.Client.Get(ctx, c.ConfigMap, configMap); err != nil {
			return fmt.Errorf("get ConfigMap: %w", err)
		}
		corefile, ok := configMap.Data[corefileKey]
		if !ok {
			return fmt.Errorf("configMap %s has no %q key", c.ConfigMap, corefileKey)
		}
		updated, changed, err := Upsert(corefile, block)
		if err != nil {
			return err
		}
		if !changed {
			c.Log.V(1).Info("CoreDNS already forwards zone", "zone", c.Zone, "ip", ip)
			return nil
		}
		configMap.Data[corefileKey] = updated
		if err := c.Client.Update(ctx, configMap); err != nil {
			return err
		}
		c.Log.Info("Configured CoreDNS to forward zone", "zone", c.Zone, "ip", ip, "configMap", c.ConfigMap)
		return nil
	})
}
