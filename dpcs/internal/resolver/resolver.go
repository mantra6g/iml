package resolver

import (
	"context"
	"fmt"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resolver struct {
	reader   client.Reader
	grpcPort uint16
}

func New(reader client.Reader, grpcPort uint16) *Resolver {
	return &Resolver{reader: reader, grpcPort: grpcPort}
}

// TODO: only the first driverIP is used; handle multiple IPs.
func (r *Resolver) Resolve(ctx context.Context, p4targetName string) (string, error) {
	p4target := &corev1alpha1.P4Target{}
	if err := r.reader.Get(ctx, types.NamespacedName{Name: p4targetName}, p4target); err != nil {
		return "", fmt.Errorf("failed to get P4Target %q: %w", p4targetName, err)
	}
	if len(p4target.Status.DriverIPs) == 0 {
		return "", fmt.Errorf("P4Target %q has no driverIPs in status yet", p4targetName)
	}
	return fmt.Sprintf("%s:%d", p4target.Status.DriverIPs[0], r.grpcPort), nil
}
