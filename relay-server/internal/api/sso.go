package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"relay-server/internal/auth"
	"relay-server/internal/authn"
	"relay-server/internal/httpapi"
	"relay-server/internal/store"
)

const oauthBrowserCookie = "__Host-eve-oauth"

type SSOStartRequest struct {
	CodeChallenge string `json:"codeChallenge"`
	RedirectURI   string `json:"redirectUri,omitempty"`
	DeviceName    string `json:"deviceName"`
	DeviceType    string `json:"deviceType"`
	PublicKey     string `json:"publicKey,omitempty"`
}
type SSOStartResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
	ExpiresIn        int64  `json:"expiresIn"`
}
type SSOCallbackRequest struct {
	State        string `json:"state"`
	Code         string `json:"code"`
	CodeVerifier string `json:"codeVerifier"`
}

func randomBrowserBinding() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ssoStart supports the browser flow (GET) and the explicit native flow (POST).
// Browser transactions retain verifier and device metadata server-side, and bind
// state to a Secure, HttpOnly, SameSite=Lax cookie.
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	if s.oauth == nil {
		httpapi.WriteError(w, r, http.StatusNotFound, "oauth_unavailable", "oauth is unavailable")
		return
	}
	if r.Method == http.MethodGet {
		binding, err := randomBrowserBinding()
		if err != nil {
			httpapi.WriteError(w, r, 500, "oauth_unavailable", "oauth is unavailable")
			return
		}
		pkce, err := auth.NewPKCE()
		if err != nil {
			httpapi.WriteError(w, r, 500, "oauth_unavailable", "oauth is unavailable")
			return
		}
		a, err := s.oauth.BeginWithPKCE(pkce, auth.StateOptions{RedirectURI: s.oauth.Config.RedirectURI, BrowserBinding: binding, DeviceName: "browser", DeviceType: "desktop"})
		if err != nil {
			httpapi.WriteError(w, r, 500, "oauth_unavailable", "oauth is unavailable")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: oauthBrowserCookie, Value: binding, Path: "/", MaxAge: int(s.oauth.Config.StateTTL / time.Second), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, a.URL, http.StatusFound)
		return
	}
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	defer r.Body.Close()
	var req SSOStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid sso request")
		return
	}
	req.CodeChallenge, req.RedirectURI, req.DeviceName, req.DeviceType, req.PublicKey = strings.TrimSpace(req.CodeChallenge), strings.TrimSpace(req.RedirectURI), strings.TrimSpace(req.DeviceName), strings.TrimSpace(req.DeviceType), strings.TrimSpace(req.PublicKey)
	if req.RedirectURI == "" {
		req.RedirectURI = s.oauth.Config.RedirectURI
	}
	if req.CodeChallenge == "" || req.DeviceName == "" || len(req.DeviceName) > 128 || (req.DeviceType != "desktop" && req.DeviceType != "mobile") || len(req.PublicKey) > 4096 || req.RedirectURI != s.oauth.Config.RedirectURI {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid sso request")
		return
	}
	a, err := s.oauth.BeginWithOptions(req.CodeChallenge, auth.StateOptions{RedirectURI: req.RedirectURI, DeviceName: req.DeviceName, DeviceType: req.DeviceType, PublicKey: req.PublicKey})
	if err != nil {
		httpapi.WriteError(w, r, 500, "oauth_unavailable", "oauth is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SSOStartResponse{AuthorizationURL: a.URL, State: a.State, ExpiresIn: int64(s.oauth.Config.StateTTL / time.Second)})
}

func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if s.oauth == nil {
		httpapi.WriteError(w, r, 404, "oauth_unavailable", "oauth is unavailable")
		return
	}
	if r.Method == http.MethodGet {
		// Expire the transaction cookie on every callback attempt, including
		// malformed or forged callbacks, to prevent stale browser transactions.
		http.SetCookie(w, &http.Cookie{Name: oauthBrowserCookie, Value: "", Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		q := r.URL.Query()
		state, code := strings.TrimSpace(q.Get("state")), strings.TrimSpace(q.Get("code"))
		cookie, err := r.Cookie(oauthBrowserCookie)
		nonce := strings.TrimSpace(q.Get("nonce"))
		if err != nil || state == "" || code == "" || nonce == "" || len(code) > 2048 {
			httpapi.WriteError(w, r, http.StatusUnauthorized, "oauth_invalid", "oauth callback is invalid")
			return
		}
		callback, identity, err := s.oauth.ExchangeBrowser(r.Context(), state, code, nonce, cookie.Value)
		if err != nil {
			httpapi.WriteError(w, r, 401, "oauth_invalid", "oauth callback is invalid")
			return
		}
		s.issueSSOSession(w, r, callback, identity, true)
		return
	}
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "method not allowed")
		return
	}
	defer r.Body.Close()
	var req SSOCallbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid sso callback")
		return
	}
	req.State, req.Code, req.CodeVerifier = strings.TrimSpace(req.State), strings.TrimSpace(req.Code), strings.TrimSpace(req.CodeVerifier)
	if req.State == "" || req.Code == "" || len(req.Code) > 2048 || req.CodeVerifier == "" {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid sso callback")
		return
	}
	callback, identity, err := s.oauth.Exchange(r.Context(), req.State, req.Code, req.CodeVerifier)
	if err != nil {
		httpapi.WriteError(w, r, 401, "oauth_invalid", "oauth callback is invalid")
		return
	}
	s.issueSSOSession(w, r, callback, identity, false)
	return
}

