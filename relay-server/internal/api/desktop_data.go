package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"relay-server/internal/httpapi"
)

func accountIDForDevice(s *Server, deviceID string) string {
	if accountID, ok := s.store.DeviceAccountID(deviceID); ok {
		return accountID
	}
	return ""
}

// outbox returns the authenticated device's pending messages. It is a
// compatibility projection over the durable device-scoped message store; the
// token-derived owner prevents one device from reading another device's queue.
func (s *Server) outbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	deviceID, ok := s.authAnyDevice(r)
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid device token")
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	// Fetch one extra row so hasMore is accurate even when the page is exactly
	// full. ListMessages is already owner-scoped and excludes ACKed rows.
	accountID, _ := s.store.DeviceAccountID(deviceID)
	rows, err := s.store.ListMessages(r.Context(), accountID, deviceID, after, limit+1)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "store_unavailable", "message store unavailable")
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]map[string]any, 0, len(rows))
	cursor := after
	for _, row := range rows {
		if row.AcknowledgedAt != nil {
			continue
		}
		var envelope Envelope
		if json.Unmarshal([]byte(row.Body), &envelope) != nil {
			// Do not advance the cursor for data that cannot be represented in
			// the compatibility response; a later repair can then replay it.
			continue
		}
		items = append(items, map[string]any{"messageId": envelope.ID, "type": envelope.Type, "sender": envelope.Sender, "recipient": envelope.Recipient, "payload": envelope.Payload, "cursor": row.Cursor, "createdAt": envelope.TS, "status": "queued", "ownerDeviceId": deviceID})
		if row.Cursor > cursor {
			cursor = row.Cursor
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "cursor": cursor, "hasMore": hasMore})
}
