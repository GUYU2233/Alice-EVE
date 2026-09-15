package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"relay-server/internal/conversations"
)

// conversationReconcileResponse is a point-in-time, account-scoped projection
// used after a realtime gap. snapshotCursor is captured before any collection
// reads and callers can persist it as the boundary for the next live stream.
type conversationReconcileResponse struct {
	SnapshotCursor int64                  `json:"snapshotCursor"`
	Conversations  []conversationSnapshot `json:"conversations"`
}

type conversationSnapshot struct {
	ID             string                           `json:"id"`
	AccountID      string                           `json:"accountId"`
	TargetDeviceID string                           `json:"targetDeviceId"`
	AgentKind      string                           `json:"agentKind"`
	Status         conversations.ConversationStatus `json:"status"`
	CreatedAt      time.Time                        `json:"createdAt"`
	UpdatedAt      time.Time                        `json:"updatedAt"`
	Messages       []conversations.Message          `json:"messages"`
	Runs           []conversations.AgentRun         `json:"runs"`
}

// realtimeSnapshot is the stable compatibility name for the reconcile
// projection. Both /api/v1/realtime/snapshot and /api/v2/realtime/snapshot use
// the same account-scoped response shape.
func (s *Server) realtimeSnapshot(w http.ResponseWriter, r *http.Request) {
	s.conversationReconcile(w, r)
}

func (s *Server) conversationReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeConversationJSONError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	actor, ok := s.requestActor(r)
	if !ok {
		writeConversationJSONError(w, r, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	repo, ok := s.conversationRepo.(conversations.SnapshotRepository)
	if !ok {
		writeConversationJSONError(w, r, http.StatusServiceUnavailable, "snapshot_unavailable", "conversation snapshot is unavailable")
		return
	}

	limit := reconcileLimit(r.URL.Query().Get("limit"), 100)
	messageLimit := reconcileLimit(r.URL.Query().Get("messageLimit"), limit)
	runLimit := reconcileLimit(r.URL.Query().Get("runLimit"), limit)

	// Capture the boundary first. New writes may race with subsequent reads; the
	// response filters their message cursors out so it remains a coherent view.
	snapshotCursor, err := repo.SnapshotCursor(r.Context(), actor.AccountID)
	if err != nil {
		writeConversationJSONError(w, r, http.StatusServiceUnavailable, "snapshot_unavailable", "conversation snapshot is unavailable")
		return
	}
	rows, err := s.conversationRepo.ListConversations(r.Context(), actor.AccountID, limit)
	if err != nil {
		writeConversationError(w, r, err)
		return
	}
	out := make([]conversationSnapshot, 0, len(rows))
	for _, c := range rows {
		if c.AccountID != actor.AccountID || (actor.Kind == conversations.DeviceDesktop && c.TargetDeviceID != actor.DeviceID) {
			continue
		}
		page, err := s.conversationSvc.ListMessages(r.Context(), actor, c.ID, 0, messageLimit)
		if err != nil {
			writeConversationError(w, r, err)
			return
		}
		messages := make([]conversations.Message, 0, len(page.Messages))
		for _, m := range page.Messages {
			if m.Cursor <= snapshotCursor {
				messages = append(messages, m)
			}
		}
		runs, err := repo.ListRuns(r.Context(), actor.AccountID, c.ID, runLimit)
		if err != nil {
			writeConversationJSONError(w, r, http.StatusServiceUnavailable, "snapshot_unavailable", "conversation snapshot is unavailable")
			return
		}
		out = append(out, conversationSnapshot{
			ID: c.ID, AccountID: c.AccountID, TargetDeviceID: c.TargetDeviceID,
			AgentKind: c.AgentKind, Status: c.Status, CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt, Messages: messages, Runs: runs,
		})
	}
	writeConversationJSON(w, conversationReconcileResponse{SnapshotCursor: snapshotCursor, Conversations: out})
}

func reconcileLimit(raw string, fallback int) int {
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			if parsed > 1000 {
				return 1000
			}
			return parsed
		}
	}
	return fallback
}

func writeConversationJSONError(w http.ResponseWriter, _ *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}
