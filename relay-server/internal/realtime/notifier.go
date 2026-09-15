package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultWakeupChannel is intentionally separate from the delivery tables:
// PostgreSQL notifications are only a hint that durable rows may have changed.
const DefaultWakeupChannel = "relay_realtime_wakeup"

// Wakeup identifies a durable delivery target whose rows may have changed.
// Empty fields are treated as a broadcast wakeup by Hub.
type Wakeup struct {
	AccountID string `json:"accountId,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
}

// Notifier is a best-effort cross-process wakeup transport. Implementations
// must never be used as durable storage: a missed notification is recovered by
// replaying the DeliveryRepository from the subscriber's last known cursor.
type Notifier interface {
	Notify(context.Context, Wakeup) error
	Wakeups() <-chan Wakeup
	Close() error
}

// PostgresNotifier uses one dedicated connection for LISTEN and the pool for
// pg_notify. Notifications are hints only; delivery rows remain authoritative.
type PostgresNotifier struct {
	pool    *pgxpool.Pool
	channel string
	conn    *pgxpool.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	wakeups chan Wakeup
	done    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

func NewPostgresNotifier(ctx context.Context, pool *pgxpool.Pool, channel ...string) (*PostgresNotifier, error) {
	if pool == nil {
		return nil, errors.New("realtime: postgres pool is required")
	}
	name := DefaultWakeupChannel
	if len(channel) > 0 && strings.TrimSpace(channel[0]) != "" {
		name = strings.TrimSpace(channel[0])
	}
	if !validNotifyChannel(name) {
		return nil, fmt.Errorf("realtime: invalid postgres notification channel %q", name)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	// LISTEN cannot use a query parameter, so quote the validated identifier.
	if _, err := conn.Exec(ctx, "LISTEN \""+name+"\""); err != nil {
		conn.Release()
		return nil, err
	}
	listenerCtx, cancel := context.WithCancel(context.Background())
	n := &PostgresNotifier{
		pool: pool, channel: name, conn: conn, ctx: listenerCtx, cancel: cancel,
		wakeups: make(chan Wakeup, 128), done: make(chan struct{}),
	}
	n.wg.Add(1)
	go n.listen()
	return n, nil
}

func validNotifyChannel(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (n *PostgresNotifier) Notify(ctx context.Context, wakeup Wakeup) error {
	if n == nil || n.pool == nil {
		return errors.New("realtime: notifier is unavailable")
	}
	payload, err := json.Marshal(wakeup)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err = n.pool.Exec(ctx, `SELECT pg_notify($1,$2)`, n.channel, string(payload))
	return err
}

func (n *PostgresNotifier) Wakeups() <-chan Wakeup { return n.wakeups }

func (n *PostgresNotifier) listen() {
	defer n.wg.Done()
	defer close(n.wakeups)
	defer close(n.done)
	for {
		notification, err := n.conn.Conn().WaitForNotification(n.ctx)
		if err != nil {
			return
		}
		var wakeup Wakeup
		if err := json.Unmarshal([]byte(notification.Payload), &wakeup); err != nil {
			// A malformed hint should not stop delivery for this process.
			wakeup = Wakeup{}
		}
		select {
		case n.wakeups <- wakeup:
		case <-n.ctx.Done():
			return
		default:
			// Coalesce when a process is busy replaying. Durable replay makes
			// dropping a hint safe and prevents an unbounded notifier backlog.
		}
	}
}

func (n *PostgresNotifier) Close() error {
	if n == nil {
		return nil
	}
	n.once.Do(func() {
		n.cancel()
		n.wg.Wait()
		n.conn.Release()
	})
	return nil
}

var _ Notifier = (*PostgresNotifier)(nil)
