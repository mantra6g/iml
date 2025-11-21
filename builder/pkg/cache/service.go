package cache

import (
	"builder/api/v1alpha1"
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

type Service struct {	
	appCache       *Cache[types.UID, dto.ApplicationDefinition]
	nfCache        *Cache[types.UID, dto.NetworkFunctionDefinition]
	appChainsCache *Cache[types.UID, dto.ApplicationServiceChains]
	chainCache     *Cache[types.UID, dto.ServiceChainDefinition]
	bus            *events.EventBus
	logger         logr.Logger
}

func New(eventbus *events.EventBus, logger logr.Logger) (*Service, error) {
	service := &Service{
		appCache:   NewCache[types.UID, dto.ApplicationDefinition](),
		nfCache:    NewCache[types.UID, dto.NetworkFunctionDefinition](),
		appChainsCache: NewCache[types.UID, dto.ApplicationServiceChains](),
		chainCache: NewCache[types.UID, dto.ServiceChainDefinition](),
		bus:        eventbus,
		logger:     logger,
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
	s.logger.Info("Received app update event", "event", event)
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for AppUpdated event"), "handleAppUpdatedEvent error")
		return
	}
	appID := app.UID
	if app.Spec.OverrideID != "" {
		appID = types.UID(app.Spec.OverrideID)
	}
	if err := s.UpdateApp(appID, app); err != nil {
		s.logger.Error(fmt.Errorf("error updating app in cache: %v", err), "handleAppUpdatedEvent error")
	}
}

func (s *Service) handleAppDeletedEvent(event events.Event) {
	s.logger.Info("Received app delete event", "event", event)
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for AppDeleted event"), "handleAppDeletedEvent error")
		return
	}
	appID := app.UID
	if app.Spec.OverrideID != "" {
		appID = types.UID(app.Spec.OverrideID)
	}
	if err := s.DeleteApp(appID); err != nil {
		s.logger.Error(fmt.Errorf("error deleting app from cache: %v", err), "handleAppDeletedEvent error")
	}
}

func (s *Service) handleNfUpdatedEvent(event events.Event) {
	s.logger.Info("Received nf update event", "event", event)
	nf, ok := event.Payload.(*v1alpha1.NetworkFunction)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for NfUpdated event"), "handleNfUpdatedEvent error")
		return
	}
	if err := s.UpdateNF(nf.UID, nf); err != nil {
		s.logger.Error(fmt.Errorf("error updating network function in cache: %v", err), "handleNfUpdatedEvent error")
	}
}

func (s *Service) handleNfDeletedEvent(event events.Event) {
	s.logger.Info("Received nf delete event", "event", event)
	nf, ok := event.Payload.(*v1alpha1.NetworkFunction)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for NfDeleted event"), "handleNfDeletedEvent error")
		return
	}
	if err := s.DeleteNF(nf.UID); err != nil {
		s.logger.Error(fmt.Errorf("error deleting network function from cache: %v", err), "handleNfDeletedEvent error")
	}
}

func (s *Service) handleChainUpdatedEvent(event events.Event) {
	s.logger.Info("Received chain update event", "event", event)
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for ChainUpdated event"), "handleChainUpdatedEvent error")
		return
	}
	if err := s.UpdateServiceChain(chain.UID, chain); err != nil {
		s.logger.Error(fmt.Errorf("error updating service chain in cache: %v", err), "handleChainUpdatedEvent error")
	}
}

func (s *Service) handleChainDeletedEvent(event events.Event) {
	s.logger.Info("Received chain delete event", "event", event)
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for ChainDeleted event"), "handleChainDeletedEvent error")
		return
	}
	if err := s.DeleteServiceChain(chain.UID); err != nil {
		s.logger.Error(fmt.Errorf("error deleting service chain from cache: %v", err), "handleChainDeletedEvent error")
	}
}

