package mqtt

import (
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type MQTTService struct {
	broker *mqtt.Server
	bus    *events.EventBus
	logger logr.Logger
}

func Initialize(bus *events.EventBus, logger logr.Logger) (*MQTTService, error) {
	// Create the new MQTT Server.
	server := mqtt.New(&mqtt.Options{
		InlineClient: true,
	})

	// Allow all connections.
	err := server.AddHook(new(auth.AllowHook), nil)
	if err != nil {
		return nil, err
	}

	// Create a TCP listener on a standard port.
	tcp := listeners.NewTCP(listeners.Config{ID: "t1", Address: ":1883"})
	err = server.AddListener(tcp)
	if err != nil {
		return nil, err
	}

	// Listen for incoming connections
	err = server.Serve()
	if err != nil {
		return nil, fmt.Errorf("failed to start MQTT server: %w", err)
	}

	service := &MQTTService{
		broker: server,
		bus:    bus,
		logger: logger,
	}
	bus.Subscribe(events.EventAppUpdated, service.handleAppUpdates)
	bus.Subscribe(events.EventAppDeleted, service.handleAppUpdates)
	bus.Subscribe(events.EventChainUpdated, service.handleChainUpdates)
	bus.Subscribe(events.EventChainDeleted, service.handleChainUpdates)
	bus.Subscribe(events.EventNfUpdated, service.handleNFUpdates)
	bus.Subscribe(events.EventNfDeleted, service.handleNFUpdates)
	bus.Subscribe(events.EventAppChainsUpdated, service.handleAppChainsUpdates)

	return service, nil
}

func (svc *MQTTService) Shutdown() error {
	err := svc.broker.Close()
	if err != nil {
		return fmt.Errorf("failed to shutdown MQTT server: %w", err)
	}
	return nil
}

func (svc *MQTTService) handleAppUpdates(event events.Event) {
	svc.logger.Info("Received app update", "event", event)
	app, ok := event.Payload.(*dto.ApplicationDefinition)
	if !ok {
		svc.logger.Error(fmt.Errorf("invalid payload for AppUpdated event"), "handleAppUpdates error")
		return
	}
	bytes, err := json.Marshal(app)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to marshal app definition: %v", err), "handleAppUpdates error")
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("apps/%s/definition", app.ID), bytes, true, 1)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to publish app definition: %v", err), "handleAppUpdates error")
	}
	svc.logger.Info("Published to apps definition", "topic", fmt.Sprintf("apps/%s/definition", app.ID), "payload", string(bytes))
}

func (svc *MQTTService) handleChainUpdates(event events.Event) {
	svc.logger.Info("Received chain update", "event", event)
	chain, ok := event.Payload.(*dto.ServiceChainDefinition)
	if !ok {
		svc.logger.Error(fmt.Errorf("invalid payload for ChainUpdated event"), "handleChainUpdates error")
		return
	}
	bytes, err := json.Marshal(chain)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to marshal chain definition: %v", err), "handleChainUpdates error")
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("chains/%s/definition", chain.ID), bytes, true, 1)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to publish chain definition: %v", err), "handleChainUpdates error")
	}
	svc.logger.Info("Published to chains definition", "topic", fmt.Sprintf("chains/%s/definition", chain.ID), "payload", string(bytes))
}

func (svc *MQTTService) handleNFUpdates(event events.Event) {
	svc.logger.Info("Received NF update", "event", event)
	nf, ok := event.Payload.(*dto.NetworkFunctionDefinition)
	if !ok {
		svc.logger.Error(fmt.Errorf("invalid payload for NfUpdated event"), "handleNFUpdates error")
		return
	}
	bytes, err := json.Marshal(nf)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to marshal network function definition: %v", err), "handleNFUpdates error")
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("nfs/%s/definition", nf.ID), bytes, true, 1)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to publish network function definition: %v", err), "handleNFUpdates error")
	}
	svc.logger.Info("Published to nfs definition", "topic", fmt.Sprintf("nfs/%s/definition", nf.ID), "payload", string(bytes))
}

func (svc *MQTTService) handleAppChainsUpdates(event events.Event) {
	svc.logger.Info("Received app chains update", "event", event)
	appChains, ok := event.Payload.(*dto.ApplicationServiceChains)
	if !ok {
		svc.logger.Error(fmt.Errorf("invalid payload for AppChainsUpdated event"), "handleAppChainsUpdates error")
		return
	}
	bytes, err := json.Marshal(appChains)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to marshal application service chains: %v", err), "handleAppChainsUpdates error")
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("apps/%s/services", appChains.AppID), bytes, true, 1)
	if err != nil {
		svc.logger.Error(fmt.Errorf("failed to publish application service chains: %v", err), "handleAppChainsUpdates error")
	}
	svc.logger.Info("Published to app services", "topic", fmt.Sprintf("apps/%s/services", appChains.AppID), "payload", string(bytes))
}
