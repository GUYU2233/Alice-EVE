package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"relay-server/internal/accounts"
	"relay-server/internal/httpapi"
	"strings"
	"time"
)

type accountDeviceResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	LastActiveAt *time.Time `json:"lastActiveAt,omitempty"`
	Current      bool       `json:"current"`
}

func (s *Server) accountDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "method not allowed")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	session, err := s.accountService.AuthenticateAccess(r.Context(), bearerToken(r))
	if err != nil {
		httpapi.WriteError(w, r, 401, "unauthorized", "access token is invalid")
		return
	}
	sessions, err := s.accountService.ListSessions(r.Context(), session.AccountID)
	if err != nil {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	out := make([]accountDeviceResponse, 0, len(sessions))
	seen := map[string]bool{}
	for _, item := range sessions {
		if item.DeviceID == "" || seen[item.DeviceID] {
			continue
		}
		seen[item.DeviceID] = true
		d, ok := s.store.GetDevice(item.DeviceID)
		if !ok {
			continue
		}
		out = append(out, accountDeviceResponse{ID: d.ID, Name: d.Name, Type: d.Type, LastActiveAt: item.LastSeenAt, Current: d.ID == session.DeviceID})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"devices": out})
}
func (s *Server) revokeAccountDeviceByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "method not allowed")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	session, err := s.accountService.AuthenticateAccess(r.Context(), bearerToken(r))
	if err != nil {
		httpapi.WriteError(w, r, 401, "unauthorized", "access token is invalid")
		return
	}
	defer r.Body.Close()
	var req struct {
		DeviceID string `json:"deviceId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || strings.TrimSpace(req.DeviceID) == "" {
		httpapi.WriteError(w, r, 400, "invalid_request", "deviceId is required")
		return
	}
	if err := s.accountService.RevokeDevice(r.Context(), session.AccountID, req.DeviceID); err != nil && !errors.Is(err, accounts.ErrNotFound) {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	if !s.store.RevokeDeviceForAccount(session.AccountID, req.DeviceID) {
		httpapi.WriteError(w, r, http.StatusNotFound, "not_found", "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) revokeAllAccountDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "method not allowed")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	session, err := s.accountService.AuthenticateAccess(r.Context(), bearerToken(r))
	if err != nil {
		httpapi.WriteError(w, r, 401, "unauthorized", "access token is invalid")
		return
	}
	if err := s.accountService.RevokeAccount(r.Context(), session.AccountID); err != nil && !errors.Is(err, accounts.ErrNotFound) {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
