package cache

import (
	"builder/api/v1alpha1"
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
)

type Service struct {	
	appCache       *Cache[types.UID, dto.ApplicationDefinition]
	nfCache        *Cache[types.UID, dto.NetworkFunctionDefinition]
	appChainsCache *Cache[types.UID, dto.ApplicationServiceChains]
	chainCache     *Cache[types.UID, dto.ServiceChainDefinition]
	bus            *events.EventBus
}

func New(eventbus *events.EventBus) (*Service, error) {
	service := &Service{
		appCache:   NewCache[types.UID, dto.ApplicationDefinition](),
		nfCache:    NewCache[types.UID, dto.NetworkFunctionDefinition](),
		chainCache: NewCache[types.UID, dto.ServiceChainDefinition](),
		bus:        eventbus,
	}

	eventbus.Subscribe(events.EventAppPreUpdated, service.handleAppUpdatedEvent)
	eventbus.Subscribe(events.EventAppPreDeleted, service.handleAppDeletedEvent)
	eventbus.Subscribe(events.EventNfPreUpdated, service.handleNfUpdatedEvent)
	eventbus.Subscribe(events.EventNfPreDeleted, service.handleNfDeletedEvent)
	eventbus.Subscribe(events.EventChainPreUpdated, service.handleChainUpdatedEvent)
	eventbus.Subscribe(events.EventChainPreDeleted, service.handleChainDeletedEvent)
	eventbus.Subscribe(events.EventAppChainsPreUpdated, service.handleAppChainsUpdatedEvent)

	return service, nil
}

func (s *Service) handleAppUpdatedEvent(event events.Event) {
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		fmt.Printf("Invalid payload for AppUpdated event\n")
		return
	}
	if err := s.UpdateApp(app.UID, app); err != nil {
		fmt.Printf("Error updating app in cache: %v\n", err)
	}
}

func (s *Service) handleAppDeletedEvent(event events.Event) {
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		fmt.Printf("Invalid payload for AppDeleted event\n")
		return
	}
	if err := s.DeleteApp(app.UID); err != nil {
		fmt.Printf("Error deleting app from cache: %v\n", err)
	}
}

func (s *Service) handleNfUpdatedEvent(event events.Event) {
	nf, ok := event.Payload.(*v1alpha1.NetworkFunction)
	if !ok {
		fmt.Printf("Invalid payload for NfUpdated event\n")
		return
	}
	if err := s.UpdateNF(nf.UID, nf); err != nil {
		fmt.Printf("Error updating network function in cache: %v\n", err)
	}
}

func (s *Service) handleNfDeletedEvent(event events.Event) {
	nf, ok := event.Payload.(*v1alpha1.NetworkFunction)
	if !ok {
		fmt.Printf("Invalid payload for NfDeleted event\n")
		return
	}
	if err := s.DeleteNF(nf.UID); err != nil {
		fmt.Printf("Error deleting network function from cache: %v\n", err)
	}
}

func (s *Service) handleChainUpdatedEvent(event events.Event) {
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		fmt.Printf("Invalid payload for ChainUpdated event\n")
		return
	}
	if err := s.UpdateServiceChain(chain.UID, chain); err != nil {
		fmt.Printf("Error updating service chain in cache: %v\n", err)
	}
}

func (s *Service) handleChainDeletedEvent(event events.Event) {
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		fmt.Printf("Invalid payload for ChainDeleted event\n")
		return
	}
	if err := s.DeleteServiceChain(chain.UID); err != nil {
		fmt.Printf("Error deleting service chain from cache: %v\n", err)
	}
}

func (s *Service) handleAppChainsUpdatedEvent(event events.Event) {
	appChains, ok := event.Payload.(*dto.ApplicationServiceChains)
	if !ok {
		fmt.Printf("Invalid payload for AppChainsUpdated event\n")
		return
	}
	if err := updateEntry(s.appChainsCache, types.UID(appChains.AppID), *appChains); err != nil {
		fmt.Printf("Error updating app service chains in cache: %v\n", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsUpdated,
		Payload: appChains,
	})
}

