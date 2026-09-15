package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"relay-server/internal/accounts"
	"relay-server/internal/authn"
	"relay-server/internal/httpapi"
	"relay-server/internal/store"
)

const deviceChallengeTTL = 5 * time.Minute

// allowUnverifiedDeviceAuth is intentionally fail-closed. Provider/subject
// values are client-controlled and may only bootstrap an account when an
// operator explicitly opts into this development/test-only behavior.
func allowUnverifiedDeviceAuth() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("ALLOW_UNVERIFIED_DEVICE_AUTH")))
	return value == "1" || value == "true"
}

type DeviceAuthStartRequest struct {
	// AccountID may be used by an already authenticated account. For a new
	// account, Provider and Subject identify the upstream identity assertion;
	// this endpoint does not accept or persist an OAuth client secret.
	AccountID   string `json:"accountId,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Subject     string `json:"subject,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	DeviceName  string `json:"deviceName"`
	DeviceType  string `json:"deviceType"`
	PublicKey   string `json:"publicKey,omitempty"`
}

type DeviceAuthStartResponse struct {
	Challenge string    `json:"challenge"`
	ExpiresAt time.Time `json:"expiresAt"`
	ExpiresIn int64     `json:"expiresIn"`
}

type deviceChallenge struct {
	AccountID   string
	Provider    string
	Subject     string
	DisplayName string
	DeviceName  string
	DeviceType  string
	PublicKey   string
	ExpiresAt   time.Time
}

type DeviceAuthCompleteRequest struct {
	Challenge string `json:"challenge"`
}

