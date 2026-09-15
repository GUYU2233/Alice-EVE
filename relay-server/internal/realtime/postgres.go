package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeliveryStatus is the durable lifecycle of an event.
type DeliveryStatus string

const (
	DeliveryPersisted DeliveryStatus = "persisted"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryProcessed DeliveryStatus = "processed"
	DeliveryFailed    DeliveryStatus = "failed"
)

type DeliveryRecord struct {
	Event
	SenderDeviceID string
	MessageID      string
	Status         DeliveryStatus
	AttemptCount   int
	LastError      string
	DeliveredAt    *time.Time
	ProcessedAt    *time.Time
	ExpiresAt      *time.Time
}

type DeliveryRepository interface {
	Publish(context.Context, string, string, Event) (Event, error)
	Replay(context.Context, string, string, int64, int) (ReplayPage, error)
	AckCursor(context.Context, string, string, int64, time.Time) error
	MarkDelivered(context.Context, string, time.Time) error
	MarkProcessed(context.Context, string, time.Time) error
	MarkFailed(context.Context, string, string, time.Time) error
	ClaimPending(context.Context, string, int, time.Time) ([]DeliveryRecord, error)
	Prune(context.Context, time.Time, int) error
}

// DeliveryAcker is implemented by durable repositories that can validate an
// event's account/device ownership while applying a protocol ACK. It is kept
// separate from DeliveryRepository so existing repository implementations keep
// compiling unchanged.
type DeliveryAcker interface {
	AckEvent(context.Context, string, string, string, string, int64, time.Time) error
}

type DeliveryCursorReader interface {
	CurrentCursor(context.Context, string, string) (int64, error)
}

type PostgresDeliveryRepository struct{ Pool *pgxpool.Pool }

func NewPostgresDeliveryRepository(pool *pgxpool.Pool) (*PostgresDeliveryRepository, error) {
	if pool == nil {
		return nil, errors.New("realtime: postgres pool is required")
	}
	return &PostgresDeliveryRepository{Pool: pool}, nil
}

