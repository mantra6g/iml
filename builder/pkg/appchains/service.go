package appchains

import (
	"builder/pkg/cache"
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
)

type Service struct {
	cache        *cache.Service
	bus          events.EventBus
	logger       logr.Logger
	eventChannel chan events.Event
	ctx          context.Context
	ctxCancel    context.CancelFunc
}

type Result struct {
	Requeue bool
}

func NewService(cache *cache.Service, bus events.EventBus, logger logr.Logger) (*Service, error) {
	if cache == nil {
		return nil, fmt.Errorf("cache service is required")
	}
	if bus == nil {
		return nil, fmt.Errorf("event bus is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{
		cache:        cache,
		bus:          bus,
		logger:       logger,
		ctx:          ctx,
		ctxCancel:    cancel,
		eventChannel: make(chan events.Event),
	}

	bus.Subscribe(events.EventAppUpdated, service.enqueueEvent)
	bus.Subscribe(events.EventAppDeleted, service.enqueueEvent)
	bus.Subscribe(events.EventChainUpdated, service.enqueueEvent)
	bus.Subscribe(events.EventChainDeleted, service.enqueueEvent)

	go service.dispatchEvents()

	return service, nil
}

// Why a Reconcile function?
// Because events can come at random times, out of order and concurrently.
// Obtaining the current state from the cache and reconciling it ensures that
// we always work with the latest data and can handle events in any order.
func (s *Service) Reconcile(e events.Event) (Result, error) {
	var appID types.UID
	switch payload := e.Payload.(type) {
	case *dto.ApplicationDefinition:
		appID = types.UID(payload.ID)
	case *dto.ServiceChainDefinition:
		appID = types.UID(payload.SrcAppID)
	default:
		return Result{}, fmt.Errorf("unknown event payload type: %T", e.Payload)
	}

	app, err := s.cache.GetApp(appID)
	if err != nil {
		return Result{}, fmt.Errorf("failed to get application definition for appID %s: %w", appID, err)
	}

	var relevantChainIDs []string
	if app.Status == "active" {
		chains := s.cache.ListServiceChains()
		for _, chain := range chains {
			if chain.SrcAppID == string(appID) {
				relevantChainIDs = append(relevantChainIDs, chain.ID)
			}
		}
	}

	var seq uint = 1
	lastAppChainsDto, err := s.cache.GetAppServiceChains(appID)
	if err == nil {
		seq = lastAppChainsDto.Seq + 1
	}

	appChainsDto := &dto.ApplicationServiceChains{
		ObjectMetadata: dto.ObjectMetadata{
			Version:   "1.0",
			Status:    app.Status,
			Seq:       seq,
			Timestamp: time.Now(),
		},
		AppID:  string(appID),
		Chains: relevantChainIDs,
	}

	s.bus.Publish(events.Event{
		Name:    events.EventAppChainsPreUpdated,
		Payload: appChainsDto,
	})

	return Result{}, nil
}

func (s *Service) Shutdown() {
	s.ctxCancel()
}

func (s *Service) enqueueEvent(evt events.Event) {
	s.logger.Info("Enqueuing event", "event", evt)
	s.eventChannel <- evt
}

func (s *Service) dispatchEvents() {
	for {
		select {
		case evt := <-s.eventChannel:
			s.logger.Info("Processing event", "event", evt)
			res, err := s.Reconcile(evt)
			if res.Requeue {
				s.logger.Info("Requeuing event", "event", evt)
				go func() {
					time.Sleep(1 * time.Second) // Simple backoff
					s.eventChannel <- evt
				}()
			}
			if err != nil {
				s.logger.Error(err, "Error reconciling event", "event", evt)
				continue
			}
		case <-s.ctx.Done():
			s.logger.Info("Shutting down event processing")
			return
		}
	}
}
