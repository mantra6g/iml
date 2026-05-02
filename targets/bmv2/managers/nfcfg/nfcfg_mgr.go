package nfcfg

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bmv2-driver/api"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
	p4v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagerConfig struct {
	P4Client       p4v1.P4RuntimeClient
	DeviceID       uint64
	ElectionIDHigh uint64
	ElectionIDLow  uint64
}

type Manager interface {
	EnsurePresentConfigForNF(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	EnsureAbsentConfig(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error
	GetAllNetworkFunctionsUsingConfig(nfCfgID client.ObjectKey) ([]client.ObjectKey, error)
}

// appliedState tracks the table entries that were written to the switch for one NF.
type appliedState struct {
	resourceVersion string
	entries         []*p4v1.TableEntry
}

type RealManager struct {
	p4client       p4v1.P4RuntimeClient
	deviceID       uint64
	electionIDHigh uint64
	electionIDLow  uint64

	mu          sync.RWMutex
	configToNFs map[client.ObjectKey]map[client.ObjectKey]struct{} // configKey -> set of nfKeys
	nfApplied   map[client.ObjectKey]*appliedState                 // nfKey -> applied state
}

var _ Manager = &RealManager{}

func NewManager(cfg ManagerConfig) (Manager, error) {
	if cfg.P4Client == nil {
		return nil, fmt.Errorf("P4Runtime client is required")
	}
	return &RealManager{
		p4client:       cfg.P4Client,
		deviceID:       cfg.DeviceID,
		electionIDHigh: cfg.ElectionIDHigh,
		electionIDLow:  cfg.ElectionIDLow,
		configToNFs:    make(map[client.ObjectKey]map[client.ObjectKey]struct{}),
		nfApplied:      make(map[client.ObjectKey]*appliedState),
	}, nil
}

// EnsurePresentConfigForNF applies the table entries from nfConfig to the switch for the given NF.
// If nfConfig is nil the call is a no-op. If the program is not yet loaded the call returns nil
// and the reconciler will retry on the next cycle.
func (m *RealManager) EnsurePresentConfigForNF(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error {
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

	tables, err := m.fetchTableMetadata()
	if err != nil {
		return fmt.Errorf("fetching P4Info: %w", err)
	}
	if tables == nil {
		return nil // no program loaded yet; reconciler will retry
	}

	entries, err := buildTableEntries(nfConfig.Spec.Tables, tables)
	if err != nil {
		return fmt.Errorf("building table entries: %w", err)
	}

	if existing != nil && len(existing.entries) > 0 {
		if err := m.writeEntries(existing.entries, p4v1.Update_DELETE); err != nil {
			return fmt.Errorf("removing stale entries: %w", err)
		}
	}

	if len(entries) > 0 {
		if err := m.writeEntries(entries, p4v1.Update_INSERT); err != nil {
			return fmt.Errorf("inserting table entries: %w", err)
		}
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
func (m *RealManager) EnsureAbsentConfig(nfConfig *corev1alpha1.NetworkFunctionConfig, nf *corev1alpha1.NetworkFunction) error {
	nfKey := client.ObjectKeyFromObject(nf)

	m.mu.RLock()
	existing := m.nfApplied[nfKey]
	m.mu.RUnlock()

	if existing == nil || len(existing.entries) == 0 {
		return nil
	}

	tables, err := m.fetchTableMetadata()
	if err != nil || tables == nil {
		// Program may have been unloaded; clean up tracking regardless.
		m.removeTracking(nfKey, nfConfig)
		return nil
	}

	if err := m.writeEntries(existing.entries, p4v1.Update_DELETE); err != nil {
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

func (m *RealManager) fetchTableMetadata() ([]api.TableMetadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := m.p4client.GetForwardingPipelineConfig(ctx, &p4v1.GetForwardingPipelineConfigRequest{
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

func (m *RealManager) writeEntries(entries []*p4v1.TableEntry, updateType p4v1.Update_Type) error {
	updates := make([]*p4v1.Update, 0, len(entries))
	for _, e := range entries {
		updates = append(updates, &p4v1.Update{
			Type:   updateType,
			Entity: &p4v1.Entity{Entity: &p4v1.Entity_TableEntry{TableEntry: e}},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.p4client.Write(ctx, &p4v1.WriteRequest{
		DeviceId:   m.deviceID,
		ElectionId: &p4v1.Uint128{High: m.electionIDHigh, Low: m.electionIDLow},
		Updates:    updates,
	})
	return err
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
