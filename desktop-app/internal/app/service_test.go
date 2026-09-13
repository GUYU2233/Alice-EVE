package app

import (
	"context"
	"sync/atomic"
	"testing"

	"eve-assistant/desktop-app/internal/protocol"
)

func TestSubscribeUnsubscribeRemovesCorrectSink(t *testing.T) {
	s := NewApplicationService()
	var first, second atomic.Int32
	stopFirst := s.Subscribe(func(protocol.EventEnvelope) { first.Add(1) })
	_ = s.Subscribe(func(protocol.EventEnvelope) { second.Add(1) })
	stopFirst()
	stopFirst() // idempotent
	s.Publish(context.Background(), protocol.EventEnvelope{MessageID: "event"})
	if first.Load() != 0 || second.Load() != 1 {
		t.Fatalf("unexpected delivery: first=%d second=%d", first.Load(), second.Load())
	}
}
