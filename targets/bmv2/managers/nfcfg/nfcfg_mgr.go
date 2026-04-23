package nfcfg

import (
	"fmt"

	corev1alpha1 "bmv2-driver/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager interface {
	EnsurePresentConfigForNF(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	EnsureAbsentConfig(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	GetAllNetworkFunctionsUsingConfig(nfCfgID client.ObjectKey) ([]client.ObjectKey, error)
}

type RealManager struct {
}

var _ Manager = &RealManager{}

func NewManager() (Manager, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *RealManager) EnsurePresentConfigForNF(nfConfig *corev1alpha1.NetworkFunctionConfig,
	nf *corev1alpha1.NetworkFunction) error {
	// TODO: implement
	return fmt.Errorf("not implemented")
}

func (m *RealManager) EnsureAbsentConfig(nfConfig *corev1alpha1.NetworkFunctionConfig,
	nf *corev1alpha1.NetworkFunction) error {
	// TODO: implement
	return fmt.Errorf("not implemented")
}

func (m *RealManager) GetAllNetworkFunctionsUsingConfig(nfCfgID client.ObjectKey) ([]client.ObjectKey, error) {
	// TODO: implement
	return nil, fmt.Errorf("not implemented")
}