type DeviceAuthResponse struct {
	TokenType        string    `json:"tokenType"`
	ExpiresIn        int64     `json:"expiresIn"`
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken"`
	DeviceToken      string    `json:"deviceToken,omitempty"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	SessionID        string    `json:"sessionId"`
	AccountID        string    `json:"accountId"`
	DeviceID         string    `json:"deviceId"`
}

func (s *Server) deviceAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	defer r.Body.Close()
	var req DeviceAuthStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid device authorization request")
		return
	}
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Subject = strings.TrimSpace(req.Subject)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	req.DeviceType = strings.TrimSpace(req.DeviceType)
	// Anonymous identity bootstrap is deliberately opt-in. Never infer safety
	// from the backing database: a development server may still contain real
	// accounts, and production deployments may be configured without DATABASE_URL.
	if req.AccountID == "" && !allowUnverifiedDeviceAuth() {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "identity_required", "verified identity is required")
		return
	}
	if req.AccountID != "" && !allowUnverifiedDeviceAuth() {
		if s.accountService == nil {
			httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
			return
		}
		session, err := s.accountService.AuthenticateAccess(r.Context(), bearerToken(r))
		if err != nil || session.AccountID != req.AccountID {
			httpapi.WriteError(w, r, http.StatusUnauthorized, "identity_required", "verified identity is required")
			return
		}
	}
	if (req.AccountID == "" && (req.Provider == "" || req.Subject == "")) ||
		(req.AccountID != "" && (req.Provider != "" || req.Subject != "")) ||
		len(req.AccountID) > 128 || len(req.Provider) > 128 || len(req.Subject) > 512 ||
		len(req.DisplayName) > 128 || req.DeviceName == "" || len(req.DeviceName) > 128 ||
		(req.DeviceType != "desktop" && req.DeviceType != "mobile") || len(req.PublicKey) > 4096 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid device authorization request")
		return
	}
	challenge, err := authn.GenerateToken()
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "auth_unavailable", "authentication is unavailable")
		return
	}
	expiresAt := time.Now().UTC().Add(deviceChallengeTTL)
	if s.challengeRepo != nil {
		if err := s.challengeRepo.CreateChallenge(r.Context(), authn.Challenge{Hash: authn.HashToken(challenge), AccountID: req.AccountID, Provider: req.Provider, Subject: req.Subject, DisplayName: req.DisplayName, DeviceName: req.DeviceName, DeviceType: req.DeviceType, PublicKey: req.PublicKey, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}); err != nil {
			httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
			return
		}
	} else {
		s.mu.Lock()
		if s.deviceChallenges == nil {
			s.deviceChallenges = make(map[string]deviceChallenge)
		}
		// Opportunistic cleanup prevents abandoned challenges from growing memory.
		now := time.Now().UTC()
		for hash, pending := range s.deviceChallenges {
			if !now.Before(pending.ExpiresAt) {
				delete(s.deviceChallenges, hash)
			}
		}
		s.deviceChallenges[authn.HashToken(challenge)] = deviceChallenge{
			AccountID: req.AccountID, Provider: req.Provider, Subject: req.Subject,
			DisplayName: req.DisplayName, DeviceName: req.DeviceName, DeviceType: req.DeviceType,
			PublicKey: req.PublicKey, ExpiresAt: expiresAt,
		}
		s.mu.Unlock()
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeviceAuthStartResponse{Challenge: challenge, ExpiresAt: expiresAt, ExpiresIn: int64(deviceChallengeTTL / time.Second)})
}

func (s *Server) deviceAuthComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	defer r.Body.Close()
	var req DeviceAuthCompleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || strings.TrimSpace(req.Challenge) == "" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "challenge is required")
		return
	}
	req.Challenge = strings.TrimSpace(req.Challenge)
	var pending deviceChallenge
	var ok bool
	if s.challengeRepo != nil {
		c, err := s.challengeRepo.ConsumeChallenge(r.Context(), authn.HashToken(req.Challenge), time.Now().UTC())
		if err == nil {
			pending = deviceChallenge{AccountID: c.AccountID, Provider: c.Provider, Subject: c.Subject, DisplayName: c.DisplayName, DeviceName: c.DeviceName, DeviceType: c.DeviceType, PublicKey: c.PublicKey, ExpiresAt: c.ExpiresAt}
			ok = true
		}
	} else {
		s.mu.Lock()
		pending, ok = s.deviceChallenges[authn.HashToken(req.Challenge)]
		if ok {
			delete(s.deviceChallenges, authn.HashToken(req.Challenge))
		}
		s.mu.Unlock()
	}
	if !ok || !time.Now().UTC().Before(pending.ExpiresAt) {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "invalid_challenge", "challenge is expired or invalid")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
		return
	}
	var account accounts.Account
	var err error
	if pending.AccountID != "" {
		account, err = s.accountRepo.GetAccount(r.Context(), pending.AccountID)
	} else {
		account, err = s.accountService.EnsureAccount(r.Context(), pending.Provider, pending.Subject, pending.DisplayName)
	}
	if err != nil {
		status, code, message := http.StatusUnauthorized, "identity_invalid", "identity is invalid"
		if errors.Is(err, accounts.ErrNotFound) {
			status, code, message = http.StatusUnauthorized, "account_not_found", "account was not found"
		}
		httpapi.WriteError(w, r, status, code, message)
		return
	}
	deviceID := store.NewDeviceID()
	credentials, err := s.accountService.IssueSession(r.Context(), account.ID, deviceID)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "auth_unavailable", "authentication is unavailable")
		return
	}
	// Keep a separate legacy device credential. Access tokens rotate on refresh,
	// while the compatibility device APIs intentionally use a stable opaque
	// device token. Only its hash is stored in the device table.
	deviceToken, err := authn.GenerateToken()
	if err != nil {
		_ = s.accountService.RevokeSession(r.Context(), credentials.Session.ID)
		httpapi.WriteError(w, r, http.StatusInternalServerError, "auth_unavailable", "authentication is unavailable")
		return
	}
	if err := s.store.AddDevice(store.Device{ID: deviceID, AccountID: account.ID, TokenHash: authn.HashToken(deviceToken), Name: pending.DeviceName, Type: pending.DeviceType, PublicKey: pending.PublicKey, CreatedAt: time.Now().UTC()}); err != nil {
		_ = s.accountService.RevokeSession(r.Context(), credentials.Session.ID)
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "device registration unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeviceAuthResponse{
		TokenType: "Bearer", ExpiresIn: int64(s.accountService.Config.AccessTokenTTL / time.Second),
		AccessToken: credentials.AccessToken, RefreshToken: credentials.RefreshToken, DeviceToken: deviceToken,
		AccessExpiresAt: credentials.AccessExpiresAt, RefreshExpiresAt: credentials.ExpiresAt,
		SessionID: credentials.Session.ID, AccountID: account.ID, DeviceID: deviceID,
	})
}

func (s *Server) revokeAccountDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if s.accountService == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
		return
	}
	session, err := s.accountService.AuthenticateAccess(r.Context(), bearerToken(r))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "access token is invalid")
		return
	}
	if err := s.accountService.RevokeDevice(r.Context(), session.AccountID, session.DeviceID); err != nil && !errors.Is(err, accounts.ErrNotFound) {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "auth_unavailable", "authentication is unavailable")
		return
	}
	if session.DeviceID != "" {
		_ = s.store.RevokeDevice(session.DeviceID)
	}
	w.WriteHeader(http.StatusNoContent)
}
