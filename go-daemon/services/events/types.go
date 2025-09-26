package events

import "sync"


const (
	EventAppGroupCreated      = "apps:group:created"
	EventAppGroupRemoved      = "apps:group:removed"
	EventAppInstanceCreated   = "apps:instance:created"
	EventAppInstanceRemoved   = "apps:instance:removed"
	EventVnfGroupCreated      = "vnfs:group:created"
	EventVnfGroupRemoved      = "vnfs:group:removed"
	EventVnfInstanceCreated   = "vnfs:instance:created"
	EventVnfInstanceRemoved   = "vnfs:instance:removed"
	EventChainCreated         = "chains:chain:created"
	EventChainRemoved         = "chains:chain:removed"
	EventChainUpdated         = "chains:chain:updated"
	EventRouteCalculated      = "routes:calculated"
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