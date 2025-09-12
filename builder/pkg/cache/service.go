package cache

import (
	"builder/api/v1alpha1"
	"builder/pkg/cache/dto"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

type Service struct {
	appCache   *Cache[types.UID, dto.ApplicationDefinition]
	nfCache    *Cache[types.UID, dto.NetworkFunctionDefinition]
	chainCache *Cache[types.UID, dto.ServiceChainDefinition]
}

func New() (*Service, error) {
	return &Service{
		apps:       make(map[types.UID]dto.ApplicationDefinition),
		appMutex:   sync.RWMutex{},
		nfs:        make(map[types.UID]dto.NetworkFunctionDefinition),
		nfMutex:    sync.RWMutex{},
		chains:     make(map[types.UID]dto.ServiceChainDefinition),
		chainMutex: sync.RWMutex{},
	}, nil
}

func (s *Service) GetApp(uid types.UID) (dto.ApplicationDefinition, error) {
	return getEntry(s.apps, uid)
}

func (s *Service) UpdateApp(uid types.UID, app v1alpha1.Application) error {
	return updateEntry(s.apps, uid, app)
}

func (s *Service) GetNF(uid types.UID) (dto.NetworkFunctionDefinition, error) {
	return getEntry(s.nfs, uid)
}

func (s *Service) UpdateNF(uid types.UID, nf v1alpha1.NetworkFunction) error {
	return updateEntry(s.nfs, uid, nf)
}

func (s *Service) GetServiceChain(uid types.UID) (dto.ServiceChainDefinition, error) {
	return getEntry(s.chains, uid)
}

func (s *Service) UpdateServiceChain(uid types.UID, chain v1alpha1.ServiceChain) error {
	return updateEntry(s.chains, uid, chain)
}


func updateEntry[T dto.Versionable](m map[types.UID]T, uid types.UID, entry T) error {
	seq := uint(1)
	prevEntry, exists := m[uid]
	if exists {
		seq = prevEntry.GetSeq() + 1
	}
	return nil
}

func getEntry[T dto.Versionable](m map[types.UID]T, uid types.UID) (T, error) {

	entry, ok := m[uid]
	if !ok {
		return new(T), fmt.Errorf("entry not found")
	}
	return entry, nil
}