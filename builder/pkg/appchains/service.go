package appchains

import (
	"builder/api/v1alpha1"
	"builder/pkg/cache"
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"fmt"
	"time"
)

type Service struct {
	cache *cache.Service
	bus   *events.EventBus
}

func NewService(cache *cache.Service, bus *events.EventBus) *Service {
	service := &Service{
		cache: cache,
		bus:   bus,
	}

	bus.Subscribe(events.EventAppUpdated, service.handleAppUpdatedEvent)
	bus.Subscribe(events.EventAppDeleted, service.handleAppDeletedEvent)
	bus.Subscribe(events.EventChainUpdated, service.handleChainUpdatedEvent)
	bus.Subscribe(events.EventChainDeleted, service.handleChainDeletedEvent)

	return service
}

func (s *Service) handleAppUpdatedEvent(event events.Event) {
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		fmt.Printf("Invalid payload for AppUpdated event\n")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(app.UID)
	if err != nil {
		// App chains does not exist, create new
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "active",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  string(app.UID),
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

	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
}

func (s *Service) handleAppDeletedEvent(event events.Event) {
	app, ok := event.Payload.(*v1alpha1.Application)
	if !ok {
		fmt.Printf("Invalid payload for AppDeleted event\n")
		return
	}
	
	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(app.UID)
	if err != nil {
		// App chains does not exist, create new with deleted status
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "deleted",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  string(app.UID),
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
		appChainsPtr.AppID = string(app.UID)
		appChainsPtr.Chains = []string{} // Clear chains on deletion
	}

	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
}

func (s *Service) handleChainUpdatedEvent(event events.Event) {
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		fmt.Printf("Invalid payload for ChainUpdated event\n")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(chain.Status.SourceAppUID)
	if err != nil {
		// App chains does not exist, create new
		appChainsPtr = &dto.ApplicationServiceChains{
			ObjectMetadata: dto.ObjectMetadata{
				Version:   "1.0",
				Status:    "active",
				Seq:       1,
				Timestamp: time.Now(),
			},
			AppID:  string(chain.Status.SourceAppUID),
			Chains: []string{string(chain.UID)},
		}
	} else {
		appChainsPtr = &appChains
		appChainsPtr.ObjectMetadata = dto.ObjectMetadata{
			Version:   appChainsPtr.Version,
			Status:    "active",
			Seq:       appChainsPtr.Seq + 1,
			Timestamp: time.Now(),
		}
		appChainsPtr.AppID = string(chain.Status.SourceAppUID)

		// Add chain if not already present
		found := false
		for _, c := range appChainsPtr.Chains {
			if c == string(chain.UID) {
				found = true
				break
			}
		}
		if !found {
			appChainsPtr.Chains = append(appChainsPtr.Chains, string(chain.UID))
			return // No need to publish if chain already exists
		}
	}

	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
}

func (s *Service) handleChainDeletedEvent(event events.Event) {
	chain, ok := event.Payload.(*v1alpha1.ServiceChain)
	if !ok {
		fmt.Printf("Invalid payload for ChainDeleted event\n")
		return
	}

	var appChainsPtr *dto.ApplicationServiceChains
	appChains, err := s.cache.GetAppServiceChains(chain.Status.SourceAppUID)
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
	appChainsPtr.AppID = string(chain.Status.SourceAppUID)

	// Remove the chain from the list
	newChains := []string{}
	found := false
	for _, c := range appChainsPtr.Chains {
		if c != string(chain.UID) {
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

	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsPtr,
	})
}