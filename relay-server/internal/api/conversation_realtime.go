package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"relay-server/internal/conversations"
	"relay-server/internal/realtime"
)

type conversationAuthorizer struct{ server *Server }

// conversationRunPublisher projects durable Agent run lifecycle writes onto
// the target desktop's account/device realtime stream. Authorization remains
// owned by conversations.Service; this adapter only resolves the already
// validated conversation target and performs best-effort delivery.
type conversationRunPublisher struct{ server *Server }

func (p *conversationRunPublisher) PublishRunEvent(ctx context.Context, accountID, conversationID, runID, typ string, payload json.RawMessage) error {
	if p == nil || p.server == nil || p.server.realtimeHub == nil {
		return errors.New("realtime unavailable")
	}
	conversation, err := p.server.conversationRepo.GetConversation(ctx, accountID, conversationID)
	if err != nil {
		return err
	}
	eventID := "run:" + runID + ":" + typ
	if typ == "agent.run.event" {
		var event conversations.RunEvent
		if json.Unmarshal(payload, &event) == nil && event.Sequence > 0 {
			eventID = fmt.Sprintf("run-event:%s:%d", runID, event.Sequence)
		}
	}
	_, err = p.server.realtimeHub.Publish(ctx, accountID, conversation.TargetDeviceID, realtime.Event{
		ID: eventID, Type: typ, Payload: append(json.RawMessage(nil), payload...),
	})
	return err
}

