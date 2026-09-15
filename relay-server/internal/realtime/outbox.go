package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// OutboxRecord is intentionally structural so realtime does not depend on the
// conversations package (which would create an import cycle).
type OutboxRecord struct {
	ID             string
	AccountID      string
	TargetDeviceID string
	Type           string
	Payload        json.RawMessage
	CreatedAt      time.Time
	AttemptCount   int
	// ClaimToken identifies the lease that authorized processing this row. It
	// is required for conditional completion/failure so stale workers cannot
	// mutate a row reclaimed by another worker.
	ClaimToken string
}

var ErrOutboxLeaseLost = errors.New("realtime: outbox lease lost")

// DefaultOutboxLease bounds how long a worker may process a claimed row.
const DefaultOutboxLease = 5 * time.Minute

type OutboxRepository interface {
	ClaimOutbox(context.Context, int, time.Time, time.Duration) ([]OutboxRecord, error)
	CompleteOutbox(context.Context, string, string) error
	FailOutbox(context.Context, string, string, string, time.Time) error
}

// OutboxDispatcher materializes committed realtime intents into delivery_events.
// Publish is idempotent by event ID; a crash between Publish and Complete is
// therefore safe and will not create a duplicate delivery event.
type OutboxDispatcher struct {
	Outbox   OutboxRepository
	Delivery DeliveryRepository
	Notifier Notifier
	Lease    time.Duration
}

func (d *OutboxDispatcher) Dispatch(ctx context.Context, limit int, now time.Time) (int, error) {
	if d == nil || d.Outbox == nil || d.Delivery == nil {
		return 0, errors.New("realtime: outbox dispatcher dependencies required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lease := d.Lease
	if lease <= 0 {
		lease = DefaultOutboxLease
	}
	rows, err := d.Outbox.ClaimOutbox(ctx, limit, now, lease)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, row := range rows {
		_, publishErr := d.Delivery.Publish(ctx, row.AccountID, row.TargetDeviceID, Event{ID: row.ID, AccountID: row.AccountID, DeviceID: row.TargetDeviceID, Type: row.Type, Payload: append(json.RawMessage(nil), row.Payload...), CreatedAt: row.CreatedAt})
		if publishErr != nil {
			_ = d.Outbox.FailOutbox(ctx, row.ID, row.ClaimToken, publishErr.Error(), now)
			continue
		}
		if err := d.Outbox.CompleteOutbox(ctx, row.ID, row.ClaimToken); err != nil {
			// A lease can expire while publishing; another worker may reclaim the
			// row and complete it. The delivery write is idempotent by event ID,
			// so losing this worker's lease is safe and should not stop the batch.
			if errors.Is(err, ErrOutboxLeaseLost) {
				continue
			}
			return completed, err
		}
		// Notify only after durable materialization and outbox completion. A
		// notification is still a hint; failures are intentionally ignored.
		if d.Notifier != nil {
			_ = d.Notifier.Notify(ctx, Wakeup{AccountID: row.AccountID, DeviceID: row.TargetDeviceID})
		}
		completed++
	}
	return completed, nil
}
