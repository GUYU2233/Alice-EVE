package app

import (
	"context"
	"sync"

	"eve-assistant/desktop-app/internal/protocol"
)

type EventSink func(protocol.EventEnvelope)

type subscription struct {
	id   uint64
	sink EventSink
}

type ApplicationService struct {
	mu     sync.RWMutex
	nextID uint64
	subs   []subscription
}

func NewApplicationService() *ApplicationService { return &ApplicationService{} }

// Subscribe registers a sink and returns an idempotent unsubscribe function.
// Function values cannot be compared in Go, so removal is keyed by an internal id.
func (s *ApplicationService) Subscribe(sink EventSink) func() {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.subs = append(s.subs, subscription{id: id, sink: sink})
	s.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			for i, sub := range s.subs {
				if sub.id == id {
					s.subs = append(s.subs[:i], s.subs[i+1:]...)
					return
				}
			}
		})
	}
}

func (s *ApplicationService) Publish(_ context.Context, event protocol.EventEnvelope) {
	s.mu.RLock()
	sinks := make([]EventSink, 0, len(s.subs))
	for _, sub := range s.subs {
		sinks = append(sinks, sub.sink)
	}
	s.mu.RUnlock()
	for _, sink := range sinks {
		sink(event)
	}
}
