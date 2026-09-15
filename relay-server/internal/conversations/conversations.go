// Package conversations contains the framework-independent conversation and
// Agent request rules. HTTP/WebSocket handlers should depend on Service and
// the repository interfaces in this package rather than on a database pool.
package conversations

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

// DeviceKind identifies the side of a conversation on which a device runs.
type DeviceKind string

const (
	DeviceDesktop DeviceKind = "desktop"
	DeviceMobile  DeviceKind = "mobile"
)

// Actor is the authenticated account/device identity. Device and account IDs
// are always obtained from authentication middleware, never from a message
// body supplied by a client.
type Actor struct {
	AccountID string
	DeviceID  string
	Kind      DeviceKind
}

// ConversationStatus is the lifecycle state of a conversation.
type ConversationStatus string

const (
	ConversationActive    ConversationStatus = "active"
	ConversationClosed    ConversationStatus = "closed"
	ConversationCancelled ConversationStatus = "cancelled"
)

// MessageStatus follows the durable delivery progression. Failed is terminal
// and may be used when an Agent cannot process an otherwise valid message.
type MessageStatus string

const (
	MessageAccepted  MessageStatus = "accepted"
	MessagePersisted MessageStatus = "persisted"
	MessageDelivered MessageStatus = "delivered"
	MessageProcessed MessageStatus = "processed"
	MessageFailed    MessageStatus = "failed"
)

// SenderKind is deliberately separate from DeviceKind so server generated
// messages can be represented without pretending they came from a device.
type SenderKind string

const (
	SenderMobile  SenderKind = "mobile"
	SenderDesktop SenderKind = "desktop"
	SenderServer  SenderKind = "server"
)

// Conversation is an account-scoped Agent conversation.
type Conversation struct {
	ID             string             `json:"id"`
	AccountID      string             `json:"accountId"`
	TargetDeviceID string             `json:"targetDeviceId"`
	AgentKind      string             `json:"agentKind"`
	Status         ConversationStatus `json:"status"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// Message is a persisted user/Agent message. Cursor is monotonic within a
// conversation and is used by the HTTP sync layer for replay.
type Message struct {
	ID              string         `json:"id"`
	AccountID       string         `json:"accountId"`
	ConversationID  string         `json:"conversationId"`
	SenderKind      SenderKind     `json:"senderKind"`
	SenderDeviceID  string         `json:"senderDeviceId"`
	ClientMessageID string         `json:"clientMessageId"`
	Operation       string         `json:"operation"`
	Body            string         `json:"body"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Status          MessageStatus  `json:"status"`
	Cursor          int64          `json:"cursor"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// MessageInput is the untrusted part of a new message.
type MessageInput struct {
	ClientMessageID string
	Operation       string
	Body            string
	Metadata        map[string]any
}

// RunStatus describes the lifecycle of one desktop Agent execution.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	RunTimedOut  RunStatus = "timed_out"
)

type AgentRun struct {
	ID             string     `json:"id"`
	AccountID      string     `json:"accountId"`
	ConversationID string     `json:"conversationId"`
	InputMessageID string     `json:"inputMessageId"`
	Status         RunStatus  `json:"status"`
	StartedAt      time.Time  `json:"startedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	ErrorCode      string     `json:"errorCode,omitempty"`
}