// Publish allocates a cursor while holding the per-target cursor row lock.
// Event IDs are idempotency keys, so retries return the original event.
func (r *PostgresDeliveryRepository) Publish(ctx context.Context, accountID, deviceID string, event Event) (Event, error) {
	if err := validateTarget(accountID, deviceID); err != nil {
		return Event{}, err
	}
	if strings.TrimSpace(event.Type) == "" || !json.Valid(event.Payload) {
		return Event{}, ErrInvalid
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	if event.ID == "" {
		event.ID = NewID()
	} else {
		var existing Event
		lookupErr := tx.QueryRow(ctx, `SELECT id,account_id,recipient_device_id,event_type,payload,cursor,created_at FROM delivery_events WHERE id=$1 AND account_id=$2 AND recipient_device_id=$3`, event.ID, accountID, deviceID).Scan(&existing.ID, &existing.AccountID, &existing.DeviceID, &existing.Type, &existing.Payload, &existing.Cursor, &existing.CreatedAt)
		if lookupErr == nil {
			if err := tx.Commit(ctx); err != nil {
				return Event{}, err
			}
			return existing, nil
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return Event{}, lookupErr
		}
	}
	var cursor int64
	err = tx.QueryRow(ctx, `INSERT INTO device_delivery_cursors(account_id,device_id,next_cursor) VALUES($1,$2,1) ON CONFLICT(account_id,device_id) DO UPDATE SET next_cursor=device_delivery_cursors.next_cursor+1 RETURNING next_cursor`, accountID, deviceID).Scan(&cursor)
	if err != nil {
		return Event{}, err
	}
	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	event.AccountID, event.DeviceID, event.Cursor = accountID, deviceID, cursor
	var got Event
	err = tx.QueryRow(ctx, `INSERT INTO delivery_events(id,account_id,recipient_device_id,event_type,payload,cursor,status,created_at) VALUES($1,$2,$3,$4,$5,$6,'persisted',$7) ON CONFLICT(id) DO NOTHING RETURNING id,account_id,recipient_device_id,event_type,payload,cursor,created_at`, event.ID, accountID, deviceID, event.Type, []byte(event.Payload), cursor, event.CreatedAt).Scan(&got.ID, &got.AccountID, &got.DeviceID, &got.Type, &got.Payload, &got.Cursor, &got.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id,account_id,recipient_device_id,event_type,payload,cursor,created_at FROM delivery_events WHERE id=$1 AND account_id=$2 AND recipient_device_id=$3`, event.ID, accountID, deviceID).Scan(&got.ID, &got.AccountID, &got.DeviceID, &got.Type, &got.Payload, &got.Cursor, &got.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			// An ID already owned by another target must never be disclosed or
			// returned as a successful publish for this target.
			return Event{}, ErrInvalid
		}
	}
	if err != nil {
		return Event{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Event{}, err
	}
	return got, nil
}

func (r *PostgresDeliveryRepository) Replay(ctx context.Context, accountID, deviceID string, after int64, limit int) (ReplayPage, error) {
	if err := validateTarget(accountID, deviceID); err != nil {
		return ReplayPage{}, err
	}
	if after < 0 {
		return ReplayPage{}, ErrInvalid
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var first int64
	err := r.Pool.QueryRow(ctx, `SELECT COALESCE(first_available_cursor,1) FROM device_delivery_cursors WHERE account_id=$1 AND device_id=$2`, accountID, deviceID).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		first = 1
	} else if err != nil {
		return ReplayPage{}, err
	}
	if after > 0 && after < first-1 {
		return ReplayPage{}, ErrCursorExpired
	}
	rows, err := r.Pool.Query(ctx, `SELECT id,account_id,recipient_device_id,event_type,payload,cursor,created_at FROM delivery_events WHERE account_id=$1 AND recipient_device_id=$2 AND cursor>$3 AND (expires_at IS NULL OR expires_at>now()) ORDER BY cursor LIMIT $4`, accountID, deviceID, after, limit+1)
	if err != nil {
		return ReplayPage{}, err
	}
	defer rows.Close()
	items := make([]Event, 0, limit)
	fetched := 0
	for rows.Next() {
		fetched++
		var e Event
		if err := rows.Scan(&e.ID, &e.AccountID, &e.DeviceID, &e.Type, &e.Payload, &e.Cursor, &e.CreatedAt); err != nil {
			return ReplayPage{}, err
		}
		if len(items) < limit {
			items = append(items, e)
		}
	}
	if err := rows.Err(); err != nil {
		return ReplayPage{}, err
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Cursor
	}
	return ReplayPage{Events: items, NextCursor: next, HasMore: fetched > limit}, nil
}

func (r *PostgresDeliveryRepository) CurrentCursor(ctx context.Context, accountID, deviceID string) (int64, error) {
	var cursor int64
	err := r.Pool.QueryRow(ctx, `SELECT COALESCE(next_cursor,0) FROM device_delivery_cursors WHERE account_id=$1 AND device_id=$2`, accountID, deviceID).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return cursor, err
}

func (r *PostgresDeliveryRepository) AckCursor(ctx context.Context, accountID, deviceID string, cursor int64, at time.Time) error {
	// Legacy callers may provide a cursor, but they must not be able to skip
	// unacknowledged events. Restrict advancement to the contiguous ACK prefix.
	if err := validateTarget(accountID, deviceID); err != nil {
		return err
	}
	if cursor < 0 {
		return ErrInvalid
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := r.advanceCursorTx(ctx, tx, accountID, deviceID, cursor, at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AckEvent validates that eventID belongs to the authenticated target before
// advancing the durable cursor. Status controls lifecycle timestamps and is
// monotonic (a processed ACK cannot be regressed by a delivered ACK).
func (r *PostgresDeliveryRepository) advanceCursorTx(ctx context.Context, tx pgx.Tx, accountID, deviceID string, requested int64, at time.Time) error {
	var current int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(last_acked_cursor,0) FROM device_delivery_cursors WHERE account_id=$1 AND device_id=$2 FOR UPDATE`, accountID, deviceID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		return err
	}
	if requested <= current {
		return nil
	}
	var contiguous int64 = current
	rows, err := tx.Query(ctx, `SELECT e.cursor FROM delivery_events e WHERE e.account_id=$1 AND e.recipient_device_id=$2 AND e.cursor>$3 AND e.cursor<=$4 AND EXISTS (SELECT 1 FROM realtime_event_acks a WHERE a.account_id=e.account_id AND a.device_id=e.recipient_device_id AND a.event_id=e.id AND a.status IN ('received','processed','failed')) ORDER BY e.cursor`, accountID, deviceID, current, requested)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c int64
		if err := rows.Scan(&c); err != nil {
			return err
		}
		if c != contiguous+1 {
			break
		}
		contiguous = c
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if contiguous == current {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE device_delivery_cursors SET last_acked_cursor=$3,updated_at=$4 WHERE account_id=$1 AND device_id=$2 AND last_acked_cursor<$3`, accountID, deviceID, contiguous, at)
	return err
}

func (r *PostgresDeliveryRepository) AckEvent(ctx context.Context, accountID, deviceID, eventID, status string, cursor int64, at time.Time) error {
	if err := validateTarget(accountID, deviceID); err != nil || strings.TrimSpace(eventID) == "" {
		return ErrInvalid
	}
	if cursor < 0 {
		return ErrInvalid
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var eventCursor int64
	if err := tx.QueryRow(ctx, `SELECT cursor FROM delivery_events WHERE id=$1 AND account_id=$2 AND recipient_device_id=$3 FOR UPDATE`, eventID, accountID, deviceID).Scan(&eventCursor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalid
		}
		return err
	}
	if cursor == 0 {
		cursor = eventCursor
	}
	if cursor != eventCursor {
		return ErrInvalid
	}
	var update string
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "received", "persisted", "delivered":
		update = `UPDATE delivery_events SET status=CASE WHEN status='processed' THEN status ELSE 'delivered' END, delivered_at=COALESCE(delivered_at,$2) WHERE id=$1 AND account_id=$3 AND recipient_device_id=$4`
	case "processed":
		update = `UPDATE delivery_events SET status='processed', delivered_at=COALESCE(delivered_at,$2), processed_at=COALESCE(processed_at,$2) WHERE id=$1 AND account_id=$3 AND recipient_device_id=$4`
	case "failed":
		update = `UPDATE delivery_events SET status='failed', attempt_count=attempt_count+1 WHERE id=$1 AND account_id=$3 AND recipient_device_id=$4`
	default:
		return ErrInvalid
	}
	if _, err := r.Pool.Exec(ctx, update, eventID, at, accountID, deviceID); err != nil {
		return err
	}
	ackStatus := strings.ToLower(strings.TrimSpace(status))
	if ackStatus == "accepted" || ackStatus == "persisted" || ackStatus == "delivered" {
		ackStatus = "received"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO realtime_event_acks(account_id,device_id,event_id,cursor,status,acknowledged_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(account_id,device_id,event_id) DO UPDATE SET cursor=EXCLUDED.cursor,status=CASE WHEN realtime_event_acks.status='processed' THEN realtime_event_acks.status WHEN EXCLUDED.status='processed' THEN EXCLUDED.status ELSE realtime_event_acks.status END,acknowledged_at=EXCLUDED.acknowledged_at`, accountID, deviceID, eventID, cursor, ackStatus, at); err != nil {
		return err
	}
	if err := r.advanceCursorTx(ctx, tx, accountID, deviceID, cursor, at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *PostgresDeliveryRepository) MarkDelivered(ctx context.Context, id string, at time.Time) error {
	return r.mark(ctx, id, "delivered_at", at)
}
func (r *PostgresDeliveryRepository) MarkProcessed(ctx context.Context, id string, at time.Time) error {
	return r.mark(ctx, id, "processed_at", at)
}
func (r *PostgresDeliveryRepository) mark(ctx context.Context, id, column string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	query := `UPDATE delivery_events SET status=CASE WHEN status='processed' THEN status ELSE $2 END,` + column + `=COALESCE(` + column + `,$3) WHERE id=$1 AND status <> 'processed'`
	_, err := r.Pool.Exec(ctx, query, id, columnStatus(column), at)
	return err
}
func columnStatus(column string) string {
	if column == "processed_at" {
		return "processed"
	}
	return "delivered"
}
func (r *PostgresDeliveryRepository) MarkFailed(ctx context.Context, id, last string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := r.Pool.Exec(ctx, `UPDATE delivery_events SET status=CASE WHEN status='processed' THEN status ELSE 'failed' END,attempt_count=attempt_count+1,last_error=$2 WHERE id=$1 AND status <> 'processed'`, id, last)
	return err
}
func (r *PostgresDeliveryRepository) ClaimPending(ctx context.Context, deviceID string, limit int, now time.Time) ([]DeliveryRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,account_id,recipient_device_id,event_type,payload,cursor,created_at,COALESCE(sender_device_id,''),COALESCE(message_id,''),status,attempt_count,COALESCE(last_error,''),delivered_at,processed_at,expires_at FROM delivery_events WHERE recipient_device_id=$1 AND status IN ('persisted','failed') AND (expires_at IS NULL OR expires_at>$2) ORDER BY cursor LIMIT $3 FOR UPDATE SKIP LOCKED`, deviceID, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeliveryRecord{}
	for rows.Next() {
		var d DeliveryRecord
		if err := rows.Scan(&d.ID, &d.AccountID, &d.DeviceID, &d.Type, &d.Payload, &d.Cursor, &d.CreatedAt, &d.SenderDeviceID, &d.MessageID, &d.Status, &d.AttemptCount, &d.LastError, &d.DeliveredAt, &d.ProcessedAt, &d.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func (r *PostgresDeliveryRepository) Prune(ctx context.Context, before time.Time, limit int) error {
	if before.IsZero() {
		before = time.Now().UTC().Add(-24 * time.Hour)
	}
	if limit <= 0 {
		limit = 1000
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM delivery_events WHERE id IN (SELECT id FROM delivery_events WHERE (expires_at IS NOT NULL AND expires_at<$1) OR (status='processed' AND created_at<$1) ORDER BY cursor LIMIT $2)`, before, limit); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE device_delivery_cursors c SET first_available_cursor=COALESCE((SELECT MIN(e.cursor) FROM delivery_events e WHERE e.account_id=c.account_id AND e.recipient_device_id=c.device_id), c.next_cursor+1) WHERE EXISTS (SELECT 1 FROM delivery_events e WHERE e.account_id=c.account_id AND e.recipient_device_id=c.device_id) OR c.first_available_cursor IS NOT NULL`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var _ DeliveryRepository = (*PostgresDeliveryRepository)(nil)