func (s *Server) issueSSOSession(w http.ResponseWriter, r *http.Request, callback auth.CallbackState, identity auth.Identity, redirect bool) {
	if s.accountService == nil || s.store == nil {
		httpapi.WriteError(w, r, 503, "auth_unavailable", "authentication is unavailable")
		return
	}
	account, err := s.accountService.EnsureAccount(r.Context(), identity.Provider, identity.Subject, identity.DisplayName)
	if err != nil {
		httpapi.WriteError(w, r, 401, "identity_invalid", "identity is invalid")
		return
	}
	deviceID := ""
	if callback.DeviceName != "" {
		deviceID = store.NewDeviceID()
	}
	credentials, err := s.accountService.IssueSession(r.Context(), account.ID, deviceID)
	if err != nil {
		httpapi.WriteError(w, r, 500, "auth_unavailable", "authentication is unavailable")
		return
	}
	response := map[string]any{"tokenType": "Bearer", "expiresIn": int64(s.accountService.Config.AccessTokenTTL / time.Second), "accessToken": credentials.AccessToken, "refreshToken": credentials.RefreshToken, "accessExpiresAt": credentials.AccessExpiresAt, "refreshExpiresAt": credentials.ExpiresAt, "sessionId": credentials.Session.ID, "accountId": account.ID}
	if deviceID != "" {
		deviceToken, e := authn.GenerateToken()
		if e != nil {
			_ = s.accountService.RevokeSession(r.Context(), credentials.Session.ID)
			httpapi.WriteError(w, r, 500, "auth_unavailable", "authentication is unavailable")
			return
		}
		if e = s.store.AddDevice(store.Device{ID: deviceID, AccountID: account.ID, TokenHash: authn.HashToken(deviceToken), Name: callback.DeviceName, Type: callback.DeviceType, PublicKey: callback.PublicKey, CreatedAt: time.Now().UTC()}); e != nil {
			_ = s.accountService.RevokeSession(r.Context(), credentials.Session.ID)
			httpapi.WriteError(w, r, 503, "auth_unavailable", "device registration unavailable")
			return
		}
		response["deviceToken"], response["deviceId"] = deviceToken, deviceID
	}
	w.Header().Set("Cache-Control", "no-store")
	if redirect {
		// Redirect only to the exact operator-configured deep link. Without one,
		// render a token-free completion page rather than looping to callback URI.
		if target := strings.TrimSpace(s.oauth.Config.DeepLinkURI); target != "" {
			u := target
			sep := "?"
			if strings.Contains(u, "?") {
				sep = "&"
			}
			u += sep + "login=success"
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Login complete. You may close this window."))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
