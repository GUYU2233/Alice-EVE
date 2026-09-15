package conversations

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryRepository is a concurrency-safe repository suitable for tests and
// local development. Its behavior mirrors the repository contract: account
// scope is checked on every read/write and client message IDs are idempotent.
type MemoryRepository struct {
	mu            sync.RWMutex
	conversations map[string]Conversation
	messages      map[string]Message
	byClient      map[string]string
	nextCursor    map[string]int64
	runs          map[string]AgentRun
	runEvents     map[string][]RunEvent
	runSeq        map[string]int64
	audit         map[string][]AuditRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		conversations: make(map[string]Conversation),
		messages:      make(map[string]Message),
		byClient:      make(map[string]string),
		nextCursor:    make(map[string]int64),
		runs:          make(map[string]AgentRun),
		runEvents:     make(map[string][]RunEvent),
		runSeq:        make(map[string]int64),
		audit:         make(map[string][]AuditRecord),
	}
}

func (r *MemoryRepository) CreateConversation(_ context.Context, c Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.AccountID) == "" {
		return ErrInvalid
	}
	if _, exists := r.conversations[c.ID]; exists {
		return ErrConflict
	}
	if c.Status == "" {
		c.Status = ConversationActive
	}
	r.conversations[c.ID] = cloneConversation(c)
	return nil
}

func (r *MemoryRepository) GetConversation(_ context.Context, accountID, id string) (Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[id]
	if !ok || c.AccountID != accountID {
		return Conversation{}, ErrNotFound
	}
	return cloneConversation(c), nil
}

func (r *MemoryRepository) ListConversations(_ context.Context, accountID string, limit int) ([]Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	out := make([]Conversation, 0, limit)
	for _, c := range r.conversations {
		if c.AccountID == accountID {
			out = append(out, cloneConversation(c))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) CloseConversation(_ context.Context, accountID, id string, status ConversationStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.conversations[id]
	if !ok || c.AccountID != accountID {
		return ErrNotFound
	}
	c.Status = status
	c.UpdatedAt = time.Now().UTC()
	r.conversations[id] = c
	return nil
}

func (r *MemoryRepository) PutMessage(_ context.Context, m Message) (Message, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.conversations[m.ConversationID]
	if !ok || c.AccountID == "" {
		return Message{}, false, ErrNotFound
	}
	if m.ID == "" || m.ConversationID == "" || m.SenderDeviceID == "" {
		return Message{}, false, ErrInvalid
	}
	if m.AccountID != "" && m.AccountID != c.AccountID {
		return Message{}, false, ErrForbidden
	}
	m.AccountID = c.AccountID
	key := clientKey(c.AccountID, m.ConversationID, m.SenderDeviceID, m.ClientMessageID)
	if existingID, exists := r.byClient[key]; exists {
		return cloneMessage(r.messages[existingID]), true, nil
	}
	if m.Status == "" {
		m.Status = MessagePersisted
	}
	r.nextCursor[m.ConversationID]++
	m.Cursor = r.nextCursor[m.ConversationID]
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	r.messages[m.ID] = cloneMessage(m)
	if m.ClientMessageID != "" {
		r.byClient[key] = m.ID
	}
	return cloneMessage(m), false, nil
}

// FindMessageByClientID lets Service avoid charging retries against the rate
// limit. It is optional on external repository implementations.
func (r *MemoryRepository) FindMessageByClientID(_ context.Context, accountID, conversationID, senderDeviceID, clientMessageID string) (Message, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byClient[clientKey(accountID, conversationID, senderDeviceID, clientMessageID)]
	if !ok {
		return Message{}, false, nil
	}
	m, ok := r.messages[id]
	if !ok {
		return Message{}, false, nil
	}
	return cloneMessage(m), true, nil
}

func (r *MemoryRepository) GetMessage(_ context.Context, accountID, id string) (Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.messages[id]
	if !ok {
		return Message{}, ErrNotFound
	}
	c, ok := r.conversations[m.ConversationID]
	if !ok || c.AccountID != accountID {
		return Message{}, ErrNotFound
	}
	return cloneMessage(m), nil
}

func (r *MemoryRepository) SnapshotCursor(_ context.Context, accountID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var cursor int64
	for _, m := range r.messages {
		c, ok := r.conversations[m.ConversationID]
		if ok && c.AccountID == accountID && m.Cursor > cursor {
			cursor = m.Cursor
		}
	}
	return cursor, nil
}

func (r *MemoryRepository) ListMessages(_ context.Context, accountID, conversationID string, after int64, limit int) (MessagePage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[conversationID]
	if !ok || c.AccountID != accountID {
		return MessagePage{}, ErrNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	items := make([]Message, 0)
	for _, m := range r.messages {
		if m.ConversationID == conversationID && m.Cursor > after {
			items = append(items, cloneMessage(m))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Cursor < items[j].Cursor })
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Cursor
	}
	return MessagePage{Messages: items, NextCursor: next, HasMore: more}, nil
}

func (r *MemoryRepository) UpdateMessageStatus(_ context.Context, accountID, id string, status MessageStatus, at time.Time) (Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[id]
	if !ok {
		return Message{}, ErrNotFound
	}
	c, ok := r.conversations[m.ConversationID]
	if !ok || c.AccountID != accountID {
		return Message{}, ErrNotFound
	}
	if !canAdvance(m.Status, status) {
		return Message{}, ErrInvalidStatus
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	m.Status = status
	m.UpdatedAt = at
	r.messages[id] = cloneMessage(m)
	return cloneMessage(m), nil
}

func clientKey(accountID, conversationID, deviceID, clientMessageID string) string {
	return accountID + "\x00" + conversationID + "\x00" + deviceID + "\x00" + clientMessageID
}

func cloneConversation(c Conversation) Conversation { return c }

func cloneMessage(m Message) Message {
	m.Metadata = cloneMetadata(m.Metadata)
	return m
}

var _ ConversationRepository = (*MemoryRepository)(nil)

// MemoryAuthorizer provides account/device authorization without coupling the
// conversations package to the legacy store.Store. RegisterDevice can be used
// by tests or by a bootstrap adapter around the real device repository.
type MemoryAuthorizer struct {
	mu      sync.RWMutex
	devices map[string]authorizedDevice
}

type authorizedDevice struct {
	accountID string
	kind      DeviceKind
	revoked   bool
}

func NewMemoryAuthorizer() *MemoryAuthorizer {
	return &MemoryAuthorizer{devices: make(map[string]authorizedDevice)}
}

func (a *MemoryAuthorizer) RegisterDevice(accountID, deviceID string, kind DeviceKind) error {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(deviceID) == "" || (kind != DeviceDesktop && kind != DeviceMobile) {
		return ErrInvalid
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices[deviceID] = authorizedDevice{accountID: accountID, kind: kind}
	return nil
}

func (a *MemoryAuthorizer) RevokeDevice(deviceID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.devices[deviceID]
	if !ok {
		return false
	}
	d.revoked = true
	a.devices[deviceID] = d
	return true
}

func (a *MemoryAuthorizer) AuthorizeDevice(_ context.Context, actor Actor) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	d, ok := a.devices[actor.DeviceID]
	if !ok || d.revoked {
		return ErrUnauthorized
	}
	if d.accountID != actor.AccountID || d.kind != actor.Kind {
		return ErrForbidden
	}
	return nil
}

var _ DeviceAuthorizer = (*MemoryAuthorizer)(nil)
