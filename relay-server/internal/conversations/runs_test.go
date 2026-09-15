package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

type recordingRunPublisher struct {
	mu     sync.Mutex
	events []struct {
		runID string
		typ   string
		body  json.RawMessage
	}
}

func (p *recordingRunPublisher) PublishRunEvent(_ context.Context, _, _, runID, typ string, body json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, struct {
		runID string
		typ   string
		body  json.RawMessage
	}{runID: runID, typ: typ, body: append(json.RawMessage(nil), body...)})
	return nil
}

func (p *recordingRunPublisher) types() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.events))
	for i, event := range p.events {
		out[i] = event.typ
	}
	return out
}

func (p *recordingRunPublisher) snapshot() []struct {
	runID string
	typ   string
	body  json.RawMessage
} {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]struct {
		runID string
		typ   string
		body  json.RawMessage
	}, len(p.events))
	for i, event := range p.events {
		out[i] = struct {
			runID string
			typ   string
			body  json.RawMessage
		}{event.runID, event.typ, append(json.RawMessage(nil), event.body...)}
	}
	return out
}

func TestRunLifecyclePublishesEvents(t *testing.T) {
	r := NewMemoryRepository()
	a := NewMemoryAuthorizer()
	_ = a.RegisterDevice("a", "m", DeviceMobile)
	_ = a.RegisterDevice("a", "d", DeviceDesktop)
	s := NewService(r, a)
	publisher := &recordingRunPublisher{}
	s.SetRunEventPublisher(publisher)
	c, err := s.CreateConversation(context.Background(), Actor{"a", "m", DeviceMobile}, "d", "agent")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := s.SendMessage(context.Background(), Actor{"a", "m", DeviceMobile}, c.ID, MessageInput{ClientMessageID: "x", Operation: "get_recent_intel", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.StartRun(context.Background(), Actor{"a", "d", DeviceDesktop}, c.ID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateRun(context.Background(), Actor{"a", "d", DeviceDesktop}, run.ID, RunRunning, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddRunEvent(context.Background(), Actor{"a", "d", DeviceDesktop}, run.ID, "delta", json.RawMessage(`{"text":"hi"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateRun(context.Background(), Actor{"a", "d", DeviceDesktop}, run.ID, RunCompleted, ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"agent.run.created", "agent.run.updated", "agent.run.event", "agent.run.completed"}
	got := publisher.types()
	if len(got) != len(want) {
		t.Fatalf("published types=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("published types=%v, want %v", got, want)
		}
	}
	for _, event := range publisher.snapshot() {
		if event.typ == "agent.run.event" {
			continue
		}
		var payload AgentRun
		if err := json.Unmarshal(event.body, &payload); err != nil {
			t.Fatalf("%s payload is not AgentRun: %v", event.typ, err)
		}
		if payload.ID != run.ID || payload.ConversationID != c.ID {
			t.Fatalf("%s payload=%+v", event.typ, payload)
		}
	}
}

func TestRunLifecycleAndDesktopOnly(t *testing.T) {
	r := NewMemoryRepository()
	a := NewMemoryAuthorizer()
	_ = a.RegisterDevice("a", "m", DeviceMobile)
	_ = a.RegisterDevice("a", "d", DeviceDesktop)
	s := NewService(r, a)
	c, e := s.CreateConversation(context.Background(), Actor{"a", "m", DeviceMobile}, "d", "agent")
	if e != nil {
		t.Fatal(e)
	}
	m, _, e := s.SendMessage(context.Background(), Actor{"a", "m", DeviceMobile}, c.ID, MessageInput{ClientMessageID: "x", Operation: "get_recent_intel", Body: "x"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.StartRun(context.Background(), Actor{"a", "m", DeviceMobile}, c.ID, m.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal(e)
	}
	run, e := s.StartRun(context.Background(), Actor{"a", "d", DeviceDesktop}, c.ID, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.UpdateRun(context.Background(), Actor{"a", "d", DeviceDesktop}, run.ID, RunRunning, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CancelRun(context.Background(), Actor{"a", "m", DeviceMobile}, run.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CancelRun(context.Background(), Actor{"a", "m", DeviceMobile}, run.ID); !errors.Is(e, ErrRunNotCancellable) {
		t.Fatal(e)
	}
}