type RunEvent struct {
	ID             string          `json:"id"`
	AccountID      string          `json:"accountId"`
	ConversationID string          `json:"conversationId"`
	RunID          string          `json:"runId"`
	Sequence       int64           `json:"sequence"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type AuditRecord struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"accountId"`
	DeviceID       string    `json:"deviceId"`
	RequestID      string    `json:"requestId,omitempty"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Size           int       `json:"size,omitempty"`
	DurationMillis int64     `json:"durationMillis,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// MessagePage is a stable cursor page. NextCursor is the highest cursor in
// this page (or after when the page is empty).
type MessagePage struct {
	Messages   []Message `json:"messages"`
	NextCursor int64     `json:"nextCursor"`
	HasMore    bool      `json:"hasMore"`
}

var (
	ErrUnauthorized      = errors.New("conversation: unauthorized device")
	ErrForbidden         = errors.New("conversation: forbidden")
	ErrNotFound          = errors.New("conversation: not found")
	ErrConflict          = errors.New("conversation: conflict")
	ErrClosed            = errors.New("conversation: closed")
	ErrInvalid           = errors.New("conversation: invalid input")
	ErrInvalidOperation  = errors.New("conversation: operation is not allowed")
	ErrOperationRequired = errors.New("conversation: operation is required")
	ErrMessageTooLarge   = errors.New("conversation: message is too large")
	ErrRateLimited       = errors.New("conversation: message rate limit exceeded")
	ErrInvalidStatus     = errors.New("conversation: invalid message status transition")
	ErrRunNotCancellable = errors.New("conversation: run is not cancellable")
	ErrTimeout           = errors.New("conversation: run timed out")
)

// RateLimitError exposes a useful retry hint while retaining errors.Is
// compatibility with ErrRateLimited.
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", ErrRateLimited, e.RetryAfter.Round(time.Millisecond))
}
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// DeviceAuthorizer is the only authorization dependency required by Service.
// Implementations normally query the account/device repository. Returning
// ErrUnauthorized for absent/revoked devices and ErrForbidden for an account
// or kind mismatch is recommended.
type DeviceAuthorizer interface {
	AuthorizeDevice(context.Context, Actor) error
}

// ConversationRepository is intentionally small and contains no pgx types.
// PutMessage must be idempotent for (conversation_id, sender_device_id,
// client_message_id), returning the existing message and duplicate=true on a
// retry.
type ConversationRepository interface {
	CreateConversation(context.Context, Conversation) error
	GetConversation(context.Context, string, string) (Conversation, error)
	ListConversations(context.Context, string, int) ([]Conversation, error)
	CloseConversation(context.Context, string, string, ConversationStatus) error
	PutMessage(context.Context, Message) (message Message, duplicate bool, err error)
	GetMessage(context.Context, string, string) (Message, error)
	ListMessages(context.Context, string, string, int64, int) (MessagePage, error)
	UpdateMessageStatus(context.Context, string, string, MessageStatus, time.Time) (Message, error)
	CreateRun(context.Context, AgentRun) error
	GetRun(context.Context, string, string) (AgentRun, error)
	UpdateRun(context.Context, string, string, RunStatus, string, time.Time) (AgentRun, error)
	AppendRunEvent(context.Context, RunEvent) (RunEvent, error)
	ListRunEvents(context.Context, string, string, int64, int) ([]RunEvent, error)
	AppendAudit(context.Context, AuditRecord) error
	ListAudit(context.Context, string, string, int) ([]AuditRecord, error)
}

// RunEventPublisher delivers server-side Agent run lifecycle events to the
// target desktop. Implementations should treat publication as best effort:
// durable repositories remain the source of truth and clients can replay them.
type RunEventPublisher interface {
	PublishRunEvent(context.Context, string, string, string, string, json.RawMessage) error
}

// MessageDeduplicator is an optional repository capability. When available,
// Service can identify an idempotent retry before charging its rate budget.
type MessageDeduplicator interface {
	FindMessageByClientID(context.Context, string, string, string, string) (Message, bool, error)
}

// SnapshotRepository exposes the optional aggregate reads needed by the
// authenticated reconcile endpoint. Keeping this separate preserves the
// existing ConversationRepository compatibility seam for adapters.
type SnapshotRepository interface {
	SnapshotCursor(context.Context, string) (int64, error)
	ListRuns(context.Context, string, string, int) ([]AgentRun, error)
}

// TransactionalEventWriter is an optional PostgreSQL capability. Implementations
// must commit the domain mutation and its realtime outbox record atomically.
type TransactionalEventWriter interface {
	PutMessageWithEvent(context.Context, Message, string, json.RawMessage) (Message, bool, error)
	CreateRunWithEvent(context.Context, AgentRun, string, json.RawMessage) error
	UpdateRunWithEvent(context.Context, string, string, RunStatus, string, time.Time, string, json.RawMessage) (AgentRun, error)
	AppendRunEventWithEvent(context.Context, RunEvent, string, json.RawMessage) (RunEvent, error)
}

