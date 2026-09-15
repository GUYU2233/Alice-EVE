package protocol

import "time"

// Conversation is the account-scoped server Agent session returned by the
// conversation API. Fields mirror the server contract and intentionally use
// JSON-friendly primitive values for Wails and client persistence.
type Conversation struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"accountId"`
	TargetDeviceID string    `json:"targetDeviceId"`
	AgentKind      string    `json:"agentKind"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ConversationMessage is a durable message in an Agent conversation.
type ConversationMessage struct {
	ID              string                 `json:"id"`
	AccountID       string                 `json:"accountId,omitempty"`
	ConversationID  string                 `json:"conversationId"`
	SenderKind      string                 `json:"senderKind"`
	SenderDeviceID  string                 `json:"senderDeviceId,omitempty"`
	ClientMessageID string                 `json:"clientMessageId,omitempty"`
	Operation       string                 `json:"operation,omitempty"`
	Body            string                 `json:"body"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Status          string                 `json:"status"`
	Cursor          int64                  `json:"cursor,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

// ConversationMessageInput is the untrusted portion of a message request.
type ConversationMessageInput struct {
	ClientMessageID string                 `json:"clientMessageId"`
	Operation       string                 `json:"operation"`
	Body            string                 `json:"body"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type ConversationMessagePage struct {
	Messages   []ConversationMessage `json:"messages"`
	NextCursor int64                 `json:"nextCursor"`
	HasMore    bool                  `json:"hasMore"`
}

type ConversationList struct {
	Conversations []Conversation `json:"conversations"`
	Cursor        int64          `json:"cursor,omitempty"`
}

// OutboxItem represents a server-delivery item. Envelope is retained as the
// structured protocol payload; status/cursor metadata is optional for legacy
// servers that return plain envelopes.
type OutboxItem struct {
	ID          string        `json:"id,omitempty"`
	Envelope    EventEnvelope `json:"envelope"`
	Status      string        `json:"status,omitempty"`
	Attempts    int           `json:"attempts,omitempty"`
	Cursor      int64         `json:"cursor,omitempty"`
	CreatedAt   time.Time     `json:"createdAt,omitempty"`
	NextAttempt time.Time     `json:"nextAttempt,omitempty"`
}

type OutboxPage struct {
	Items      []OutboxItem    `json:"items"`
	Messages   []EventEnvelope `json:"messages,omitempty"`
	Cursor     int64           `json:"cursor,omitempty"`
	NextCursor int64           `json:"nextCursor,omitempty"`
	HasMore    bool            `json:"hasMore,omitempty"`
}
