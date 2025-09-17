package mqtt

import (
	"builder/pkg/cache/dto"
	"builder/pkg/events"
	"encoding/json"
	"fmt"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type MQTTService struct {
	broker *mqtt.Server
	bus    *events.EventBus
}

func Initialize(bus *events.EventBus) (*MQTTService, error) {
	// Create the new MQTT Server.
	server := mqtt.New(nil)

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
	app, ok := event.Payload.(*dto.ApplicationDefinition)
	if !ok {
		fmt.Printf("Invalid payload for AppUpdated event\n")
		return
	}
	bytes, err := json.Marshal(app)
	if err != nil {
		fmt.Printf("Failed to marshal app definition: %v\n", err)
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("apps/%s/definition", app.ID), bytes, true, 1)
	if err != nil {
		fmt.Printf("Failed to publish app definition: %v\n", err)
	}
}

func (svc *MQTTService) handleChainUpdates(event events.Event) {
	chain, ok := event.Payload.(*dto.ServiceChainDefinition)
	if !ok {
		fmt.Printf("Invalid payload for ChainUpdated event\n")
		return
	}
	bytes, err := json.Marshal(chain)
	if err != nil {
		fmt.Printf("Failed to marshal chain definition: %v\n", err)
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("chains/%s/definition", chain.ID), bytes, true, 1)
	if err != nil {
		fmt.Printf("Failed to publish chain definition: %v\n", err)
	}
}

func (svc *MQTTService) handleNFUpdates(event events.Event) {
	nf, ok := event.Payload.(*dto.NetworkFunctionDefinition)
	if !ok {
		fmt.Printf("Invalid payload for NfUpdated event\n")
		return
	}
	bytes, err := json.Marshal(nf)
	if err != nil {
		fmt.Printf("Failed to marshal network function definition: %v\n", err)
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("nfs/%s/definition", nf.ID), bytes, true, 1)
	if err != nil {
		fmt.Printf("Failed to publish network function definition: %v\n", err)
	}
}

func (svc *MQTTService) handleAppChainsUpdates(event events.Event) {
	appChains, ok := event.Payload.(*dto.ApplicationServiceChains)
	if !ok {
		fmt.Printf("Invalid payload for AppChainsUpdated event\n")
		return
	}
	bytes, err := json.Marshal(appChains)
	if err != nil {
		fmt.Printf("Failed to marshal application service chains: %v\n", err)
		return
	}
	err = svc.broker.Publish(fmt.Sprintf("apps/%s/chains", appChains.AppID), bytes, true, 1)
	if err != nil {
		fmt.Printf("Failed to publish application service chains: %v\n", err)
	}
}
