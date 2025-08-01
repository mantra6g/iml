package mqtt

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/services/eventbus"
)

type MQTTService struct {
	eb       *eventbus.EventBus
	registry *db.Registry
}

func InitializeMQTTService(registry *db.Registry, eb *eventbus.EventBus) (*MQTTService, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}
	if eb == nil {
		return nil, fmt.Errorf("event bus cannot be nil")
	}

	svc := &MQTTService{
		eb:       eb,
		registry: registry,
	}

	return svc, nil
}
