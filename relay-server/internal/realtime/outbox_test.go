package realtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeOutbox struct {
	rows      []OutboxRecord
	completed []string
	failed    []string
}

func (f *fakeOutbox) ClaimOutbox(context.Context, int, time.Time, time.Duration) ([]OutboxRecord, error) {
	out := append([]OutboxRecord(nil), f.rows...)
	f.rows = nil
	return out, nil
}
func (f *fakeOutbox) CompleteOutbox(_ context.Context, id, _ string) error {
	f.completed = append(f.completed, id)
	return nil
}
func (f *fakeOutbox) FailOutbox(_ context.Context, id, _, _ string, _ time.Time) error {
	f.failed = append(f.failed, id)
	return nil
}

type fakeOutboxDelivery struct{ events []Event }

func (f *fakeOutboxDelivery) Publish(_ context.Context, a, d string, e Event) (Event, error) {
	e.AccountID = a
	e.DeviceID = d
	f.events = append(f.events, e)
	return e, nil
}
func (f *fakeOutboxDelivery) Replay(context.Context, string, string, int64, int) (ReplayPage, error) {
	return ReplayPage{}, nil
}
func (f *fakeOutboxDelivery) AckCursor(context.Context, string, string, int64, time.Time) error {
	return nil
}
func (f *fakeOutboxDelivery) MarkDelivered(context.Context, string, time.Time) error      { return nil }
func (f *fakeOutboxDelivery) MarkProcessed(context.Context, string, time.Time) error      { return nil }
func (f *fakeOutboxDelivery) MarkFailed(context.Context, string, string, time.Time) error { return nil }
func (f *fakeOutboxDelivery) ClaimPending(context.Context, string, int, time.Time) ([]DeliveryRecord, error) {
	return nil, nil
}
func (f *fakeOutboxDelivery) Prune(context.Context, time.Time, int) error { return nil }
func TestOutboxDispatcherPublishesAndCompletesIdempotently(t *testing.T) {
	f := &fakeOutbox{rows: []OutboxRecord{{ID: "e1", AccountID: "a", TargetDeviceID: "d", Type: "run", Payload: json.RawMessage(`{}`)}}}
	delivery := &fakeOutboxDelivery{}
	n, err := (&OutboxDispatcher{Outbox: f, Delivery: delivery}).Dispatch(context.Background(), 10, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("dispatch n=%d err=%v", n, err)
	}
	if len(delivery.events) != 1 || delivery.events[0].ID != "e1" || len(f.completed) != 1 {
		t.Fatalf("delivery=%#v completed=%v", delivery.events, f.completed)
	}
}
