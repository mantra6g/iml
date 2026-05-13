package nfcfg

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bmv2-driver/api"
	"bmv2-driver/pkg/p4rutils"
	p4switch "bmv2-driver/pkg/p4switch"

	"github.com/go-logr/logr"
	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerConfig struct {
	Switch *p4switch.SwitchClient
	Log    logr.Logger
}

type Manager interface {
	EnsurePresentConfigForNF(ctx context.Context, nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	EnsureAbsentConfig(ctx context.Context, nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	GetAllNetworkFunctionsUsingConfig(nfCfgID client.ObjectKey) ([]client.ObjectKey, error)
}

// appliedState tracks the table entries that were written to the switch for one NF.
type appliedState struct {
	resourceVersion string
	entries         []*p4v1.TableEntry
}

type RealManager struct {
	switchClient *p4switch.SwitchClient
	log          logr.Logger

	mu          sync.RWMutex
	configToNFs map[client.ObjectKey]map[client.ObjectKey]struct{} // configKey -> set of nfKeys
	nfApplied   map[client.ObjectKey]*appliedState                 // nfKey -> applied state
}

var _ Manager = &RealManager{}

func NewManager(cfg ManagerConfig) (Manager, error) {
	if cfg.Switch == nil {
		return nil, fmt.Errorf("switch client is required")
	}
	return &RealManager{
		switchClient: cfg.Switch,
		configToNFs:  make(map[client.ObjectKey]map[client.ObjectKey]struct{}),
		nfApplied:    make(map[client.ObjectKey]*appliedState),
		log:          cfg.Log,
	}, nil
}

// EnsurePresentConfigForNF applies the table entries from nfConfig to the switch for the given NF.
// If nfConfig is nil the call is a no-op. If the program is not yet loaded the call returns nil
// and the reconciler will retry on the next cycle.
func (m *RealManager) EnsurePresentConfigForNF(ctx context.Context, nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error {
	nfKey := client.ObjectKeyFromObject(nf)

	if nfConfig == nil {
		m.mu.Lock()
		delete(m.nfApplied, nfKey)
		m.mu.Unlock()
		return nil
	}

	configKey := client.ObjectKeyFromObject(nfConfig)

	m.mu.RLock()
	existing := m.nfApplied[nfKey]
	m.mu.RUnlock()

	if existing != nil && existing.resourceVersion == nfConfig.ResourceVersion {
		return nil // already up to date
	}

	tables, err := m.fetchTableMetadata(ctx)
	if err != nil {
		return fmt.Errorf("fetching P4Info: %w", err)
	}
	if tables == nil {
		return nil // no program loaded yet; reconciler will retry
	}

	entries, err := p4rutils.BuildTableEntries(nfConfig.Spec.Tables, tables)
	if err != nil {
		return fmt.Errorf("building table entries: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = m.switchClient.SetAllTableEntries(writeCtx, entries)
	cancel()
	if err != nil {
		return fmt.Errorf("setting table entries: %w", err)
	}

	m.mu.Lock()
	m.nfApplied[nfKey] = &appliedState{
		resourceVersion: nfConfig.ResourceVersion,
		entries:         entries,
	}
	if m.configToNFs[configKey] == nil {
		m.configToNFs[configKey] = make(map[client.ObjectKey]struct{})
	}
	m.configToNFs[configKey][nfKey] = struct{}{}
	m.mu.Unlock()

	return nil
}

// EnsureAbsentConfig removes the table entries that were applied for the given NF.
func (m *RealManager) EnsureAbsentConfig(ctx context.Context, nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error {
	nfKey := client.ObjectKeyFromObject(nf)

	m.mu.RLock()
	existing := m.nfApplied[nfKey]
	m.mu.RUnlock()

	if existing == nil || len(existing.entries) == 0 {
		return nil
	}

	tables, err := m.fetchTableMetadata(ctx)
	if err != nil || tables == nil {
		// Program may have been unloaded; clean up tracking regardless.
		m.removeTracking(nfKey, nfConfig)
		return nil
	}

	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = m.switchClient.EditTableEntries(writeCtx, existing.entries, p4v1.Update_DELETE)
	cancel()
	if err != nil {
		return fmt.Errorf("deleting table entries: %w", err)
	}

	m.removeTracking(nfKey, nfConfig)
	return nil
}

// GetAllNetworkFunctionsUsingConfig returns all NF keys that reference the given config.
func (m *RealManager) GetAllNetworkFunctionsUsingConfig(nfCfgID client.ObjectKey) ([]client.ObjectKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nfs := m.configToNFs[nfCfgID]
	result := make([]client.ObjectKey, 0, len(nfs))
	for key := range nfs {
		result = append(result, key)
	}
	return result, nil
}

func (m *RealManager) fetchTableMetadata(ctx context.Context) ([]api.TableMetadata, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := m.switchClient.P4Client.GetForwardingPipelineConfig(reqCtx, &p4v1.GetForwardingPipelineConfigRequest{
		ResponseType: p4v1.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	})
	if err != nil {
		return nil, err
	}
	if resp.Config == nil || resp.Config.P4Info == nil || len(resp.Config.P4Info.Tables) == 0 {
		return nil, nil
	}
	return api.GetTableMetadata(&api.P4Program{P4Info: resp.Config.P4Info}), nil
}

func (m *RealManager) removeTracking(nfKey client.ObjectKey, nfConfig *corev1alpha1.NetworkFunctionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nfApplied, nfKey)
	if nfConfig != nil {
		configKey := client.ObjectKeyFromObject(nfConfig)
		if nfs := m.configToNFs[configKey]; nfs != nil {
			delete(nfs, nfKey)
			if len(nfs) == 0 {
				delete(m.configToNFs, configKey)
			}
		}
	}
}
