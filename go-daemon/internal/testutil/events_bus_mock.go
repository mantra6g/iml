package testutil

import (
	"iml-daemon/services/events"

	"github.com/stretchr/testify/mock"
)

type MockEventBus struct {
	mock.Mock
}
func (eb *MockEventBus) Subscribe(eventName string, handler events.Handler) uint64 {
	args := eb.Called(eventName, handler)
	return args.Get(0).(uint64)
}
func (eb *MockEventBus) Unsubscribe(eventName string, id uint64) {
	eb.Called(eventName, id)
}
func (eb *MockEventBus) Publish(event events.Event) {
	eb.Called(event)
}