// Config is safe for production defaults but remains straightforward to
// replace in tests. MaxMessageBytes covers UTF-8 body plus JSON metadata and
// operation. A zero value is filled with DefaultConfig values by NewService.
type Config struct {
	MaxMessageBytes   int
	MessagesPerWindow int
	RateWindow        time.Duration
	Now               func() time.Time
	RequireOperation  bool
}

func DefaultConfig() Config {
	return Config{
		MaxMessageBytes:   256 * 1024,
		MessagesPerWindow: 30,
		RateWindow:        time.Minute,
		Now:               time.Now,
		RequireOperation:  true,
	}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = d.MaxMessageBytes
	}
	if c.MessagesPerWindow <= 0 {
		c.MessagesPerWindow = d.MessagesPerWindow
	}
	if c.RateWindow <= 0 {
		c.RateWindow = d.RateWindow
	}
	if c.Now == nil {
		c.Now = d.Now
	}
	return c
}

// Service applies authorization, operation, size, rate, lifecycle and status
// rules before delegating persistence to a repository.
type Service struct {
	repo       ConversationRepository
	authorizer DeviceAuthorizer
	publisher  RunEventPublisher
	cfg        Config
	limiter    *rateLimiter
}

// NewService creates an integration-ready service. The optional authorizer is
// convenient for trusted internal callers; network-facing callers should
// always provide one. NewServiceWithConfig is available when limits need to be
// customized.
func NewService(repo ConversationRepository, authorizer ...DeviceAuthorizer) *Service {
	return NewServiceWithConfig(repo, DefaultConfig(), authorizer...)
}

func NewServiceWithConfig(repo ConversationRepository, cfg Config, authorizer ...DeviceAuthorizer) *Service {
	if repo == nil {
		panic("conversations: nil repository")
	}
	cfg = cfg.withDefaults()
	var a DeviceAuthorizer
	if len(authorizer) > 0 {
		a = authorizer[0]
	}
	return &Service{repo: repo, authorizer: a, cfg: cfg, limiter: &rateLimiter{now: cfg.Now}}
}

// SetRunEventPublisher configures best-effort realtime publication for Agent
// run lifecycle events. It is safe to call during server wiring before use.
func (s *Service) SetRunEventPublisher(publisher RunEventPublisher) { s.publisher = publisher }

func (s *Service) publishRunEvent(ctx context.Context, run AgentRun, typ string, payload any) {
	if s.publisher == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = s.publisher.PublishRunEvent(ctx, run.AccountID, run.ConversationID, run.ID, typ, body)
}

