package realtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// leaseTestOutbox models the atomic claim/conditional completion contract used
// by the PostgreSQL repository. It lets us exercise stale-worker behavior
// without requiring a running PostgreSQL instance.
type leaseTestOutbox struct {
	mu       sync.Mutex
	pending  bool
	token    string
	claims   int
	leaseDur time.Duration
}

func (f *leaseTestOutbox) ClaimOutbox(_ context.Context, _ int, _ time.Time, lease time.Duration) ([]OutboxRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.pending {
		return nil, nil
	}
	f.pending = false
	f.claims++
	f.leaseDur = lease
	f.token = "claim-" + string(rune('0'+f.claims))
	return []OutboxRecord{{ID: "event-1", AccountID: "a", TargetDeviceID: "d", Type: "event", ClaimToken: f.token}}, nil
}

func (f *leaseTestOutbox) CompleteOutbox(_ context.Context, id, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != "event-1" || token != f.token {
		return ErrOutboxLeaseLost
	}
	return nil
}

func (f *leaseTestOutbox) FailOutbox(_ context.Context, id, token, _ string, _ time.Time) error {
	return f.CompleteOutbox(context.Background(), id, token)
}

func TestOutboxDispatcherPassesConfiguredLeaseAndClaimToken(t *testing.T) {
	outbox := &leaseTestOutbox{pending: true}
	delivery := &fakeOutboxDelivery{}
	lease := 17 * time.Second
	if _, err := (&OutboxDispatcher{Outbox: outbox, Delivery: delivery, Lease: lease}).Dispatch(context.Background(), 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if outbox.leaseDur != lease {
		t.Fatalf("lease=%s, want %s", outbox.leaseDur, lease)
	}
}

func TestOutboxStaleWorkerCannotCompleteReclaimedClaim(t *testing.T) {
	outbox := &leaseTestOutbox{pending: true}
	first, err := outbox.ClaimOutbox(context.Background(), 1, time.Now(), time.Second)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim=%#v err=%v", first, err)
	}
	// Simulate lease expiry/requeue and another worker reclaiming the row.
	outbox.mu.Lock()
	outbox.pending = true
	outbox.mu.Unlock()
	second, err := outbox.ClaimOutbox(context.Background(), 1, time.Now().Add(2*time.Second), time.Second)
	if err != nil || len(second) != 1 {
		t.Fatalf("second claim=%#v err=%v", second, err)
	}
	if err := outbox.CompleteOutbox(context.Background(), first[0].ID, first[0].ClaimToken); !errors.Is(err, ErrOutboxLeaseLost) {
		t.Fatalf("stale completion err=%v, want ErrOutboxLeaseLost", err)
	}
	if err := outbox.CompleteOutbox(context.Background(), second[0].ID, second[0].ClaimToken); err != nil {
		t.Fatalf("current completion err=%v", err)
	}
}

func TestOutboxConcurrentClaimsOnlyOneWorkerGetsRow(t *testing.T) {
	outbox := &leaseTestOutbox{pending: true}
	start := make(chan struct{})
	results := make(chan []OutboxRecord, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			rows, _ := outbox.ClaimOutbox(context.Background(), 1, time.Now(), time.Minute)
			results <- rows
		}()
	}
	close(start)
	var claimed int
	for i := 0; i < 2; i++ {
		claimed += len(<-results)
	}
	if claimed != 1 {
		t.Fatalf("claimed=%d, want exactly one", claimed)
	}
}
