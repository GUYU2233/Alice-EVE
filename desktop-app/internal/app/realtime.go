package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/storage"
	"github.com/gorilla/websocket"
)

type RealtimeEvent struct {
	ID        string          `json:"id"`
	AccountID string          `json:"accountId"`
	DeviceID  string          `json:"deviceId"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Cursor    int64           `json:"cursor"`
	CreatedAt time.Time       `json:"createdAt"`
}

type realtimeFrame struct {
	Type            string         `json:"type"`
	ProtocolVersion int            `json:"protocolVersion,omitempty"`
	Event           *RealtimeEvent `json:"event,omitempty"`
	Cursor          int64          `json:"cursor,omitempty"`
	Error           string         `json:"error,omitempty"`
	Code            string         `json:"code,omitempty"`
	Stream          string         `json:"stream,omitempty"`
	RetryAfterMs    int64          `json:"retryAfterMs,omitempty"`
}

type realtimeRequest struct {
	Type    string `json:"type"`
	Cursor  int64  `json:"cursor,omitempty"`
	After   int64  `json:"after,omitempty"`
	EventID string `json:"eventId,omitempty"`
	Status  string `json:"status,omitempty"`
}

type RealtimeStatus struct {
	State     string `json:"state"`
	Cursor    int64  `json:"cursor"`
	RetryInMs int64  `json:"retryInMs,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

type RealtimeClient struct {
	relay      *RelayClient
	store      storageCursor
	mu         sync.RWMutex
	writeMu    sync.Mutex
	seenMu     sync.Mutex
	seen       map[string]struct{}
	state      RealtimeStatus
	cancel     context.CancelFunc
	events     chan RealtimeEvent
	status     chan RealtimeStatus
	conn       *websocket.Conn
	generation uint64
	deviceID   string
}

type storageCursor interface {
	LoadCursor(context.Context, string) (int64, error)
	SaveCursor(context.Context, string, int64) error
}

type memoryCursor struct {
	mu     sync.Mutex
	values map[string]int64
}

func newMemoryCursor() *memoryCursor { return &memoryCursor{values: map[string]int64{}} }
func (m *memoryCursor) LoadCursor(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[key], nil
}
func (m *memoryCursor) SaveCursor(_ context.Context, key string, v int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v > m.values[key] {
		m.values[key] = v
	}
	return nil
}

func NewRealtimeClient(relay *RelayClient, store storage.Store) *RealtimeClient {
	var curs storageCursor = newMemoryCursor()
	if store != nil {
		curs = &sqliteCursor{store: store}
	}
	return &RealtimeClient{relay: relay, store: curs, state: RealtimeStatus{State: "stopped"}, events: make(chan RealtimeEvent, 128), status: make(chan RealtimeStatus, 32), seen: make(map[string]struct{})}
}

type sqliteCursor struct{ store storage.Store }

func (s *sqliteCursor) LoadCursor(ctx context.Context, key string) (int64, error) {
	if _, err := s.store.Exec(ctx, `CREATE TABLE IF NOT EXISTS realtime_cursor (name TEXT PRIMARY KEY, cursor INTEGER NOT NULL DEFAULT 0)`); err != nil {
		return 0, err
	}
	row, err := s.store.Query(ctx, `SELECT cursor FROM realtime_cursor WHERE name=?`, key)
	if err != nil {
		return 0, err
	}
	defer row.Close()
	if row.Next() {
		var v int64
		return v, row.Scan(&v)
	}
	return 0, nil
}
func (s *sqliteCursor) SaveCursor(ctx context.Context, key string, v int64) error {
	_, err := s.store.Exec(ctx, `CREATE TABLE IF NOT EXISTS realtime_cursor (name TEXT PRIMARY KEY, cursor INTEGER NOT NULL DEFAULT 0); INSERT INTO realtime_cursor(name,cursor) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET cursor=CASE WHEN excluded.cursor > cursor THEN excluded.cursor ELSE cursor END`, key, v)
	return err
}

func (c *RealtimeClient) Ack(ctx context.Context, event RealtimeEvent, status string) error {
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("ack status is required")
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("realtime connection is not ready")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(realtimeRequest{Type: "ack", EventID: event.ID, Cursor: event.Cursor, Status: status})
}

func (c *RealtimeClient) Events() <-chan RealtimeEvent    { return c.events }
func (c *RealtimeClient) Statuses() <-chan RealtimeStatus { return c.status }
func (c *RealtimeClient) Status() RealtimeStatus          { c.mu.RLock(); defer c.mu.RUnlock(); return c.state }
func (c *RealtimeClient) emitStatus(s RealtimeStatus) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	select {
	case c.status <- s:
	default:
	}
}

