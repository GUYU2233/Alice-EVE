package realtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testAuthorizer struct{ allowed map[string]bool }

func (a testAuthorizer) AuthorizeDevice(_ context.Context, accountID, deviceID string) error {
	if !a.allowed[accountID+"/"+deviceID] {
		return ErrUnauthorized
	}
	return nil
}

func TestHubBroadcastsToIndependentDeviceSubscribers(t *testing.T) {
	authz := testAuthorizer{allowed: map[string]bool{"acct/a": true, "acct/b": true}}
	hub := NewHub(authz)
	ctx := context.Background()
	a, err := hub.Subscribe(ctx, "acct", "a", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hub.Subscribe(ctx, "acct", "b", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()
	if _, err := hub.Publish(ctx, "acct", "a", Event{Type: "one", Payload: []byte(`{"value":1}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Publish(ctx, "acct", "b", Event{Type: "two", Payload: []byte(`{"value":2}`)}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-a.Events():
		if event.DeviceID != "a" || event.Cursor != 1 || event.Type != "one" {
			t.Fatalf("unexpected a event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("a did not receive event")
	}
	select {
	case event := <-b.Events():
		if event.DeviceID != "b" || event.Cursor != 1 || event.Type != "two" {
			t.Fatalf("unexpected b event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("b did not receive event")
	}
	select {
	case event := <-a.Events():
		t.Fatalf("a received b event: %+v", event)
	default:
	}
}

func TestHubReplayAndCursorExpiry(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := hub.Publish(ctx, "acct", "device", Event{Type: "event"}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := hub.Replay(ctx, "acct", "device", 1, 1)
	if err != nil || len(page.Events) != 1 || page.Events[0].Cursor != 2 || !page.HasMore || page.NextCursor != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, err = hub.Replay(ctx, "acct", "device", page.NextCursor, 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].Cursor != 3 || page.HasMore {
		t.Fatalf("second page=%+v err=%v", page, err)
	}
	limited := NewHubWithConfig(Config{RetentionPerDevice: 2})
	for i := 0; i < 3; i++ {
		if _, err := limited.Publish(ctx, "acct", "device", Event{Type: "event"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := limited.Replay(ctx, "acct", "device", 0, 10); !errors.Is(err, ErrCursorExpired) {
		t.Fatalf("expected expired cursor, got %v", err)
	}
}

type fakeDelivery struct{ events []Event }

func (f *fakeDelivery) Publish(_ context.Context, accountID, deviceID string, event Event) (Event, error) {
	for _, prior := range f.events {
		if prior.ID != "" && prior.ID == event.ID {
			return prior, nil
		}
	}
	if event.ID == "" {
		event.ID = "durable-id"
	}
	event.AccountID, event.DeviceID = accountID, deviceID
	event.Cursor = int64(len(f.events) + 1)
	f.events = append(f.events, event)
	return event, nil
}
func (f *fakeDelivery) Replay(_ context.Context, accountID, deviceID string, after int64, limit int) (ReplayPage, error) {
	out := make([]Event, 0, limit)
	for _, event := range f.events {
		if event.AccountID == accountID && event.DeviceID == deviceID && event.Cursor > after && len(out) < limit {
			out = append(out, event)
		}
	}
	return ReplayPage{Events: out, NextCursor: after, HasMore: len(out) == limit}, nil
}
func (f *fakeDelivery) AckCursor(context.Context, string, string, int64, time.Time) error { return nil }
func (f *fakeDelivery) MarkDelivered(context.Context, string, time.Time) error            { return nil }
func (f *fakeDelivery) MarkProcessed(context.Context, string, time.Time) error            { return nil }
func (f *fakeDelivery) MarkFailed(context.Context, string, string, time.Time) error       { return nil }
func (f *fakeDelivery) ClaimPending(context.Context, string, int, time.Time) ([]DeliveryRecord, error) {
	return nil, nil
}
func (f *fakeDelivery) Prune(context.Context, time.Time, int) error { return nil }

func TestHubDurableRepositoryPublishesAndReplays(t *testing.T) {
	repo := &fakeDelivery{}
	hub := NewHubWithDeliveryRepository(repo)
	ctx := context.Background()
	if _, err := hub.Publish(ctx, "acct", "device", Event{ID: "event-1", Type: "one"}); err != nil {
		t.Fatal(err)
	}
	page, err := hub.Replay(ctx, "acct", "device", 0, 10)
	if err != nil || len(page.Events) != 1 || page.Events[0].ID != "event-1" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err := hub.Publish(ctx, "acct", "device", Event{ID: "event-1", Type: "one"}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("duplicate durable events=%d", len(repo.events))
	}
}

func TestSubscriptionCursorForUsesDeliveredServerCursor(t *testing.T) {
	hub := NewHub()
	sub, err := hub.Subscribe(context.Background(), "acct", "device", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if _, err := hub.Publish(context.Background(), "acct", "device", Event{ID: "event-1", Type: "one"}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-sub.Events():
		if event.Cursor != 1 || sub.CursorFor(event.ID) != event.Cursor {
			t.Fatalf("event=%+v resolved=%d", event, sub.CursorFor(event.ID))
		}
	case <-time.After(time.Second):
		t.Fatal("event was not delivered")
	}
	if sub.CursorFor("missing") != 0 {
		t.Fatal("missing event unexpectedly had a cursor")
	}
}
func TestHubBackpressureClosesOnlySlowSubscriber(t *testing.T) {
	hub := NewHubWithConfig(Config{QueueSize: 1})
	ctx := context.Background()
	slow, err := hub.Subscribe(ctx, "acct", "device", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if _, err := hub.Publish(ctx, "acct", "device", Event{Type: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Publish(ctx, "acct", "device", Event{Type: "second"}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(slow.Err(), ErrBackpressure) {
		t.Fatalf("subscriber error=%v", slow.Err())
	}
	if _, err := hub.Publish(ctx, "acct", "other", Event{Type: "other"}); err != nil {
		t.Fatal(err)
	}
}

type fakeNotifier struct{ wakeups chan Wakeup }

func newFakeNotifier() *fakeNotifier { return &fakeNotifier{wakeups: make(chan Wakeup, 8)} }
func (n *fakeNotifier) Notify(_ context.Context, wakeup Wakeup) error {
	n.wakeups <- wakeup
	return nil
}
func (n *fakeNotifier) Wakeups() <-chan Wakeup { return n.wakeups }
func (n *fakeNotifier) Close() error           { close(n.wakeups); return nil }

func TestHubReplaysDurableEventsAfterWakeup(t *testing.T) {
	repo := &fakeDelivery{}
	notifier := newFakeNotifier()
	hub := NewHubWithConfigRepositoryAndNotifier(DefaultConfig(), repo, notifier)
	defer hub.Close()
	sub, err := hub.Subscribe(context.Background(), "acct", "device", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	repo.events = append(repo.events, Event{ID: "remote", AccountID: "acct", DeviceID: "device", Cursor: 1, Type: "remote", Payload: []byte(`{}`)})
	notifier.wakeups <- Wakeup{AccountID: "acct", DeviceID: "device"}
	select {
	case event := <-sub.Events():
		if event.ID != "remote" || event.Cursor != 1 {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("durable event was not replayed after wakeup")
	}
}
func TestHubCloseWithoutNotifierIsSafe(t *testing.T) {
	hub := NewHub()
	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}
	if err := hub.Close(); err != nil {
		t.Fatal(err)
	}
}
