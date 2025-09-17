package events

import "sync"


const (
	EventAppPreUpdated = "app:updated:pre"
	EventAppUpdated    = "app:updated"
	EventAppPreDeleted = "app:deleted:pre"
	EventAppDeleted    = "app:deleted"
	EventNfPreUpdated  = "nf:updated:pre"
	EventNfUpdated     = "nf:updated"
	EventNfPreDeleted  = "nf:deleted:pre"
	EventNfDeleted     = "nf:deleted"
	EventChainPreUpdated = "chain:updated:pre"
	EventChainUpdated    = "chain:updated"
	EventChainPreDeleted = "chain:deleted:pre"
	EventChainDeleted    = "chain:deleted"
	EventAppChainsPreUpdated = "app_services:updated:pre"
	EventAppChainsUpdated    = "app_services:updated"
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