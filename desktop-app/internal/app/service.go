package app

import (
	"context"
	"sync"

	"eve-assistant/desktop-app/internal/protocol"
)

type EventSink func(protocol.EventEnvelope)

type ApplicationService struct {
	mu    sync.RWMutex
	sinks []EventSink
}

func NewApplicationService() *ApplicationService { return &ApplicationService{} }

func (s *ApplicationService) Subscribe(sink EventSink) func() {
	s.mu.Lock()
	s.sinks = append(s.sinks, sink)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, candidate := range s.sinks {
			if &candidate == &sink {
				s.sinks = append(s.sinks[:i], s.sinks[i+1:]...)
				break
			}
		}
	}
}

func (s *ApplicationService) Publish(_ context.Context, event protocol.EventEnvelope) {
	s.mu.RLock()
	sinks := append([]EventSink(nil), s.sinks...)
	s.mu.RUnlock()
	for _, sink := range sinks {
		sink(event)
	}
}
