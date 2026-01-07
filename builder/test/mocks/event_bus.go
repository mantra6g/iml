package mocks

import "builder/pkg/events"

type FakeEventBus struct{}

func (f *FakeEventBus) Publish(event events.Event) {
	// No-op for the fake
}

func (f *FakeEventBus) Subscribe(eventName string, handler events.Handler) uint64 {
	// No-op for the fake
	return 0
}

func (f *FakeEventBus) Unsubscribe(eventName string, id uint64) {
	// No-op for the fake
}