// CreateConversation authorizes the caller and verifies the target desktop is
// in the same account before creating an active conversation.
func (s *Service) CreateConversation(ctx context.Context, actor Actor, targetDeviceID, agentKind string) (Conversation, error) {
	if err := validateActor(actor); err != nil {
		return Conversation{}, err
	}
	if err := s.authorize(ctx, actor); err != nil {
		return Conversation{}, err
	}
	if strings.TrimSpace(targetDeviceID) == "" || actor.AccountID == "" || strings.TrimSpace(agentKind) == "" {
		return Conversation{}, ErrInvalid
	}
	target := Actor{AccountID: actor.AccountID, DeviceID: targetDeviceID, Kind: DeviceDesktop}
	if err := s.authorize(ctx, target); err != nil {
		return Conversation{}, err
	}
	now := s.cfg.Now().UTC()
	c := Conversation{ID: NewID(), AccountID: actor.AccountID, TargetDeviceID: targetDeviceID, AgentKind: agentKind, Status: ConversationActive, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateConversation(ctx, c); err != nil {
		return Conversation{}, err
	}
	return c, nil
}

func (s *Service) GetConversation(ctx context.Context, actor Actor, id string) (Conversation, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return Conversation{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Conversation{}, ErrInvalid
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, id)
	if err != nil {
		return Conversation{}, err
	}
	if !conversationVisible(c, actor) {
		return Conversation{}, ErrForbidden
	}
	return c, nil
}

// SendMessage persists a mobile request with status=persisted. Retries using
// the same client message ID are returned without consuming rate budget and
// without creating another message.
func (s *Service) SendMessage(ctx context.Context, actor Actor, conversationID string, input MessageInput) (Message, bool, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return Message{}, false, err
	}
	if actor.Kind != DeviceMobile {
		return Message{}, false, ErrForbidden
	}
	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(input.ClientMessageID) == "" || strings.TrimSpace(input.Body) == "" {
		return Message{}, false, ErrInvalid
	}
	if s.cfg.RequireOperation && strings.TrimSpace(input.Operation) == "" {
		return Message{}, false, ErrOperationRequired
	}
	if input.Operation != "" && !IsAllowedOperation(input.Operation) {
		return Message{}, false, ErrInvalidOperation
	}
	if size := MessageSize(input); size > s.cfg.MaxMessageBytes {
		return Message{}, false, ErrMessageTooLarge
	}
	if dedup, ok := s.repo.(MessageDeduplicator); ok {
		if existing, found, err := dedup.FindMessageByClientID(ctx, actor.AccountID, conversationID, actor.DeviceID, input.ClientMessageID); err != nil {
			return Message{}, false, err
		} else if found {
			return existing, true, nil
		}
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, conversationID)
	if err != nil {
		return Message{}, false, err
	}
	if !conversationVisible(c, actor) {
		return Message{}, false, ErrForbidden
	}
	if c.Status != ConversationActive {
		return Message{}, false, ErrClosed
	}
	if err := s.limiter.allow(actor.AccountID+"/"+actor.DeviceID, s.cfg.MessagesPerWindow, s.cfg.RateWindow); err != nil {
		return Message{}, false, err
	}
	now := s.cfg.Now().UTC()
	m := Message{ID: NewID(), AccountID: actor.AccountID, ConversationID: conversationID, SenderKind: SenderMobile, SenderDeviceID: actor.DeviceID, ClientMessageID: input.ClientMessageID, Operation: input.Operation, Body: input.Body, Metadata: cloneMetadata(input.Metadata), Status: MessagePersisted, CreatedAt: now, UpdatedAt: now}
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		payload, _ := json.Marshal(m)
		return writer.PutMessageWithEvent(ctx, m, "conversation.message", payload)
	}
	return s.repo.PutMessage(ctx, m)
}

// ListMessages returns only messages from a conversation visible to actor.
func (s *Service) ListMessages(ctx context.Context, actor Actor, conversationID string, after int64, limit int) (MessagePage, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return MessagePage{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, conversationID)
	if err != nil {
		return MessagePage{}, err
	}
	if !conversationVisible(c, actor) {
		return MessagePage{}, ErrForbidden
	}
	return s.repo.ListMessages(ctx, actor.AccountID, conversationID, after, limit)
}

// UpdateMessageStatus enforces the status state machine and device direction:
// the target desktop advances delivered/processed; the mobile sender may
// advance accepted/persisted. Repeated stages are idempotent.
func (s *Service) UpdateMessageStatus(ctx context.Context, actor Actor, messageID string, status MessageStatus) (Message, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return Message{}, err
	}
	if strings.TrimSpace(messageID) == "" || !validMessageStatus(status) {
		return Message{}, ErrInvalid
	}
	m, err := s.repo.GetMessage(ctx, actor.AccountID, messageID)
	if err != nil {
		return Message{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, m.ConversationID)
	if err != nil {
		return Message{}, err
	}
	if !conversationVisible(c, actor) {
		return Message{}, ErrForbidden
	}
	if actor.Kind == DeviceMobile && m.SenderDeviceID != actor.DeviceID {
		return Message{}, ErrForbidden
	}
	if actor.Kind == DeviceDesktop && c.TargetDeviceID != actor.DeviceID {
		return Message{}, ErrForbidden
	}
	if !statusAllowedForActor(actor.Kind, status) {
		return Message{}, ErrForbidden
	}
	if !canAdvance(m.Status, status) {
		return Message{}, ErrInvalidStatus
	}
	return s.repo.UpdateMessageStatus(ctx, actor.AccountID, messageID, status, s.cfg.Now().UTC())
}

