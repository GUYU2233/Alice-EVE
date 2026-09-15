package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"eve-assistant/desktop-app/internal/protocol"
)

// AlertItem is the structured alert projection returned by the legacy sync
// API. MessageID accepts both the legacy messageId field and the protocol id.
type AlertItem struct {
	MessageID string                 `json:"messageId"`
	Type      string                 `json:"type"`
	Sender    string                 `json:"sender"`
	Payload   map[string]interface{} `json:"payload"`
	Cursor    int64                  `json:"cursor"`
	CreatedAt string                 `json:"createdAt"`
	Status    string                 `json:"status"`
}

func (a *AlertItem) UnmarshalJSON(data []byte) error {
	type alias AlertItem
	var value struct {
		alias
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = AlertItem(value.alias)
	if a.MessageID == "" {
		a.MessageID = value.ID
	}
	return nil
}

type OutboxItem struct {
	MessageID string      `json:"messageId"`
	Type      string      `json:"type"`
	Sender    string      `json:"sender"`
	Recipient string      `json:"recipient,omitempty"`
	Payload   interface{} `json:"payload"`
	Cursor    int64       `json:"cursor"`
	CreatedAt string      `json:"createdAt"`
	Status    string      `json:"status"`
}

func (o *OutboxItem) UnmarshalJSON(data []byte) error {
	type alias OutboxItem
	var value struct {
		alias
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*o = OutboxItem(value.alias)
	if o.MessageID == "" {
		o.MessageID = value.ID
	}
	return nil
}

type AgentConversation struct {
	ID             string `json:"id"`
	AccountID      string `json:"accountId"`
	TargetDeviceID string `json:"targetDeviceId"`
	AgentKind      string `json:"agentKind"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type ConversationMessagePage struct {
	Messages   []protocol.ConversationMessage `json:"messages"`
	NextCursor int64                          `json:"nextCursor"`
	HasMore    bool                           `json:"hasMore"`
}

type AgentRun struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	InputMessageID string `json:"inputMessageId"`
	Status         string `json:"status"`
	ErrorCode      string `json:"errorCode,omitempty"`
	StartedAt      string `json:"startedAt"`
	CompletedAt    string `json:"completedAt,omitempty"`
}

type AgentRunEvent struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversationId"`
	RunID          string          `json:"runId"`
	Sequence       int64           `json:"sequence"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      string          `json:"createdAt"`
}

type DataPage[T any] struct {
	Items   []T   `json:"items"`
	Cursor  int64 `json:"cursor"`
	HasMore bool  `json:"hasMore"`
}

func (r *RelayClient) getJSON(ctx context.Context, path string, out interface{}) error {
	return r.do(ctx, http.MethodGet, path, nil, out)
}

func normalizeLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}

func (r *RelayClient) FetchAlerts(ctx context.Context, cursor int64, limit int) (DataPage[AlertItem], error) {
	if cursor < 0 {
		cursor = 0
	}
	limit = normalizeLimit(limit)
	path := "/api/v1/alerts?" + url.Values{
		"cursor": {strconv.FormatInt(cursor, 10)},
		"limit":  {strconv.Itoa(limit)},
	}.Encode()
	var raw json.RawMessage
	if err := r.getJSON(ctx, path, &raw); err != nil {
		return DataPage[AlertItem]{}, err
	}
	var response struct {
		Messages []AlertItem `json:"messages"`
		Items    []AlertItem `json:"items"`
		Cursor   int64       `json:"cursor"`
		HasMore  bool        `json:"hasMore"`
	}
	if len(strings.TrimSpace(string(raw))) > 0 && strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		if err := json.Unmarshal(raw, &response.Items); err != nil {
			return DataPage[AlertItem]{}, fmt.Errorf("decode alerts: %w", err)
		}
	} else if err := json.Unmarshal(raw, &response); err != nil {
		return DataPage[AlertItem]{}, fmt.Errorf("decode alerts: %w", err)
	}
	if len(response.Items) == 0 {
		response.Items = response.Messages
	}
	if response.Cursor < cursor {
		response.Cursor = cursor
	}
	return DataPage[AlertItem]{Items: response.Items, Cursor: response.Cursor, HasMore: response.HasMore}, nil
}

// FetchOutbox reads the compatibility sync endpoint first, then tries the
// newer outbox path when a deployment exposes it. Both response envelopes are
// accepted so desktop clients can roll out independently of the server.
func (r *RelayClient) FetchOutbox(ctx context.Context, cursor int64, limit int) (DataPage[OutboxItem], error) {
	if cursor < 0 {
		cursor = 0
	}
	limit = normalizeLimit(limit)
	query := url.Values{"cursor": {strconv.FormatInt(cursor, 10)}, "limit": {strconv.Itoa(limit)}}.Encode()
	var last error
	for _, path := range []string{"/api/v1/sync?" + query, "/api/v1/outbox?" + query} {
		var raw json.RawMessage
		if err := r.getJSON(ctx, path, &raw); err != nil {
			last = err
			continue
		}
		var response struct {
			Messages []OutboxItem `json:"messages"`
			Items    []OutboxItem `json:"items"`
			Cursor   int64        `json:"cursor"`
			HasMore  bool         `json:"hasMore"`
		}
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "[") {
			if err := json.Unmarshal(raw, &response.Items); err != nil {
				return DataPage[OutboxItem]{}, fmt.Errorf("decode outbox: %w", err)
			}
		} else if err := json.Unmarshal(raw, &response); err != nil {
			return DataPage[OutboxItem]{}, fmt.Errorf("decode outbox: %w", err)
		}
		if len(response.Items) == 0 {
			response.Items = response.Messages
		}
		if response.Cursor < cursor {
			response.Cursor = cursor
		}
		return DataPage[OutboxItem]{Items: response.Items, Cursor: response.Cursor, HasMore: response.HasMore}, nil
	}
	return DataPage[OutboxItem]{}, last
}

func (r *RelayClient) FetchConversationMessages(ctx context.Context, conversationID string, cursor, limit int) (ConversationMessagePage, error) {
	if strings.TrimSpace(conversationID) == "" {
		return ConversationMessagePage{}, fmt.Errorf("conversation id is required")
	}
	if cursor < 0 {
		cursor = 0
	}
	query := url.Values{"cursor": {strconv.Itoa(cursor)}, "limit": {strconv.Itoa(normalizeLimit(limit))}}.Encode()
	var page ConversationMessagePage
	if err := r.getJSON(ctx, "/api/v1/conversations/"+url.PathEscape(conversationID)+"/messages?"+query, &page); err != nil {
		return page, err
	}
	return page, nil
}

func (r *RelayClient) GetRun(ctx context.Context, conversationID, runID string) (AgentRun, error) {
	var run AgentRun
	if err := r.getJSON(ctx, "/api/v1/conversations/"+url.PathEscape(conversationID)+"/runs/"+url.PathEscape(runID), &run); err != nil {
		return run, err
	}
	return run, nil
}

func (r *RelayClient) ListRunEvents(ctx context.Context, conversationID, runID string, after, limit int) ([]AgentRunEvent, int64, bool, error) {
	query := url.Values{"after": {strconv.Itoa(after)}, "limit": {strconv.Itoa(normalizeLimit(limit))}}.Encode()
	var response struct {
		Events     []AgentRunEvent `json:"events"`
		NextCursor int64           `json:"nextCursor"`
		HasMore    bool            `json:"hasMore"`
	}
	if err := r.getJSON(ctx, "/api/v1/conversations/"+url.PathEscape(conversationID)+"/runs/"+url.PathEscape(runID)+"/events?"+query, &response); err != nil {
		return nil, int64(after), false, err
	}
	return response.Events, response.NextCursor, response.HasMore, nil
}

func (r *RelayClient) StartRun(ctx context.Context, conversationID, messageID string) (AgentRun, error) {
	body, _ := json.Marshal(map[string]string{"messageId": messageID})
	var run AgentRun
	if err := r.do(ctx, http.MethodPost, "/api/v1/conversations/"+url.PathEscape(conversationID)+"/runs", body, &run); err != nil {
		return run, err
	}
	return run, nil
}

func (r *RelayClient) CancelRun(ctx context.Context, conversationID, runID string) (AgentRun, error) {
	var run AgentRun
	if err := r.do(ctx, http.MethodPost, "/api/v1/conversations/"+url.PathEscape(conversationID)+"/runs/"+url.PathEscape(runID)+"/cancel", nil, &run); err != nil {
		return run, err
	}
	return run, nil
}

func (r *RelayClient) FetchConversations(ctx context.Context) ([]AgentConversation, error) {
	var last error
	for _, path := range []string{"/api/v1/conversations", "/api/v1/agent/conversations"} {
		var raw json.RawMessage
		if err := r.getJSON(ctx, path, &raw); err != nil {
			last = err
			continue
		}
		var response struct {
			Items         []AgentConversation `json:"items"`
			Conversations []AgentConversation `json:"conversations"`
		}
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "[") {
			if err := json.Unmarshal(raw, &response.Items); err != nil {
				return nil, fmt.Errorf("decode conversations: %w", err)
			}
		} else if err := json.Unmarshal(raw, &response); err != nil {
			return nil, fmt.Errorf("decode conversations: %w", err)
		}
		if len(response.Items) == 0 {
			response.Items = response.Conversations
		}
		return response.Items, nil
	}
	return nil, last
}

func (r *RelayClient) FetchRealtimeSnapshot(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := r.getJSON(ctx, "/api/v2/realtime/snapshot", &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (r *RelayClient) SendConversationMessage(ctx context.Context, conversationID string, input protocol.ConversationMessageInput) (protocol.ConversationMessage, bool, error) {
	return r.SendAgentMessage(ctx, conversationID, input.ClientMessageID, input.Operation, input.Body)
}

func (r *RelayClient) SendAgentMessage(ctx context.Context, conversationID, clientMessageID, operation, body string) (protocol.ConversationMessage, bool, error) {
	if strings.TrimSpace(conversationID) == "" || strings.TrimSpace(clientMessageID) == "" || strings.TrimSpace(operation) == "" || strings.TrimSpace(body) == "" {
		return protocol.ConversationMessage{}, false, fmt.Errorf("conversation message fields are required")
	}
	payload, err := json.Marshal(protocol.ConversationMessageInput{ClientMessageID: clientMessageID, Operation: operation, Body: body})
	if err != nil {
		return protocol.ConversationMessage{}, false, fmt.Errorf("encode conversation message: %w", err)
	}
	var last error
	for _, path := range []string{"/api/v1/conversations/" + url.PathEscape(conversationID) + "/messages", "/api/v1/agent/conversations/messages"} {
		var raw json.RawMessage
		if err := r.do(ctx, http.MethodPost, path, payload, &raw); err != nil {
			last = err
			continue
		}
		var response struct {
			Message   protocol.ConversationMessage `json:"message"`
			Duplicate bool                         `json:"duplicate"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return protocol.ConversationMessage{}, false, fmt.Errorf("decode conversation message: %w", err)
		}
		if response.Message.ID == "" {
			_ = json.Unmarshal(raw, &response.Message)
		}
		if response.Message.ID == "" {
			return protocol.ConversationMessage{}, false, fmt.Errorf("conversation response missing message id")
		}
		return response.Message, response.Duplicate, nil
	}
	return protocol.ConversationMessage{}, false, last
}
