// Package realtime provides an in-process delivery hub with durable cursor
// replay semantics. It is deliberately independent of HTTP/WebSocket types:
// adapters can expose Subscription.Events as SSE or WSS frames.
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized  = errors.New("realtime: unauthorized device")
	ErrInvalid       = errors.New("realtime: invalid event or device")
	ErrTooLarge      = errors.New("realtime: event is too large")
	ErrRateLimited   = errors.New("realtime: publish rate limit exceeded")
	ErrBackpressure  = errors.New("realtime: subscriber queue is full")
	ErrCursorExpired = errors.New("realtime: cursor is no longer available")
)

// RateLimitError exposes a retry hint and can be checked with errors.Is.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", ErrRateLimited, e.RetryAfter.Round(time.Millisecond))
}
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// DeviceAuthorizer prevents a caller from subscribing/publishing into a
// different account's device queue. It should derive authorization from the
// caller's authenticated session in an HTTP/WSS adapter.
type DeviceAuthorizer interface {
	AuthorizeDevice(context.Context, string, string) error
}

// Event is a target-device event. Cursor is assigned by Hub and increases per
// account/device target, not globally; this makes replay deterministic without
// imposing an unrelated global ordering across devices.
type Event struct {
	ID        string          `json:"id"`
	AccountID string          `json:"accountId"`
	DeviceID  string          `json:"deviceId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Cursor    int64           `json:"cursor"`
	CreatedAt time.Time       `json:"createdAt"`
}

type ErrorCode string

const (
	ErrorUnauthorized  ErrorCode = "unauthorized"
	ErrorForbidden     ErrorCode = "forbidden"
	ErrorCursorExpired ErrorCode = "cursor_expired"
	ErrorBackpressure  ErrorCode = "backpressure"
	ErrorProtocol      ErrorCode = "protocol_error"
)

type ProtocolError struct {
	Code       ErrorCode
	Message    string
	RetryAfter time.Duration
}

func (e *ProtocolError) Error() string { return e.Message }

// ReplayPage is used by HTTPS sync and by a reconnecting WebSocket before it
// subscribes. A page's next cursor is safe to persist as the last applied
// event cursor.
type ReplayPage struct {
	Events     []Event
	NextCursor int64
	HasMore    bool
}

// DeliveryHub is the integration seam for handlers. A new Subscribe call has
// its own queue and receives a copy of each target event; subscribers never
// consume from a shared global channel.
type DeliveryHub interface {
	Publish(context.Context, string, string, Event) (Event, error)
	Subscribe(context.Context, string, string, int64) (*Subscription, error)
	Replay(context.Context, string, string, int64, int) (ReplayPage, error)
}

type Config struct {
	QueueSize          int
	MaxEventBytes      int
	EventsPerWindow    int
	RateWindow         time.Duration
	RetentionPerDevice int
	Now                func() time.Time
}

// DeliveryRepository is an optional durable backing store. When configured,
// publishes and replays go through the repository while the Hub still owns
// in-process subscriptions and fan-out. A nil repository preserves the
// original memory-only behavior used by tests and local development.

func DefaultConfig() Config {
	return Config{QueueSize: 64, MaxEventBytes: 256 * 1024, EventsPerWindow: 120, RateWindow: time.Minute, RetentionPerDevice: 1000, Now: time.Now}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	if c.QueueSize <= 0 {
		c.QueueSize = d.QueueSize
	}
	if c.MaxEventBytes <= 0 {
		c.MaxEventBytes = d.MaxEventBytes
	}
	if c.EventsPerWindow <= 0 {
		c.EventsPerWindow = d.EventsPerWindow
	}
	if c.RateWindow <= 0 {
		c.RateWindow = d.RateWindow
	}
	if c.RetentionPerDevice <= 0 {
		c.RetentionPerDevice = d.RetentionPerDevice
	}
	if c.Now == nil {
		c.Now = d.Now
	}
	return c
}

type targetQueue struct {
	nextCursor int64
	events     []Event
	subs       map[uint64]*Subscription
}

type Hub struct {
	mu         sync.Mutex
	cfg        Config
	authorizer DeviceAuthorizer
	delivery   DeliveryRepository
	notifier   Notifier
	queues     map[string]*targetQueue
	nextSubID  uint64
	limiter    *rateLimiter
	stop       chan struct{}
	closeOnce  sync.Once
	wg         sync.WaitGroup
}

func NewHub(authorizer ...DeviceAuthorizer) *Hub {
	return NewHubWithConfigAndRepository(DefaultConfig(), nil, authorizer...)
}

func NewHubWithConfig(cfg Config, authorizer ...DeviceAuthorizer) *Hub {
	return NewHubWithConfigAndRepository(cfg, nil, authorizer...)
}

// NewHubWithDeliveryRepository creates a hub that persists events and cursor
// replay through delivery. The repository is optional so callers can retain
// the original memory-only constructors and tests.
func NewHubWithDeliveryRepository(repository DeliveryRepository, authorizer ...DeviceAuthorizer) *Hub {
	return NewHubWithConfigAndRepository(DefaultConfig(), repository, authorizer...)
}

// NewHubWithConfigAndRepository is the fully configurable constructor.
func NewHubWithConfigAndRepository(cfg Config, repository DeliveryRepository, authorizer ...DeviceAuthorizer) *Hub {
	return newHub(cfg, repository, nil, authorizer...)
}

// NewHubWithConfigRepositoryAndNotifier configures a best-effort cross-process
// wakeup source. Notifications never replace durable replay.
func NewHubWithConfigRepositoryAndNotifier(cfg Config, repository DeliveryRepository, notifier Notifier, authorizer ...DeviceAuthorizer) *Hub {
	return newHub(cfg, repository, notifier, authorizer...)
}

func newHub(cfg Config, repository DeliveryRepository, notifier Notifier, authorizer ...DeviceAuthorizer) *Hub {
	cfg = cfg.withDefaults()
	var a DeviceAuthorizer
	if len(authorizer) > 0 {
		a = authorizer[0]
	}
	h := &Hub{cfg: cfg, authorizer: a, delivery: repository, notifier: notifier, queues: make(map[string]*targetQueue), limiter: &rateLimiter{now: cfg.Now}, stop: make(chan struct{})}
	if notifier != nil {
		h.wg.Add(1)
		go h.consumeWakeups()
	}
	return h
}

// Delivery returns the durable repository, if one was configured.
func (h *Hub) Delivery() DeliveryRepository { return h.delivery }

// Close stops the notifier consumer and releases notifier resources. It is
// safe to call repeatedly and is a no-op for memory-only hubs.
func (h *Hub) Close() error {
	var err error
	h.closeOnce.Do(func() {
		close(h.stop)
		if h.notifier != nil {
			err = h.notifier.Close()
		}
		h.wg.Wait()
	})
	return err
}

func (h *Hub) Publish(ctx context.Context, accountID, deviceID string, event Event) (Event, error) {
	if err := validateTarget(accountID, deviceID); err != nil {
		return Event{}, err
	}
	if h.authorizer != nil {
		if err := h.authorizer.AuthorizeDevice(ctx, accountID, deviceID); err != nil {
			return Event{}, err
		}
	}
	if strings.TrimSpace(event.Type) == "" {
		return Event{}, ErrInvalid
	}
	if len(event.Payload) > h.cfg.MaxEventBytes || eventSize(event) > h.cfg.MaxEventBytes {
		return Event{}, ErrTooLarge
	}
	if err := h.limiter.allow(accountID+"/"+deviceID, h.cfg.EventsPerWindow, h.cfg.RateWindow); err != nil {
		return Event{}, err
	}
	if h.delivery != nil {
		// The repository allocates the durable per-target cursor and performs
		// idempotency before this process fans the event out to subscribers.
		persisted, err := h.delivery.Publish(ctx, accountID, deviceID, event)
		if err != nil {
			return Event{}, err
		}
		event = persisted
		if h.notifier != nil {
			// NOTIFY is an optimization only. A failed or dropped hint must not
			// make a durable publish fail; subscribers recover through Replay.
			_ = h.notifier.Notify(ctx, Wakeup{AccountID: accountID, DeviceID: deviceID})
		}
	}
	now := h.cfg.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.queueLocked(accountID, deviceID)
	if event.ID == "" {
		event.ID = NewID()
	}
	event.AccountID = accountID
	event.DeviceID = deviceID
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	// A durable retry returns the original event. Do not broadcast it twice.
	for _, prior := range q.events {
		if prior.ID == event.ID {
			return cloneEvent(prior), nil
		}
	}
	if event.Cursor <= 0 {
		event.Cursor = q.nextCursor + 1
	}
	if event.Cursor > q.nextCursor {
		q.nextCursor = event.Cursor
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	q.events = append(q.events, cloneEvent(event))
	if len(q.events) > h.cfg.RetentionPerDevice {
		q.events = q.events[len(q.events)-h.cfg.RetentionPerDevice:]
	}
	for id, sub := range q.subs {
		if !sub.sendLocked(event) {
			delete(q.subs, id)
			sub.closeOnce.Do(func() { sub.closeLocked(ErrBackpressure) })
		}
	}
	return cloneEvent(event), nil
}

// Replay returns events strictly after after. A cursor older than retained
// history is rejected, allowing callers to fall back to a full conversation
// sync instead of silently missing events.
func (h *Hub) Replay(ctx context.Context, accountID, deviceID string, after int64, limit int) (ReplayPage, error) {
	if err := validateTarget(accountID, deviceID); err != nil {
		return ReplayPage{}, err
	}
	if h.authorizer != nil {
		if err := h.authorizer.AuthorizeDevice(ctx, accountID, deviceID); err != nil {
			return ReplayPage{}, err
		}
	}
	if after < 0 {
		return ReplayPage{}, ErrInvalid
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if h.delivery != nil {
		return h.delivery.Replay(ctx, accountID, deviceID, after, limit)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.queueLocked(accountID, deviceID)
	if len(q.events) > 0 && after < q.events[0].Cursor-1 {
		return ReplayPage{}, ErrCursorExpired
	}
	items := make([]Event, 0, limit)
	for _, event := range q.events {
		if event.Cursor > after {
			items = append(items, cloneEvent(event))
			if len(items) == limit {
				break
			}
		}
	}
	hasMore := false
	if len(items) > 0 {
		last := items[len(items)-1].Cursor
		for _, event := range q.events {
			if event.Cursor > last {
				hasMore = true
				break
			}
		}
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Cursor
	}
	return ReplayPage{Events: items, NextCursor: next, HasMore: hasMore}, nil
}

// Subscribe registers a target-device connection. Existing events after
// cursor are copied into this connection's private queue before the method
// returns; subsequent Publish calls broadcast to this same queue.
func (h *Hub) Subscribe(ctx context.Context, accountID, deviceID string, after int64) (*Subscription, error) {
	if err := validateTarget(accountID, deviceID); err != nil {
		return nil, err
	}
	if h.authorizer != nil {
		if err := h.authorizer.AuthorizeDevice(ctx, accountID, deviceID); err != nil {
			return nil, err
		}
	}
	if after < 0 {
		return nil, ErrInvalid
	}
	var replay []Event
	if h.delivery != nil {
		page, err := h.delivery.Replay(ctx, accountID, deviceID, after, h.cfg.QueueSize+1)
		if err != nil {
			return nil, err
		}
		if len(page.Events) > h.cfg.QueueSize || page.HasMore {
			return nil, ErrBackpressure
		}
		replay = page.Events
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.queueLocked(accountID, deviceID)
	if h.delivery == nil {
		if len(q.events) > 0 && after < q.events[0].Cursor-1 {
			return nil, ErrCursorExpired
		}
		replay = make([]Event, 0, h.cfg.QueueSize)
		for _, event := range q.events {
			if event.Cursor > after {
				replay = append(replay, cloneEvent(event))
			}
		}
	}
	if len(replay) > h.cfg.QueueSize {
		return nil, ErrBackpressure
	}
	h.nextSubID++
	sub := newSubscription(h, h.nextSubID, accountID, deviceID, h.cfg.QueueSize)
	for _, event := range replay {
		sub.ch <- cloneEvent(event)
		sub.cursors[event.ID] = event.Cursor
		if event.Cursor > q.nextCursor {
			q.nextCursor = event.Cursor
		}
	}
	q.subs[sub.id] = sub
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				sub.Close()
			case <-sub.done:
			}
		}()
	}
	return sub, nil
}

func (h *Hub) consumeWakeups() {
	defer h.wg.Done()
	for {
		select {
		case <-h.stop:
			return
		case wakeup, ok := <-h.notifier.Wakeups():
			if !ok {
				return
			}
			h.replayWakeup(wakeup)
		}
	}
}

func (h *Hub) replayWakeup(wakeup Wakeup) {
	if h.delivery == nil {
		return
	}
	// Snapshot targets and cursors without holding the Hub lock while querying
	// PostgreSQL. Start from the oldest locally retained cursor so a gap caused
	// by another instance is replayed instead of being skipped; duplicate IDs
	// are ignored below.
	type target struct {
		accountID, deviceID string
		after               int64
	}
	h.mu.Lock()
	targets := make([]target, 0)
	for key, q := range h.queues {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 || (wakeup.AccountID != "" && wakeup.AccountID != parts[0]) || (wakeup.DeviceID != "" && wakeup.DeviceID != parts[1]) || len(q.subs) == 0 {
			continue
		}
		after := int64(0)
		if len(q.events) > 0 && q.events[0].Cursor > 0 {
			after = q.events[0].Cursor - 1
		}
		targets = append(targets, target{parts[0], parts[1], after})
	}
	h.mu.Unlock()
	for _, t := range targets {
		page, err := h.delivery.Replay(context.Background(), t.accountID, t.deviceID, t.after, h.cfg.QueueSize)
		if err != nil {
			continue
		}
		h.mu.Lock()
		q := h.queues[targetKey(t.accountID, t.deviceID)]
		if q != nil {
			for _, event := range page.Events {
				h.acceptReplayLocked(q, event)
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) acceptReplayLocked(q *targetQueue, event Event) {
	for _, prior := range q.events {
		if prior.ID == event.ID || (event.Cursor > 0 && prior.Cursor == event.Cursor) {
			return
		}
	}
	if event.Cursor > q.nextCursor {
		q.nextCursor = event.Cursor
	}
	q.events = append(q.events, cloneEvent(event))
	if len(q.events) > h.cfg.RetentionPerDevice {
		q.events = q.events[len(q.events)-h.cfg.RetentionPerDevice:]
	}
	for id, sub := range q.subs {
		if !sub.sendLocked(event) {
			delete(q.subs, id)
			sub.closeOnce.Do(func() { sub.closeLocked(ErrBackpressure) })
		}
	}
}

func (h *Hub) queueLocked(accountID, deviceID string) *targetQueue {
	key := targetKey(accountID, deviceID)
	q := h.queues[key]
	if q == nil {
		q = &targetQueue{subs: make(map[uint64]*Subscription)}
		h.queues[key] = q
	}
	return q
}

func validateTarget(accountID, deviceID string) error {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(deviceID) == "" {
		return ErrInvalid
	}
	return nil
}

func targetKey(accountID, deviceID string) string { return accountID + "\x00" + deviceID }

func eventSize(event Event) int {
	b, _ := json.Marshal(event)
	return len(b)
}

func cloneEvent(event Event) Event {
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	return event
}

func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

// Subscription owns one channel. Close is safe to call repeatedly and
// removes the subscription from its target queue.
type Subscription struct {
	hub       *Hub
	id        uint64
	accountID string
	deviceID  string
	ch        chan Event
	done      chan struct{}
	// cursors tracks events delivered to this subscription, including durable
	// replay events which are not necessarily present in the process-local queue.
	// It lets protocol adapters resolve ACK cursors from the server-assigned
	// event cursor instead of trusting a client-supplied cursor.
	cursors   map[string]int64
	closeOnce sync.Once
	errMu     sync.RWMutex
	err       error
}

func newSubscription(h *Hub, id uint64, accountID, deviceID string, size int) *Subscription {
	return &Subscription{hub: h, id: id, accountID: accountID, deviceID: deviceID, ch: make(chan Event, size), done: make(chan struct{}), cursors: make(map[string]int64)}
}

// Events returns this connection's private event stream.
func (s *Subscription) Events() <-chan Event  { return s.ch }
func (s *Subscription) Done() <-chan struct{} { return s.done }

// CursorFor returns the server-assigned cursor for an event delivered to this
// subscription. It is safe to call concurrently with event delivery.
func (s *Subscription) CursorFor(id string) int64 {
	if s == nil || strings.TrimSpace(id) == "" {
		return 0
	}
	if s.hub != nil {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()
	}
	return s.cursors[id]
}
func (s *Subscription) Err() error {
	s.errMu.RLock()
	defer s.errMu.RUnlock()
	return s.err
}

func (s *Subscription) Close() { s.close(nil) }

func (s *Subscription) close(err error) {
	s.closeOnce.Do(func() {
		s.hub.mu.Lock()
		q := s.hub.queueLocked(s.accountID, s.deviceID)
		delete(q.subs, s.id)
		s.closeLocked(err)
		s.hub.mu.Unlock()
	})
}

func (s *Subscription) closeLocked(err error) {
	if err != nil {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
	}
	close(s.done)
	close(s.ch)
}

func (s *Subscription) sendLocked(event Event) bool {
	if s.cursors == nil {
		s.cursors = make(map[string]int64)
	}
	if event.ID != "" {
		s.cursors[event.ID] = event.Cursor
	}
	select {
	case s.ch <- cloneEvent(event):
		return true
	default:
		return false
	}
}

type rateLimiter struct {
	mu     sync.Mutex
	now    func() time.Time
	stamps map[string][]time.Time
}

func (l *rateLimiter) allow(key string, max int, window time.Duration) error {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stamps == nil {
		l.stamps = make(map[string][]time.Time)
	}
	cutoff := now.Add(-window)
	stamps := l.stamps[key]
	first := 0
	for first < len(stamps) && !stamps[first].After(cutoff) {
		first++
	}
	stamps = stamps[first:]
	if len(stamps) >= max {
		retry := stamps[0].Add(window).Sub(now)
		if retry < 0 {
			retry = 0
		}
		l.stamps[key] = stamps
		return &RateLimitError{RetryAfter: retry}
	}
	l.stamps[key] = append(stamps, now)
	return nil
}

var _ DeliveryHub = (*Hub)(nil)