func (s *Service) GetApp(uid types.UID) (dto.ApplicationDefinition, error) {
	return getEntry(s.appCache, uid)
}

func (s *Service) UpdateApp(uid types.UID, app *v1alpha1.Application) error {
	seq := getNextSeq(s.appCache, uid)
	appDef := dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        string(app.UID),
		Name:      app.Name,
		Namespace: app.Namespace,
	}
	err := updateEntry(s.appCache, uid, appDef)
	if err != nil {
		return fmt.Errorf("failed to update app: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventAppUpdated,
		Payload: &appDef,
	})
	return nil
}

func (s *Service) DeleteApp(uid types.UID) error {
	appDef := dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.appCache, uid),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.appCache, uid, appDef)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventAppDeleted,
		Payload: &appDef,
	})
	return nil
}

func (s *Service) GetNF(uid types.UID) (dto.NetworkFunctionDefinition, error) {
	return getEntry(s.nfCache, uid)
}

func (s *Service) UpdateNF(uid types.UID, nf *v1alpha1.NetworkFunction) error {
	nfDef := dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       getNextSeq(s.nfCache, uid),
			Timestamp: time.Now(),
		},
		ID:        string(nf.UID),
		Name:      nf.Name,
		Namespace: nf.Namespace,
	}
	err := updateEntry(s.nfCache, uid, nfDef)
	if err != nil {
		return fmt.Errorf("failed to update network function: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventNfUpdated,
		Payload: &nfDef,
	})
	return nil
}

func (s *Service) DeleteNF(uid types.UID) error {
	nfDef := dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.nfCache, uid),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.nfCache, uid, nfDef)
	if err != nil {
		return fmt.Errorf("failed to delete network function: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventNfDeleted,
		Payload: &nfDef,
	})
	return nil
}

func (s *Service) GetAppServiceChains(appID types.UID) (dto.ApplicationServiceChains, error) {
	return getEntry(s.appChainsCache, appID)
}

func (s *Service) GetServiceChain(uid types.UID) (dto.ServiceChainDefinition, error) {
	return getEntry(s.chainCache, uid)
}

func (s *Service) UpdateServiceChain(uid types.UID, chain *v1alpha1.ServiceChain) error {
	seq := getNextSeq(s.chainCache, uid)
	chainDef := dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:   string(chain.UID),
		Name: chain.Name,
		Namespace: chain.Namespace,
		SrcAppID: string(chain.Status.SourceAppUID),
		DstAppID: string(chain.Status.DestinationAppUID),
		Functions: func(uids []types.UID) []string {
			strs := make([]string, len(uids))
			for i, uid := range uids {
				strs[i] = string(uid)
			}
			return strs
		}(chain.Status.Functions),
	}
	err := updateEntry(s.chainCache, uid, chainDef)
	if err != nil {
		return fmt.Errorf("failed to update service chain: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventChainUpdated,
		Payload: &chainDef,
	})
	return nil
}

func (s *Service) DeleteServiceChain(uid types.UID) error {
	chainDef := dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.chainCache, uid),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.chainCache, uid, chainDef)
	if err != nil {
		return fmt.Errorf("failed to delete service chain: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventChainDeleted,
		Payload: &chainDef,
	})
	return nil
}

func getNextSeq[T dto.Versionable](cache *Cache[types.UID, T], uid types.UID) uint {
	seq := uint(1)
	prevEntry, exists := cache.Get(uid)
	if exists {
		seq = prevEntry.GetSeq() + 1
	}
	return seq
}

func updateEntry[T dto.Versionable](cache *Cache[types.UID, T], uid types.UID, entry T) error {
	cache.Set(uid, entry)
	return nil
}

func getEntry[T dto.Versionable](cache *Cache[types.UID, T], uid types.UID) (T, error) {
	entry, ok := cache.Get(uid)
	if !ok {
		var zero T
		return zero, fmt.Errorf("entry not found")
	}
	return entry, nil
}