func (s *Service) handleAppChainsUpdatedEvent(event events.Event) {
	s.logger.Info("Received app chains update event", "event", event)
	appChains, ok := event.Payload.(*dto.ApplicationServiceChains)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for AppChainsUpdated event"), "handleAppChainsUpdatedEvent error")
		return
	}
	if err := updateEntry(s.appChainsCache, types.UID(appChains.AppID), *appChains); err != nil {
		s.logger.Error(fmt.Errorf("error updating app service chains in cache: %v", err), "handleAppChainsUpdatedEvent error")
	}
	s.logger.Info("Successfully updated app service chains in cache", "appID", appChains.AppID)
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsUpdated,
		Payload: appChains,
	})
	s.logger.Info("Successfully published app chains updated event", "appID", appChains.AppID)
}

func (s *Service) GetApp(uid types.UID) (dto.ApplicationDefinition, error) {
	return getEntry(s.appCache, uid)
}

func (s *Service) UpdateApp(appID types.UID, app *v1alpha1.Application) error {
	seq := getNextSeq(s.appCache, appID)
	appDef := dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:        string(appID),
		Name:      app.Name,
		Namespace: app.Namespace,
	}
	err := updateEntry(s.appCache, appID, appDef)
	if err != nil {
		return fmt.Errorf("failed to update app: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventAppUpdated,
		Payload: &appDef,
	})
	return nil
}

func (s *Service) DeleteApp(appID types.UID) error {
	appDef := dto.ApplicationDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.appCache, appID),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.appCache, appID, appDef)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventAppDeleted,
		Payload: &appDef,
	})
	return nil
}

func (s *Service) GetNF(nfID types.UID) (dto.NetworkFunctionDefinition, error) {
	return getEntry(s.nfCache, nfID)
}

func (s *Service) UpdateNF(nfID types.UID, nf *v1alpha1.NetworkFunction) error {
	nfDef := dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       getNextSeq(s.nfCache, nfID),
			Timestamp: time.Now(),
		},
		ID:        string(nfID),
		Name:      nf.Name,
		Namespace: nf.Namespace,
		Type: 		 nf.Spec.Type,
		SubFunctions: func(subFuncs []v1alpha1.SubFunctionSpec) []dto.SubFunctionDefinition {
			defs := make([]dto.SubFunctionDefinition, len(subFuncs))
			for i, sf := range subFuncs {
				defs[i] = dto.SubFunctionDefinition{
					ID:   sf.ID,
				}
			}
			return defs
		}(nf.Spec.SubFunctions),
	}
	err := updateEntry(s.nfCache, nfID, nfDef)
	if err != nil {
		return fmt.Errorf("failed to update network function: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventNfUpdated,
		Payload: &nfDef,
	})
	return nil
}

func (s *Service) DeleteNF(nfID types.UID) error {
	nfDef := dto.NetworkFunctionDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.nfCache, nfID),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.nfCache, nfID, nfDef)
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

func (s *Service) GetServiceChain(chainID types.UID) (dto.ServiceChainDefinition, error) {
	return getEntry(s.chainCache, chainID)
}

func (s *Service) ListServiceChains() []dto.ServiceChainDefinition {
	return s.chainCache.List()
}

func (s *Service) UpdateServiceChain(chainID types.UID, chain *v1alpha1.ServiceChain) error {
	seq := getNextSeq(s.chainCache, chainID)
	chainDef := dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "active",
			Seq:       seq,
			Timestamp: time.Now(),
		},
		ID:   string(chainID),
		Name: chain.Name,
		Namespace: chain.Namespace,
		SrcAppID: string(chain.Status.SourceAppUID),
		DstAppID: string(chain.Status.DestinationAppUID),
		Functions: chain.Status.Functions,
	}
	err := updateEntry(s.chainCache, chainID, chainDef)
	if err != nil {
		return fmt.Errorf("failed to update service chain: %w", err)
	}
	s.bus.Publish(events.Event{
		Name:    events.EventChainUpdated,
		Payload: &chainDef,
	})
	return nil
}

func (s *Service) DeleteServiceChain(chainID types.UID) error {
	chainDef := dto.ServiceChainDefinition{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    "deleted",
			Seq:       getNextSeq(s.chainCache, chainID),
			Timestamp: time.Now(),
		},
	}
	err := updateEntry(s.chainCache, chainID, chainDef)
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