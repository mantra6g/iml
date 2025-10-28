package appchains

import (
	"builder/pkg/cache"
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

type Service struct {
	cache *cache.Service
	bus   *events.EventBus
	logger logr.Logger
}

func NewService(cache *cache.Service, bus *events.EventBus, logger logr.Logger) (*Service, error) {
	if cache == nil {
		return nil, fmt.Errorf("cache service is required")
	}
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}

	service := &Service{
		cache: cache,
		bus:   bus,
		logger: logger,
	}

	bus.Subscribe(events.EventAppUpdated, service.handleAppUpdatedEvent)
	bus.Subscribe(events.EventAppDeleted, service.handleAppDeletedEvent)
	bus.Subscribe(events.EventChainUpdated, service.handleChainUpdatedEvent)
	bus.Subscribe(events.EventChainDeleted, service.handleChainDeletedEvent)

	return service, nil
}

func (s *Service) handleAppUpdatedEvent(event events.Event) {
	s.logger.Info("Handling AppUpdated event")
	app, ok := event.Payload.(*dto.ApplicationDefinition)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for AppUpdated event"), "error casting event payload to Application")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(types.UID(app.ID))
	s.logger.Info("Got app service chains", "appChains", appChains, "err", err)
	if err != nil {
		// App chains does not exist, create new
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "active",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  app.ID,
			Chains: []string{},
		}
	} else {
		appChainsPtr = &appChains
		appChainsPtr.ObjectMetadata = dto.ObjectMetadata{
			Version:   appChainsPtr.Version,
			Status:    "active",
			Seq:       appChainsPtr.Seq + 1,
			Timestamp: time.Now(),
		}
	}

	s.logger.Info("Publishing app chains pre-updated event", "appChains", appChainsPtr)
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
	s.logger.Info("Successfully published app chains pre-updated event", "appID", appChainsPtr.AppID)
}

func (s *Service) handleAppDeletedEvent(event events.Event) {
	s.logger.Info("Handling AppDeleted event")
	app, ok := event.Payload.(*dto.ApplicationDefinition)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for AppDeleted event"), "error casting event payload to Application")
		return
	}
	
	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(types.UID(app.ID))
	s.logger.Info("Got app service chains", "appChains", appChains, "err", err)
	if err != nil {
		// App chains does not exist, create new with deleted status
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "deleted",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  app.ID,
			Chains: []string{},
		}
	} else {
		appChainsPtr = &appChains
		appChainsPtr.ObjectMetadata = dto.ObjectMetadata{
			Version:   appChainsPtr.Version,
			Status:    "deleted",
			Seq:       appChainsPtr.Seq + 1,
			Timestamp: time.Now(),
		}
		appChainsPtr.AppID = app.ID
		appChainsPtr.Chains = []string{} // Clear chains on deletion
	}

	s.logger.Info("Publishing app chains pre-updated event for deletion", "appChains", appChainsPtr)
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
	s.logger.Info("Successfully published app chains pre-updated event for deletion", "appID", appChainsPtr.AppID)
}

func (s *Service) handleChainUpdatedEvent(event events.Event) {
	s.logger.Info("Handling ChainUpdated event")
	chain, ok := event.Payload.(*dto.ServiceChainDefinition)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for ChainUpdated event"), "error casting event payload to ServiceChainDefinition")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(types.UID(chain.SrcAppID))
	s.logger.Info("Got app service chains", "appChains", appChains, "err", err)
	if err != nil {
		// App chains does not exist, create new
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "active",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  chain.SrcAppID,
			Chains: []string{chain.ID},
		}
	} else {
		appChainsPtr = &appChains
		appChainsPtr.ObjectMetadata = dto.ObjectMetadata{
			Version:   appChainsPtr.Version,
			Status:    "active",
			Seq:       appChainsPtr.Seq + 1,
			Timestamp: time.Now(),
		}
		appChainsPtr.AppID = chain.SrcAppID

		// Add chain if not already present
		found := false
		for _, c := range appChainsPtr.Chains {
			if c == chain.ID {
				found = true
				break
			}
		}
		if found {
			return // No need to publish if chain already exists
		}
		appChainsPtr.Chains = append(appChainsPtr.Chains, chain.ID)
	}

	s.logger.Info("Publishing app chains pre-updated event", "appChains", appChainsPtr)
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
	s.logger.Info("Successfully published app chains pre-updated event", "appID", appChainsPtr.AppID)
}

func (s *Service) handleChainDeletedEvent(event events.Event) {
	s.logger.Info("Handling ChainDeleted event")
	chain, ok := event.Payload.(*dto.ServiceChainDefinition)
	if !ok {
		s.logger.Error(fmt.Errorf("invalid payload for ChainDeleted event"), "error casting event payload to ServiceChain")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(types.UID(chain.SrcAppID))
	s.logger.Info("Got app service chains", "appChains", appChains, "err", err)
	if err != nil {
		// App chains does not exist, nothing to delete
		return
	}
	appChainsPtr = &appChains
	appChainsPtr.ObjectMetadata = dto.ObjectMetadata{
		Version:   appChainsPtr.Version,
		Status:    "active",
		Seq:       appChainsPtr.Seq + 1,
		Timestamp: time.Now(),
	}
	appChainsPtr.AppID = chain.SrcAppID

	// Remove the chain from the list
	newChains := []string{}
	found := false
	for _, c := range appChainsPtr.Chains {
		if c != chain.ID {
			newChains = append(newChains, c)
			continue
		}
		found = true
	}
	if !found {
		// Chain not found, nothing to do
		return
	}
	appChainsPtr.Chains = newChains

	s.logger.Info("Publishing app chains pre-updated event", "appChains", appChainsPtr)
	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
	s.logger.Info("Successfully published app chains pre-updated event", "appID", appChainsPtr.AppID)
}