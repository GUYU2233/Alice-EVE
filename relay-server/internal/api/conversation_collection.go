package api

import (
	"encoding/json"
	"net/http"
	"relay-server/internal/conversations"
	"strconv"
)

// conversationsCollection is the canonical account-scoped conversation endpoint.
func (s *Server) conversationsCollection(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requestActor(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		rows, e := s.conversationRepo.ListConversations(r.Context(), actor.AccountID, limit)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		rows = conversationsVisibleToActor(rows, actor)
		writeConversationJSON(w, map[string]any{"conversations": rows})
	case http.MethodPost:
		var req struct {
			TargetDeviceID string `json:"targetDeviceId"`
			AgentKind      string `json:"agentKind"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		c, e := s.conversationSvc.CreateConversation(r.Context(), actor, req.TargetDeviceID, req.AgentKind)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, c)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func conversationsVisibleToActor(rows []conversations.Conversation, actor conversations.Actor) []conversations.Conversation {
	if actor.Kind != conversations.DeviceDesktop {
		return rows
	}
	filtered := rows[:0]
	for _, c := range rows {
		if c.TargetDeviceID == actor.DeviceID {
			filtered = append(filtered, c)
		}
	}
	return filtered
}
