package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"relay-server/internal/httpapi"
	"relay-server/internal/notifications"
)

func (s *Server) notificationPreferences(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authDevice(r, "mobile")
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid device token")
		return
	}
	switch r.Method {
	case http.MethodGet:
		doc, err := s.notifications.GetPreferences(r.Context(), deviceID)
		if err != nil {
			httpapi.WriteError(w, r, 400, "invalid_device", err.Error())
			return
		}
		writeJSON(w, doc)
	case http.MethodPut:
		defer r.Body.Close()
		var body struct {
			Preferences notifications.ReminderPreferences `json:"preferences"`
			BaseVersion int64                             `json:"baseVersion"`
			IfMatch     string                            `json:"ifMatch"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&body) != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid preferences")
			return
		}
		doc, err := s.notifications.UpdatePreferences(r.Context(), notifications.UpdateRequest{DeviceID: deviceID, Preferences: body.Preferences, BaseVersion: body.BaseVersion, IfMatch: body.IfMatch})
		if err != nil {
			var conflict *notifications.ConflictError
			if errors.As(err, &conflict) {
				writeJSONStatus(w, http.StatusConflict, conflict.Current)
				return
			}
			httpapi.WriteError(w, r, 400, "invalid_request", err.Error())
			return
		}
		writeJSON(w, doc)
	default:
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

type registerTokenRequest struct {
	Provider   string `json:"provider"`
	Platform   string `json:"platform"`
	Token      string `json:"token"`
	AppVersion string `json:"appVersion,omitempty"`
}

func (s *Server) notificationProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	// Provider catalog is capability metadata and does not require device auth.
	writeJSON(w, s.notifications.ProviderMetadata())
}

func (s *Server) notificationTokens(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authDevice(r, "mobile")
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid device token")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.notifications.ListTokens(r.Context(), deviceID))
	case http.MethodPost:
		defer r.Body.Close()
		var req registerTokenRequest
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid token registration")
			return
		}
		token, err := s.notifications.RegisterToken(r.Context(), notifications.TokenRegistration{DeviceID: deviceID, Provider: req.Provider, Platform: req.Platform, Token: req.Token, AppVersion: req.AppVersion})
		if err != nil {
			status := 400
			code := "invalid_request"
			if errors.Is(err, notifications.ErrInvalidProvider) {
				code = "invalid_provider"
			} else if errors.Is(err, notifications.ErrInvalidPlatform) {
				code = "invalid_platform"
			}
			httpapi.WriteError(w, r, status, code, err.Error())
			return
		}
		writeJSONStatus(w, http.StatusCreated, token)
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if err := s.notifications.RevokeToken(r.Context(), deviceID, id, ""); err != nil {
			httpapi.WriteError(w, r, http.StatusNotFound, "not_found", "token not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}
func writeJSON(w http.ResponseWriter, v any) { writeJSONStatus(w, http.StatusOK, v) }
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