// StartRun creates a queued Agent run. Only the bound desktop may execute it.
func (s *Service) StartRun(ctx context.Context, actor Actor, conversationID, messageID string) (AgentRun, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return AgentRun{}, err
	}
	if actor.Kind != DeviceDesktop {
		return AgentRun{}, ErrForbidden
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, conversationID)
	if err != nil {
		return AgentRun{}, err
	}
	if c.TargetDeviceID != actor.DeviceID {
		return AgentRun{}, ErrForbidden
	}
	if c.Status != ConversationActive {
		return AgentRun{}, ErrClosed
	}
	if strings.TrimSpace(messageID) == "" {
		return AgentRun{}, ErrInvalid
	}
	message, err := s.repo.GetMessage(ctx, actor.AccountID, messageID)
	if err != nil {
		return AgentRun{}, err
	}
	if message.ConversationID != conversationID {
		return AgentRun{}, ErrForbidden
	}
	now := s.cfg.Now().UTC()
	run := AgentRun{ID: NewID(), AccountID: actor.AccountID, ConversationID: conversationID, InputMessageID: messageID, Status: RunQueued, StartedAt: now}
	payload, _ := json.Marshal(run)
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		if err := writer.CreateRunWithEvent(ctx, run, "agent.run.created", payload); err != nil {
			return AgentRun{}, err
		}
	} else if err := s.repo.CreateRun(ctx, run); err != nil {
		return AgentRun{}, err
	}
	if _, ok := s.repo.(TransactionalEventWriter); !ok {
		s.publishRunEvent(ctx, run, "agent.run.created", run)
	}
	return run, nil
}
func (s *Service) UpdateRun(ctx context.Context, actor Actor, runID string, status RunStatus, errorCode string) (AgentRun, error) {
	if !validRunStatus(status) {
		return AgentRun{}, ErrInvalid
	}
	if err := s.authorize(ctx, actor); err != nil {
		return AgentRun{}, err
	}
	if actor.Kind != DeviceDesktop {
		return AgentRun{}, ErrForbidden
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return AgentRun{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return AgentRun{}, err
	}
	if c.TargetDeviceID != actor.DeviceID {
		return AgentRun{}, ErrForbidden
	}
	typ := "agent.run.updated"
	switch status {
	case RunCompleted:
		typ = "agent.run.completed"
	case RunFailed:
		typ = "agent.run.failed"
	case RunCancelled:
		typ = "agent.run.cancelled"
	case RunTimedOut:
		typ = "agent.run.timed_out"
	}
	at := s.cfg.Now().UTC()
	payload, _ := json.Marshal(func() AgentRun {
		v := run
		v.Status = status
		v.ErrorCode = errorCode
		if status == RunCompleted || status == RunFailed || status == RunCancelled || status == RunTimedOut {
			v.CompletedAt = &at
		}
		return v
	}())
	var updated AgentRun
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		updated, err = writer.UpdateRunWithEvent(ctx, actor.AccountID, runID, status, errorCode, at, typ, payload)
	} else {
		updated, err = s.repo.UpdateRun(ctx, actor.AccountID, runID, status, errorCode, at)
	}
	if err != nil {
		return AgentRun{}, err
	}
	if _, ok := s.repo.(TransactionalEventWriter); !ok {
		s.publishRunEvent(ctx, updated, typ, updated)
	}
	return updated, nil
}
func (s *Service) CancelRun(ctx context.Context, actor Actor, runID string) (AgentRun, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return AgentRun{}, err
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return AgentRun{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return AgentRun{}, err
	}
	if !conversationVisible(c, actor) {
		return AgentRun{}, ErrForbidden
	}
	if run.Status != RunQueued && run.Status != RunRunning {
		return AgentRun{}, ErrRunNotCancellable
	}
	at := s.cfg.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"id": runID, "status": RunCancelled})
	var updated AgentRun
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		updated, err = writer.UpdateRunWithEvent(ctx, actor.AccountID, runID, RunCancelled, "", at, "agent.run.cancelled", payload)
	} else {
		updated, err = s.repo.UpdateRun(ctx, actor.AccountID, runID, RunCancelled, "", at)
	}
	if err != nil {
		return AgentRun{}, err
	}
	if _, ok := s.repo.(TransactionalEventWriter); !ok {
		s.publishRunEvent(ctx, updated, "agent.run.cancelled", updated)
	}
	return updated, nil
}
func (s *Service) TimeoutRun(ctx context.Context, actor Actor, runID string) (AgentRun, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return AgentRun{}, err
	}
	if actor.Kind != DeviceDesktop {
		return AgentRun{}, ErrForbidden
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return AgentRun{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return AgentRun{}, err
	}
	if c.TargetDeviceID != actor.DeviceID {
		return AgentRun{}, ErrForbidden
	}
	if run.Status != RunQueued && run.Status != RunRunning {
		return AgentRun{}, ErrRunNotCancellable
	}
	at := s.cfg.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"id": runID, "status": RunTimedOut, "errorCode": ErrTimeout.Error()})
	var updated AgentRun
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		updated, err = writer.UpdateRunWithEvent(ctx, actor.AccountID, runID, RunTimedOut, ErrTimeout.Error(), at, "agent.run.timed_out", payload)
	} else {
		updated, err = s.repo.UpdateRun(ctx, actor.AccountID, runID, RunTimedOut, ErrTimeout.Error(), at)
	}
	if err != nil {
		return AgentRun{}, err
	}
	if _, ok := s.repo.(TransactionalEventWriter); !ok {
		s.publishRunEvent(ctx, updated, "agent.run.timed_out", updated)
	}
	return updated, nil
}
func (s *Service) AddRunEvent(ctx context.Context, actor Actor, runID, typ string, payload json.RawMessage) (RunEvent, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return RunEvent{}, err
	}
	if actor.Kind != DeviceDesktop {
		return RunEvent{}, ErrForbidden
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return RunEvent{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return RunEvent{}, err
	}
	if c.TargetDeviceID != actor.DeviceID {
		return RunEvent{}, ErrForbidden
	}
	if strings.TrimSpace(typ) == "" {
		return RunEvent{}, ErrInvalid
	}
	eventInput := RunEvent{ID: NewID(), AccountID: actor.AccountID, ConversationID: run.ConversationID, RunID: runID, Type: typ, Payload: payload, CreatedAt: s.cfg.Now().UTC()}
	var event RunEvent
	if writer, ok := s.repo.(TransactionalEventWriter); ok {
		event, err = writer.AppendRunEventWithEvent(ctx, eventInput, "agent.run.event", payload)
	} else {
		event, err = s.repo.AppendRunEvent(ctx, eventInput)
	}
	if err != nil {
		return RunEvent{}, err
	}
	if _, ok := s.repo.(TransactionalEventWriter); !ok {
		s.publishRunEvent(ctx, run, "agent.run.event", event)
	}
	return event, nil
}
func (s *Service) GetRun(ctx context.Context, actor Actor, runID string) (AgentRun, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return AgentRun{}, err
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return AgentRun{}, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return AgentRun{}, err
	}
	if !conversationVisible(c, actor) {
		return AgentRun{}, ErrForbidden
	}
	return run, nil
}
func (s *Service) ListRunEvents(ctx context.Context, actor Actor, runID string, after int64, limit int) ([]RunEvent, error) {
	if err := s.authorize(ctx, actor); err != nil {
		return nil, err
	}
	run, err := s.repo.GetRun(ctx, actor.AccountID, runID)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, run.ConversationID)
	if err != nil {
		return nil, err
	}
	if !conversationVisible(c, actor) {
		return nil, ErrForbidden
	}
	return s.repo.ListRunEvents(ctx, actor.AccountID, runID, after, limit)
}

