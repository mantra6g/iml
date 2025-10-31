package events

import "sync"

const (
	EventLocalAppGroupCreated  = "apps:group:local:created"
	EventLocalAppGroupRemoved  = "apps:group:local:removed"
	EventRemoteAppGroupCreated = "apps:group:remote:created"
	EventRemoteAppGroupRemoved = "apps:group:remote:removed"
	EventLocalVnfGroupCreated  = "vnfs:group:local:created"
	EventLocalVnfGroupRemoved  = "vnfs:group:local:removed"
	EventRemoteVnfGroupCreated  = "vnfs:group:remote:created"
	EventRemoteVnfGroupRemoved  = "vnfs:group:remote:removed"
	EventNodeCreated         = "nodes:worker:created"
	EventNodeRemoved         = "nodes:worker:removed"
	EventChainCreated          = "chains:chain:created"
	EventChainRemoved          = "chains:chain:removed"
	EventChainUpdated          = "chains:chain:updated"
	EventRouteRecalculationStarted  = "routes:recalculation:started"
	EventRouteRecalculationFinished = "routes:recalculation:finished"
)

// Event represents a domain event with a name and payload.
// Payload can be any type representing event data.
type Event struct {
	Name    string
	Payload any
}

// Handler is a function to process an Event.
type Handler func(Event)

// subscription holds the handler and a unique ID for unsubscribing.
type subscription struct {
	id      uint64
	handler Handler
}

// EventBus provides publish/subscribe capabilities for domain events.
type EventBus struct {
	subscribers map[string][]subscription // map of event name -> list of subscriptions
	mu          sync.RWMutex              // protects subscribers
	nextID      uint64                    // incremental ID generator
}