func (a *conversationAuthorizer) AuthorizeDevice(ctx context.Context, actor conversations.Actor) error {
	if a == nil || a.server == nil {
		return conversations.ErrUnauthorized
	}
	typ, ok := a.server.store.DeviceType(actor.DeviceID)
	if !ok {
		return conversations.ErrUnauthorized
	}
	if typ != string(actor.Kind) {
		return conversations.ErrForbidden
	}
	if account, ok := a.server.store.DeviceAccountID(actor.DeviceID); !ok || account != actor.AccountID {
		return conversations.ErrForbidden
	}
	return nil
}
func (s *Server) requestActor(r *http.Request) (conversations.Actor, bool) {
	token := bearerToken(r)
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("access_token"))
	}
	if token == "" || s.accountService == nil {
		return conversations.Actor{}, false
	}
	sess, err := s.accountService.AuthenticateAccess(r.Context(), token)
	if err != nil || sess.DeviceID == "" {
		return conversations.Actor{}, false
	}
	typ, ok := s.store.DeviceType(sess.DeviceID)
	if !ok {
		return conversations.Actor{}, false
	}
	kind := conversations.DeviceKind(typ)
	if kind != conversations.DeviceDesktop && kind != conversations.DeviceMobile {
		return conversations.Actor{}, false
	}
	return conversations.Actor{AccountID: sess.AccountID, DeviceID: sess.DeviceID, Kind: kind}, true
}
func (s *Server) conversationHandler(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requestActor(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/conversations/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "conversation id required", 400)
		return
	}
	id := parts[0]
	if len(parts) == 3 && (parts[1] == "runs" || parts[1] == "run") && r.Method == http.MethodGet {
		run, e := s.conversationSvc.GetRun(r.Context(), actor, parts[2])
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 4 && (parts[1] == "runs" || parts[1] == "run") && parts[3] == "events" && r.Method == http.MethodGet {
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		ev, e := s.conversationSvc.ListRunEvents(r.Context(), actor, parts[2], after, limit)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		next := after
		if len(ev) > 0 {
			next = ev[len(ev)-1].Sequence
		}
		writeConversationJSON(w, map[string]any{"events": ev, "nextCursor": next, "hasMore": len(ev) == limit})
		return
	}
	if len(parts) == 4 && (parts[1] == "runs" || parts[1] == "run") && parts[3] == "cancel" && r.Method == http.MethodPost {
		run, e := s.conversationSvc.CancelRun(r.Context(), actor, parts[2])
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 4 && (parts[1] == "runs" || parts[1] == "run") && parts[3] == "timeout" && r.Method == http.MethodPost {
		run, e := s.conversationSvc.TimeoutRun(r.Context(), actor, parts[2])
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 4 && (parts[1] == "runs" || parts[1] == "run") && parts[3] == "events" && r.Method == http.MethodPost {
		var req struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil {
			http.Error(w, "invalid event", 400)
			return
		}
		ev, e := s.conversationSvc.AddRunEvent(r.Context(), actor, parts[2], req.Type, req.Payload)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, ev)
		return
	}
	if len(parts) == 2 && (parts[1] == "run" || parts[1] == "runs") && r.Method == http.MethodPost {
		var req struct {
			MessageID string `json:"messageId"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		run, e := s.conversationSvc.StartRun(r.Context(), actor, id, req.MessageID)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		var req struct {
			RunID string `json:"runId"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req)
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "runId required", 400)
			return
		}
		run, e := s.conversationSvc.CancelRun(r.Context(), actor, req.RunID)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 3 && (parts[1] == "run" || parts[1] == "runs") && parts[2] == "cancel" && r.Method == http.MethodPost {
		run, e := s.conversationSvc.CancelRun(r.Context(), actor, id)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, run)
		return
	}
	if len(parts) == 3 && (parts[1] == "run" || parts[1] == "runs") && r.Method == http.MethodGet {
		run, e := s.conversationSvc.ListRunEvents(r.Context(), actor, parts[2], 0, 100)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, map[string]any{"events": run})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		c, e := s.conversationSvc.GetConversation(r.Context(), actor, id)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, c)
		return
	}
	if len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodGet {
		after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
		if after < 0 {
			after = 0
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		page, e := s.conversationSvc.ListMessages(r.Context(), actor, id, after, limit)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		writeConversationJSON(w, page)
		return
	}
	if len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodPost {
		var in conversations.MessageInput
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil {
			http.Error(w, "invalid message", 400)
			return
		}
		m, d, e := s.conversationSvc.SendMessage(r.Context(), actor, id, in)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		if d {
			returnJSONMessage(w, m, d)
			return
		}
		if _, transactional := s.conversationRepo.(conversations.TransactionalEventWriter); !transactional {
			payload, _ := json.Marshal(m)
			target := actor.DeviceID
			if c, ce := s.conversationRepo.GetConversation(r.Context(), actor.AccountID, id); ce == nil {
				target = c.TargetDeviceID
			}
			_, _ = s.realtimeHub.Publish(r.Context(), actor.AccountID, target, realtime.Event{ID: "conversation.message:" + m.ID, Type: "conversation.message", Payload: payload})
		}
		writeConversationJSON(w, map[string]any{"message": m, "duplicate": d})
		return
	}
	http.Error(w, "not found", 404)
}
func (s *Server) agentConversations(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requestActor(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if r.Method == http.MethodGet {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		rows, e := s.conversationRepo.ListConversations(r.Context(), actor.AccountID, limit)
		if e != nil {
			writeConversationError(w, r, e)
			return
		}
		// A desktop may only list conversations bound to that desktop. Mobile
		// remains account-scoped and can see all of its account's conversations.
		if actor.Kind == conversations.DeviceDesktop {
			filtered := rows[:0]
			for _, c := range rows {
				if c.TargetDeviceID == actor.DeviceID {
					filtered = append(filtered, c)
				}
			}
			rows = filtered
		}
		writeConversationJSON(w, map[string]any{"conversations": rows})
		return
	}
	if r.Method == http.MethodPost {
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
		return
	}
	http.Error(w, "method not allowed", 405)
}
func (s *Server) agentConversationMessages(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requestActor(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ConversationID string `json:"conversationId"`
		conversations.MessageInput
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil || strings.TrimSpace(req.ConversationID) == "" {
		http.Error(w, "invalid request", 400)
		return
	}
	m, d, e := s.conversationSvc.SendMessage(r.Context(), actor, req.ConversationID, req.MessageInput)
	if e != nil {
		writeConversationError(w, r, e)
		return
	}
	writeConversationJSON(w, map[string]any{"message": m, "duplicate": d})
}
func returnJSONMessage(w http.ResponseWriter, m conversations.Message, duplicate bool) {
	writeConversationJSON(w, map[string]any{"message": m, "duplicate": duplicate})
}

func writeConversationJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeConversationError(w http.ResponseWriter, _ *http.Request, e error) {
	status := 500
	switch {
	case errors.Is(e, conversations.ErrUnauthorized):
		status = 401
	case errors.Is(e, conversations.ErrForbidden):
		status = 403
	case errors.Is(e, conversations.ErrNotFound):
		status = 404
	case errors.Is(e, conversations.ErrInvalid), errors.Is(e, conversations.ErrOperationRequired), errors.Is(e, conversations.ErrInvalidOperation):
		status = 400
	case errors.Is(e, conversations.ErrClosed):
		status = 409
	}
	http.Error(w, e.Error(), status)
}
func wsOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	for _, candidate := range strings.FieldsFunc(os.Getenv("CORS_ALLOWED_ORIGINS"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(candidate), "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

var wsUpgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: wsOriginAllowed}

type wsRequest struct {
	Type    string `json:"type"`
	Cursor  int64  `json:"cursor"`
	After   int64  `json:"after"`
	EventID string `json:"eventId"`
	Status  string `json:"status"`
}

func findEventCursor(sub *realtime.Subscription, id string) int64 {
	if sub == nil || strings.TrimSpace(id) == "" {
		return 0
	}
	return sub.CursorFor(id)
}

type wsFrame struct {
	Type            string          `json:"type"`
	ProtocolVersion int             `json:"protocolVersion,omitempty"`
	Event           *realtime.Event `json:"event,omitempty"`
	Cursor          int64           `json:"cursor,omitempty"`
	Error           string          `json:"error,omitempty"`
	Code            string          `json:"code,omitempty"`
	Stream          string          `json:"stream,omitempty"`
	RetryAfterMs    int64           `json:"retryAfterMs,omitempty"`
}

func (s *Server) realtimeWebSocket(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requestActor(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	conn, e := wsUpgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(512 * 1024)
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(2 * time.Minute)) })
	after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	if after < 0 {
		after = 0
	}
	sub, e := s.realtimeHub.Subscribe(r.Context(), actor.AccountID, actor.DeviceID, after)
	if e != nil {
		code := "protocol_error"
		if errors.Is(e, realtime.ErrCursorExpired) {
			code = string(realtime.ErrorCursorExpired)
		}
		if errors.Is(e, realtime.ErrBackpressure) {
			code = string(realtime.ErrorBackpressure)
		}
		_ = conn.WriteJSON(wsFrame{Type: "error", ProtocolVersion: 2, Code: code, Error: e.Error(), Stream: "account-device"})
		return
	}
	defer sub.Close()
	requests := make(chan wsRequest, 16)
	readErr := make(chan error, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			var req wsRequest
			if err := conn.ReadJSON(&req); err != nil {
				readErr <- err
				return
			}
			select {
			case requests <- req:
			case <-r.Context().Done():
				return
			}
		}
	}()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-readErr:
			if err != nil {
				return
			}
			return
		case event, open := <-sub.Events():
			if !open {
				return
			}
			if err := conn.WriteJSON(wsFrame{Type: "event", ProtocolVersion: 2, Event: &event, Cursor: event.Cursor, Stream: "account-device"}); err != nil {
				return
			}
			if hub, ok := s.realtimeHub.(*realtime.Hub); ok {
				if d, ok := hub.Delivery().(realtime.DeliveryRepository); ok {
					_ = d.MarkDelivered(r.Context(), event.ID, time.Now().UTC())
				}
			}
		case <-sub.Done():
			return
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case req := <-requests:
			switch req.Type {
			case "ack":
				if strings.TrimSpace(req.EventID) == "" || req.Status == "" {
					_ = conn.WriteJSON(wsFrame{Type: "error", ProtocolVersion: 2, Code: string(realtime.ErrorProtocol), Error: "eventId and status are required"})
					continue
				}
				// ACK cursors are server-assigned. Ignore any client cursor and
				// resolve the cursor from the event delivered on this stream.
				ackCursor := findEventCursor(sub, req.EventID)
				if ackCursor == 0 {
					_ = conn.WriteJSON(wsFrame{Type: "error", ProtocolVersion: 2, Code: string(realtime.ErrorProtocol), Error: "cursor is required for ack"})
					continue
				}
				var delivery realtime.DeliveryAcker
				if hub, ok := s.realtimeHub.(*realtime.Hub); ok {
					delivery, _ = hub.Delivery().(realtime.DeliveryAcker)
				}
				if delivery != nil {
					if err := delivery.AckEvent(r.Context(), actor.AccountID, actor.DeviceID, req.EventID, req.Status, ackCursor, time.Now().UTC()); err != nil {
						_ = conn.WriteJSON(wsFrame{Type: "error", ProtocolVersion: 2, Code: string(realtime.ErrorProtocol), Error: "ack rejected"})
						continue
					}
				}
				if ackCursor > after {
					after = ackCursor
				}
				if err := conn.WriteJSON(wsFrame{Type: "acked", ProtocolVersion: 2, Event: &realtime.Event{ID: req.EventID, Cursor: ackCursor}, Cursor: after}); err != nil {
					return
				}
			case "ping":
				if err := conn.WriteJSON(wsFrame{Type: "pong", ProtocolVersion: 2, Cursor: after, Stream: "account-device"}); err != nil {
					return
				}
			case "subscribe":
				if req.Cursor > after {
					after = req.Cursor
				}
				if req.After > after {
					after = req.After
				}
				if err := conn.WriteJSON(wsFrame{Type: "subscribed", ProtocolVersion: 2, Cursor: after, Stream: "account-device"}); err != nil {
					return
				}
			default:
				if err := conn.WriteJSON(wsFrame{Type: "error", ProtocolVersion: 2, Code: string(realtime.ErrorProtocol), Error: "unsupported frame", Stream: "account-device"}); err != nil {
					return
				}
			}
		}
	}
}
