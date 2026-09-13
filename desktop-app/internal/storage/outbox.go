package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"eve-assistant/desktop-app/internal/protocol"
	"github.com/google/uuid"
	"time"
)

type Outbox struct {
	ID          string
	Envelope    protocol.EventEnvelope
	Attempts    int
	NextAttempt time.Time
}

func EnsureReliableSchema(ctx context.Context, s Store) error {
	_, e := s.Exec(ctx, `CREATE TABLE IF NOT EXISTS outbox (id TEXT PRIMARY KEY, payload TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_attempt_ms INTEGER NOT NULL, acked INTEGER NOT NULL DEFAULT 0); CREATE TABLE IF NOT EXISTS sync_cursor (name TEXT PRIMARY KEY, cursor INTEGER NOT NULL DEFAULT 0); CREATE TABLE IF NOT EXISTS pairing_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_ms INTEGER NOT NULL)`)
	return e
}
func Enqueue(ctx context.Context, s Store, e protocol.EventEnvelope) (string, error) {
	if e.MessageID == "" {
		e.MessageID = uuid.NewString()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	_, err = s.Exec(ctx, "INSERT INTO outbox(id,payload,next_attempt_ms) VALUES(?,?,?)", e.MessageID, b, time.Now().UnixMilli())
	return e.MessageID, err
}
func Ack(ctx context.Context, s Store, id string) error {
	_, e := s.Exec(ctx, "DELETE FROM outbox WHERE id=?", id)
	return e
}
func Pending(ctx context.Context, s Store, limit int) ([]Outbox, error) {
	rows, e := s.Query(ctx, "SELECT id,payload,attempts,next_attempt_ms FROM outbox WHERE acked=0 AND next_attempt_ms<=? ORDER BY next_attempt_ms LIMIT ?", time.Now().UnixMilli(), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Outbox
	for rows.Next() {
		var id, p string
		var a int
		var n int64
		if e = rows.Scan(&id, &p, &a, &n); e != nil {
			return nil, e
		}
		var ev protocol.EventEnvelope
		if e = json.Unmarshal([]byte(p), &ev); e != nil {
			return nil, e
		}
		out = append(out, Outbox{id, ev, a, time.UnixMilli(n)})
	}
	return out, rows.Err()
}
func ScheduleRetry(ctx context.Context, s Store, id string, attempt int, d time.Duration) error {
	_, e := s.Exec(ctx, "UPDATE outbox SET attempts=?,next_attempt_ms=? WHERE id=?", attempt, time.Now().Add(d).UnixMilli(), id)
	return e
}

var _ = sql.ErrNoRows