func (c *RealtimeClient) Start(ctx context.Context, accountID, deviceID string) error {
	if c.relay == nil {
		return fmt.Errorf("relay client is required")
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(deviceID) == "" {
		return fmt.Errorf("account and device are required")
	}
	c.Stop()
	runCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.deviceID = deviceID
	c.generation++
	generation := c.generation
	c.mu.Unlock()
	go c.loop(runCtx, accountID, deviceID, generation)
	return nil
}
func (c *RealtimeClient) currentDeviceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deviceID
}

func (c *RealtimeClient) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.generation++
	cursor := c.state.Cursor
	c.mu.Unlock()
	c.emitStatus(RealtimeStatus{State: "stopped", Cursor: cursor})
}
func (c *RealtimeClient) loop(ctx context.Context, accountID, deviceID string, generation uint64) {
	key := accountID + "\x00" + deviceID
	cursor, err := c.store.LoadCursor(ctx, key)
	if err != nil {
		c.emitStatus(RealtimeStatus{State: "error", LastError: "realtime cursor unavailable"})
		return
	}
	c.mu.Lock()
	if generation != c.generation {
		c.mu.Unlock()
		return
	}
	c.state.Cursor = cursor
	c.mu.Unlock()
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		ws, err := c.connect(ctx, accountID, deviceID, cursor)
		if err == nil {
			c.mu.Lock()
			if generation != c.generation {
				c.mu.Unlock()
				_ = ws.Close()
				return
			}
			c.conn = ws
			c.mu.Unlock()
			attempt = 0
			c.emitStatus(RealtimeStatus{State: "connected", Cursor: cursor})
			err = c.readLoop(ctx, ws, &cursor, key)
			_ = ws.Close()
			c.mu.Lock()
			if c.conn == ws {
				c.conn = nil
			}
			c.mu.Unlock()
		}
		if ctx.Err() != nil {
			return
		}
		attempt++
		delay := time.Second * time.Duration(1<<min(attempt-1, 5))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		c.emitStatus(RealtimeStatus{State: "reconnecting", Cursor: cursor, RetryInMs: delay.Milliseconds(), LastError: safeError(err)})
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func (c *RealtimeClient) connect(ctx context.Context, accountID, deviceID string, cursor int64) (*websocket.Conn, error) {
	base := c.relay.URL()
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/realtime"
	q := u.Query()
	q.Set("cursor", fmt.Sprint(cursor))
	u.RawQuery = q.Encode()
	h := http.Header{}
	c.relay.authHeader(h)
	d := websocket.DefaultDialer
	d = &websocket.Dialer{HandshakeTimeout: 10 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 4096}
	ws, response, err := d.DialContext(ctx, u.String(), h)
	if err != nil && response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
		return nil, fmt.Errorf("realtime authentication rejected (%d)", response.StatusCode)
	}
	return ws, err
}
func (c *RealtimeClient) readLoop(ctx context.Context, ws *websocket.Conn, cursor *int64, key string) error {
	ws.SetReadLimit(512 * 1024)
	_ = ws.SetReadDeadline(time.Now().Add(70 * time.Second))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				c.writeMu.Lock()
				_ = ws.WriteJSON(realtimeRequest{Type: "ping"})
				c.writeMu.Unlock()
			}
		}
	}()
	for {
		var frame realtimeFrame
		if err := ws.ReadJSON(&frame); err != nil {
			return err
		}
		if frame.Error != "" || frame.Code != "" {
			code := strings.ToLower(strings.TrimSpace(frame.Code))
			if code == "cursor_expired" || code == "backpressure" || strings.Contains(strings.ToLower(frame.Error), "cursor") || strings.Contains(strings.ToLower(frame.Error), "backpressure") {
				return fmt.Errorf("realtime reconcile required: %s", frame.Error)
			}
			return fmt.Errorf("realtime %s: %s", code, frame.Error)
		}
		if frame.Type != "event" || frame.Event == nil {
			continue
		}
		if frame.Event.ID == "" || frame.Event.DeviceID != "" && frame.Event.DeviceID != c.currentDeviceID() {
			continue
		}
		c.seenMu.Lock()
		_, duplicate := c.seen[frame.Event.ID]
		if !duplicate {
			c.seen[frame.Event.ID] = struct{}{}
		}
		c.seenMu.Unlock()
		if duplicate {
			continue
		}
		if frame.Event.Cursor <= *cursor {
			continue
		}
		*cursor = frame.Event.Cursor
		if err := c.store.SaveCursor(ctx, key, *cursor); err != nil {
			return err
		}
		c.mu.Lock()
		c.state.Cursor = *cursor
		c.mu.Unlock()
		select {
		case c.events <- *frame.Event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
