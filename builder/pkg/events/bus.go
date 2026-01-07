package events

// New creates a new EventBus instance.
func New() *EventBusImpl {
	return &EventBusImpl{
		subscribers: make(map[string][]subscription),
		nextID:      1,
	}
}

// Subscribe registers a handler for the given event name. It returns a unique subscription ID.
func (eb *EventBusImpl) Subscribe(eventName string, handler Handler) uint64 {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	id := eb.nextID
	eb.nextID++

	sub := subscription{id: id, handler: handler}
	eb.subscribers[eventName] = append(eb.subscribers[eventName], sub)

	return id
}

// Unsubscribe removes the subscription for the given event name and subscription ID.
func (eb *EventBusImpl) Unsubscribe(eventName string, id uint64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	subs := eb.subscribers[eventName]
	for i, sub := range subs {
		if sub.id == id {
			// remove subscription
			eb.subscribers[eventName] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Publish sends the event to all registered handlers for its name.
// Each handler is invoked asynchronously in its own goroutine.
func (eb *EventBusImpl) Publish(event Event) {
	eb.mu.RLock()
	subs, found := eb.subscribers[event.Name]
	eb.mu.RUnlock()

	if !found {
		return
	}

	// Invoke subscribers concurrently
	for _, sub := range subs {
		go func(h Handler) {
			// recover from panics in handler to avoid crashing the bus
			defer func() {
				if r := recover(); r != nil {
					// optionally log the panic
				}
			}()

			h(event)
		}(sub.handler)
	}
}