func (s *Service) CloseConversation(ctx context.Context, actor Actor, id string, status ConversationStatus) error {
	if err := s.authorize(ctx, actor); err != nil {
		return err
	}
	c, err := s.repo.GetConversation(ctx, actor.AccountID, id)
	if err != nil {
		return err
	}
	if !conversationVisible(c, actor) {
		return ErrForbidden
	}
	if status != ConversationClosed && status != ConversationCancelled {
		return ErrInvalid
	}
	return s.repo.CloseConversation(ctx, actor.AccountID, id, status)
}

func (s *Service) authorize(ctx context.Context, actor Actor) error {
	if err := validateActor(actor); err != nil {
		return err
	}
	if s.authorizer == nil {
		return nil
	}
	return s.authorizer.AuthorizeDevice(ctx, actor)
}

func validateActor(a Actor) error {
	if strings.TrimSpace(a.AccountID) == "" || strings.TrimSpace(a.DeviceID) == "" || (a.Kind != DeviceMobile && a.Kind != DeviceDesktop) {
		return ErrUnauthorized
	}
	return nil
}

func conversationVisible(c Conversation, a Actor) bool {
	return c.AccountID == a.AccountID && (a.Kind == DeviceMobile || c.TargetDeviceID == a.DeviceID)
}

func validMessageStatus(s MessageStatus) bool {
	switch s {
	case MessageAccepted, MessagePersisted, MessageDelivered, MessageProcessed, MessageFailed:
		return true
	default:
		return false
	}
}

