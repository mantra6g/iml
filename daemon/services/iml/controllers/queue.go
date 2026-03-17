package controllers

import (
	"iml-daemon/mqtt"
	"sync"
)

type Event struct {
	Topic   Topic
	Message mqtt.Message
}

type Queue interface {
	Enqueue(Event)
	Dequeue() (Event, bool)
}

type SliceQueue struct {
	events []Event
	mutex  sync.Mutex
}

func (q *SliceQueue) Enqueue(event Event) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.events = append(q.events, event)
}
func (q *SliceQueue) Dequeue() (Event, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.events) == 0 {
		return Event{}, false
	}
	event := q.events[0]
	q.events = q.events[1:]
	return event, true
}
