package ipassign

import (
	"fmt"
	"net"

	corev1alpha1 "iml-daemon/api/core/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager interface {
	AllocateAppInstance(app *corev1alpha1.Application) (*net.IPNet, error)
	ReleaseAppInstance(containerID string, app *corev1alpha1.Application) error
	AllocateP4Target(target *corev1alpha1.P4Target) (*net.IPNet, error)
	ReleaseP4Target(containerID string, target *corev1alpha1.P4Target) error
}

type RealManager struct {
	k8sClient     client.Client
	baseNetwork   *net.IPNet
	appAllocators map[types.UID]*IPv6Allocator
	tgtAllocators map[types.UID]*IPv6Allocator
}

func NewManager(k8sClient client.Client, baseNet net.IPNet) (Manager, error) {
	if k8sClient == nil {
		return nil, fmt.Errorf("k8sClient cannot be nil")
	}
	return &RealManager{
		k8sClient:     k8sClient,
		baseNetwork:   &baseNet,
		appAllocators: make(map[types.UID]*IPv6Allocator),
		tgtAllocators: make(map[types.UID]*IPv6Allocator),
	}, nil
}

func (r *RealManager) AllocateAppInstance(app *corev1alpha1.Application) (ip *net.IPNet, err error) {
	allocator, ok := r.appAllocators[app.UID]
	if !ok {
		allocator, err = NewIPv6Allocator(r.baseNetwork)
		if err != nil {
			return ip, fmt.Errorf("failed to allocate IPv6 allocator: %w", err)
		}
	}
	ip, err = allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}
	return ip, nil
}

func (r *RealManager) ReleaseAppInstance(containerID string, app *corev1alpha1.Application) error {
	// TODO: Implement actual release logic
	return nil
}

func (r *RealManager) AllocateP4Target(target *corev1alpha1.P4Target) (ip *net.IPNet, err error) {
	allocator, ok := r.tgtAllocators[target.UID]
	if !ok {
		allocator, err = NewIPv6Allocator(r.baseNetwork)
		if err != nil {
			return ip, fmt.Errorf("failed to allocate IPv6 allocator: %w", err)
		}
	}
	ip, err = allocator.Allocate()
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}
	return ip, nil
}

func (r *RealManager) ReleaseP4Target(containerID string, target *corev1alpha1.P4Target) error {
	// TODO: Implement actual release logic
	return nil
}
