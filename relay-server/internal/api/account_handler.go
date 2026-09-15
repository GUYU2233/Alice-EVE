package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"relay-server/internal/accounts"
	"relay-server/internal/httpapi"
)

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type refreshResponse struct {
	TokenType        string `json:"tokenType"`
	ExpiresIn        int64  `json:"expiresIn"`
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	AccessExpiresAt  string `json:"accessExpiresAt"`
	RefreshExpiresAt string `json:"refreshExpiresAt"`
	SessionID        string `json:"sessionId"`
	AccountID        string `json:"accountId"`
	DeviceID         string `json:"deviceId,omitempty"`
}

// refreshSession rotates a refresh token. Raw credentials are only returned in
// this response; repositories receive hashes through accounts.Service.
func (s *Server) refreshSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	defer r.Body.Close()
	var req refreshRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := decoder.Decode(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "refreshToken is required")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
		return
	}
	credentials, err := s.accountService.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		status, code, message := http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid"
		switch {
		case errors.Is(err, accounts.ErrRefreshReplay):
			code, message = "refresh_token_replay", "refresh token replay detected"
		case errors.Is(err, accounts.ErrRefreshExpired):
			code, message = "refresh_token_expired", "refresh token is expired"
		case errors.Is(err, accounts.ErrAccountRevoked):
			code, message = "account_revoked", "account is revoked"
		}
		httpapi.WriteError(w, r, status, code, message)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(refreshResponse{
		TokenType: "Bearer", ExpiresIn: int64(s.accountService.Config.AccessTokenTTL / 1e9),
		AccessToken: credentials.AccessToken, RefreshToken: credentials.RefreshToken,
		AccessExpiresAt:  credentials.AccessExpiresAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		RefreshExpiresAt: credentials.ExpiresAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		SessionID:        credentials.Session.ID, AccountID: credentials.Session.AccountID,
		DeviceID: credentials.Session.DeviceID,
	})
}