func statusRank(s MessageStatus) int {
	switch s {
	case MessageAccepted:
		return 1
	case MessagePersisted:
		return 2
	case MessageDelivered:
		return 3
	case MessageProcessed:
		return 4
	default:
		return 0
	}
}

func canAdvance(from, to MessageStatus) bool {
	if from == to {
		return true
	}
	if to == MessageFailed && from != MessageProcessed && from != MessageFailed {
		return true
	}
	if from == MessageFailed || from == MessageProcessed {
		return false
	}
	return statusRank(to) >= statusRank(from)
}

func statusAllowedForActor(kind DeviceKind, status MessageStatus) bool {
	if kind == DeviceMobile {
		return status == MessageAccepted || status == MessagePersisted
	}
	return status == MessageDelivered || status == MessageProcessed || status == MessageFailed
}

// AllowedOperations is an immutable copy of the server allowlist. Callers
// cannot mutate the package's policy by changing the returned map.
func AllowedOperations() map[string]struct{} {
	return map[string]struct{}{
		"get_desktop_status":    {},
		"get_recent_intel":      {},
		"get_route_summary":     {},
		"get_character_summary": {},
	}
}

func IsAllowedOperation(operation string) bool {
	_, ok := AllowedOperations()[strings.TrimSpace(operation)]
	return ok
}

func DesktopOperations() map[string]struct{} {
	return map[string]struct{}{"show_notification": {}, "notify": {}, "open_workspace": {}}
}
func IsDesktopOperation(operation string) bool {
	_, ok := DesktopOperations()[strings.TrimSpace(operation)]
	return ok
}

func ValidateOperation(operation string) error {
	if !IsAllowedOperation(operation) {
		return ErrInvalidOperation
	}
	return nil
}

// MessageSize measures the complete bounded user input rather than only the
// visible body. JSON encoding also catches metadata values that stringify to a
// larger representation than expected.
func MessageSize(input MessageInput) int {
	b, _ := json.Marshal(struct {
		ClientMessageID string         `json:"clientMessageId"`
		Operation       string         `json:"operation"`
		Body            string         `json:"body"`
		Metadata        map[string]any `json:"metadata,omitempty"`
	}{input.ClientMessageID, input.Operation, input.Body, input.Metadata})
	return len(b)
}

func cloneMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// NewID is intentionally dependency-free. Repositories may replace IDs with
// ULIDs, provided they retain uniqueness within an account/device scope.
func NewID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
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
		l.stamps = map[string][]time.Time{}
	}
	cutoff := now.Add(-window)
	stamps := l.stamps[key]
	first := 0
	for first < len(stamps) && !stamps[first].After(cutoff) {
		first++
	}
	stamps = stamps[first:]
	if len(stamps) >= max {
		retry := window
		if len(stamps) > 0 {
			retry = stamps[0].Add(window).Sub(now)
			if retry < 0 {
				retry = 0
			}
		}
		l.stamps[key] = stamps
		return &RateLimitError{RetryAfter: retry}
	}
	l.stamps[key] = append(stamps, now)
	return nil
}
