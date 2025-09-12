package mqtt

import (
	"fmt"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

type MQTTService struct {
	broker    *mqtt.Server
	apps      map[string]dto.ApplicationDefinition
	appChains map[string]dto.ApplicationServiceChains
	nfs       map[string]dto.NetworkFunctionDefinition
	chains    map[string]dto.ServiceChainDefinition
}

func Initialize() (*MQTTService, error) {
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

	return &MQTTService{
		broker: server,
	}, nil
}

func (svc *MQTTService) Shutdown() error {
	err := svc.broker.Close()
	if err != nil {
		return fmt.Errorf("failed to shutdown MQTT server: %w", err)
	}
	return nil
}
