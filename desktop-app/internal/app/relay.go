package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RelayError is retained as a source-compatibility name for the legacy client API.
type RelayError struct {
	Kind   string
	Status int
	Err    error
}

func (e *RelayError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("relay %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("relay %s (HTTP %d)", e.Kind, e.Status)
}
func (e *RelayError) Unwrap() error { return e.Err }

// RelayClient is the legacy compatibility facade for the Server API.
type RelayClient struct {
	mu                                      sync.RWMutex
	url, token                              string
	accessToken, refreshToken               string
	pendingOAuthState, pendingOAuthVerifier string
	client                                  *http.Client
	secrets                                 storage.SecretStore
	routeCache                              *routeClientCache
	entityCache                             *entityClientCache
}

func IsOAuthPending(err error) bool {
	var relayErr *RelayError
	return errors.As(err, &relayErr) && relayErr.Status == http.StatusAccepted
}

type OAuthCredentials struct {
	Pending          bool      `json:"pending,omitempty"`
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken"`
	DeviceToken      string    `json:"deviceToken,omitempty"`
	DeviceID         string    `json:"deviceId,omitempty"`
	AccountID        string    `json:"accountId,omitempty"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt,omitempty"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt,omitempty"`
}

type OAuthStartResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
	ExpiresIn        int64  `json:"expiresIn"`
}

func NewRelayClient() *RelayClient {
	return &RelayClient{client: &http.Client{Timeout: 8 * time.Second}, routeCache: newRouteClientCache(10*time.Minute, 2048), entityCache: newEntityClientCache(24*time.Hour, 4096)}
}
func NewRelayClientWithSecrets(secrets storage.SecretStore) *RelayClient {
	r := NewRelayClient()
	r.secrets = secrets
	return r
}
func (r *RelayClient) RestoreToken() error {
	if r.secrets == nil {
		return nil
	}
	if creds, e := r.secrets.Load("relay-oauth-credentials"); e == nil {
		var c OAuthCredentials
		if json.Unmarshal([]byte(creds), &c) == nil && c.AccessToken != "" {
			r.mu.Lock()
			r.accessToken, r.refreshToken = c.AccessToken, c.RefreshToken
			r.mu.Unlock()
		}
	}
	t, e := r.secrets.Load("relay-device-token")
	if e != nil {
		if errors.Is(e, storage.ErrSecretNotFound) {
			return nil
		}
		return e
	}
	r.SetToken(t)
	return nil
}
func (r *RelayClient) SaveOAuthCredentials(c OAuthCredentials) error {
	if strings.TrimSpace(c.AccessToken) == "" || strings.TrimSpace(c.RefreshToken) == "" {
		return fmt.Errorf("oauth access and refresh tokens are required")
	}
	if r.secrets == nil {
		return storage.ErrSecureStorageUnavailable
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := r.secrets.Save("relay-oauth-credentials", string(b)); err != nil {
		return err
	}
	r.mu.Lock()
	r.accessToken, r.refreshToken = c.AccessToken, c.RefreshToken
	r.mu.Unlock()
	return nil
}
func (r *RelayClient) RefreshToken(ctx context.Context) (OAuthCredentials, error) {
	r.mu.RLock()
	refresh, base := r.refreshToken, r.url
	r.mu.RUnlock()
	if refresh == "" {
		return OAuthCredentials{}, fmt.Errorf("refresh token unavailable")
	}
	body, _ := json.Marshal(map[string]string{"refreshToken": refresh})
	var out OAuthCredentials
	if err := r.doRequest(ctx, http.MethodPost, "/api/v1/auth/refresh", body, &out, false); err != nil {
		return OAuthCredentials{}, err
	}
	if err := r.SaveOAuthCredentials(out); err != nil {
		return OAuthCredentials{}, err
	}
	_ = base
	return out, nil
}
func (r *RelayClient) BeginOAuthPKCE(ctx context.Context, deviceName string) (OAuthStartResponse, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return OAuthStartResponse{}, "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	body, _ := json.Marshal(map[string]string{"codeChallenge": challenge, "deviceName": strings.TrimSpace(deviceName), "deviceType": "desktop"})
	var out OAuthStartResponse
	if err := r.doRequest(ctx, http.MethodPost, "/api/v1/auth/sso/start", body, &out, false); err != nil {
		return OAuthStartResponse{}, "", err
	}
	if out.AuthorizationURL == "" || out.State == "" {
		return OAuthStartResponse{}, "", fmt.Errorf("oauth authorization response is incomplete")
	}
	r.mu.Lock()
	r.pendingOAuthState, r.pendingOAuthVerifier = out.State, verifier
	r.mu.Unlock()
	return out, verifier, nil
}
func (r *RelayClient) CompletePendingOAuthPKCE(ctx context.Context) (OAuthCredentials, error) {
	r.mu.RLock()
	state, verifier := r.pendingOAuthState, r.pendingOAuthVerifier
	r.mu.RUnlock()
	return r.CompleteOAuthPKCE(ctx, state, "", verifier)
}

func (r *RelayClient) CompleteOAuthPKCE(ctx context.Context, state, code, verifier string) (OAuthCredentials, error) {
	if strings.TrimSpace(state) == "" || strings.TrimSpace(verifier) == "" {
		return OAuthCredentials{}, fmt.Errorf("oauth callback is incomplete")
	}
	r.mu.Lock()
	if r.pendingOAuthState == "" || state != r.pendingOAuthState || verifier != r.pendingOAuthVerifier {
		r.mu.Unlock()
		return OAuthCredentials{}, fmt.Errorf("oauth callback state is invalid")
	}
	r.mu.Unlock()
	body, _ := json.Marshal(map[string]string{"state": state, "code": code, "codeVerifier": verifier})
	var out OAuthCredentials
	if err := r.doRequest(ctx, http.MethodPost, "/api/v1/auth/sso/callback", body, &out, false); err != nil {
		return OAuthCredentials{}, err
	}
	// Consume the local callback capability only after a successful exchange.
	// Pending polls and transient network failures must remain retryable.
	r.mu.Lock()
	if r.pendingOAuthState == state && r.pendingOAuthVerifier == verifier {
		r.pendingOAuthState, r.pendingOAuthVerifier = "", ""
	}
	r.mu.Unlock()
	if err := r.SaveOAuthCredentials(out); err != nil {
		return OAuthCredentials{}, err
	}
	if out.DeviceToken != "" {
		_ = r.SetTokenSecure(out.DeviceToken)
	}
	return out, nil
}
func (r *RelayClient) RevokeAndClear(ctx context.Context, deviceID string) error {
	r.mu.RLock()
	token, access := r.token, r.accessToken
	r.mu.RUnlock()
	// OAuth sessions are revoked through the authenticated access token; legacy
	// device-token revocation remains supported for compatibility.
	if deviceID != "" && (token != "" || access != "") {
		body, _ := json.Marshal(map[string]string{"deviceId": deviceID})
		if access != "" {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL()+"/api/v1/auth/device/revoke", bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+access)
			resp, err := r.client.Do(req)
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("remote revoke failed")
			}
		} else if err := r.do(ctx, http.MethodPost, "/api/v1/devices/revoke", body, nil); err != nil {
			return err
		}
	}
	return r.ClearAllCredentials()
}
func (r *RelayClient) ClearAllCredentials() error {
	r.SetToken("")
	r.mu.Lock()
	r.accessToken, r.refreshToken = "", ""
	r.mu.Unlock()
	if r.secrets == nil {
		return nil
	}
	var first error
	for _, key := range []string{"relay-device-token", "relay-oauth-credentials"} {
		if err := r.secrets.Delete(key); err != nil && !errors.Is(err, storage.ErrSecretNotFound) && first == nil {
			first = err
		}
	}
	return first
}

func (r *RelayClient) SetTokenSecure(t string) error {
	if strings.TrimSpace(t) == "" {
		return fmt.Errorf("token cannot be empty")
	}
	if r.secrets != nil {
		if err := r.secrets.Save("relay-device-token", t); err != nil {
			return err
		}
	}
	r.SetToken(t)
	return nil
}
func (r *RelayClient) ClearToken() error {
	r.SetToken("")
	if r.secrets != nil {
		return r.secrets.Delete("relay-device-token")
	}
	return nil
}
func (r *RelayClient) SetURL(raw string) error {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid relay URL")
	}
	// Never forward bearer credentials over cleartext or to an opaque URL. HTTP
	// remains available only for loopback development servers used by tests.
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return fmt.Errorf("relay URL must use HTTPS (HTTP is allowed only on loopback)")
	}
	r.mu.Lock()
	r.url = raw
	r.mu.Unlock()
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}
func (r *RelayClient) URL() string       { r.mu.RLock(); defer r.mu.RUnlock(); return r.url }
func (r *RelayClient) SetToken(t string) { r.mu.Lock(); r.token = t; r.mu.Unlock() }
func (r *RelayClient) authHeader(header http.Header) {
	r.mu.RLock()
	t := r.accessToken
	if t == "" {
		t = r.token
	}
	r.mu.RUnlock()
	if t != "" {
		header.Set("Authorization", "Bearer "+t)
	}
}
func (r *RelayClient) auth(req *http.Request) { r.authHeader(req.Header) }
func (r *RelayClient) Health(ctx context.Context) error {
	return r.do(ctx, http.MethodGet, "/health", nil, nil)
}
func (r *RelayClient) do(ctx context.Context, m, p string, body []byte, out interface{}) error {
	return r.doRequest(ctx, m, p, body, out, true)
}

type refreshAttemptKey struct{}

func (r *RelayClient) doRequest(ctx context.Context, m, p string, body []byte, out interface{}, authenticated bool) error {
	u := r.URL()
	if u == "" {
		return &RelayError{Kind: "config", Err: fmt.Errorf("relay URL is not configured")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, e := http.NewRequestWithContext(ctx, m, u+p, bytes.NewReader(body))
	if e != nil {
		return &RelayError{Kind: "request", Err: e}
	}
	req.Header.Set("Content-Type", "application/json")
	if authenticated {
		r.auth(req)
	}
	resp, e := r.client.Do(req)
	if e != nil {
		kind := "network"
		if ctx.Err() != nil {
			kind = "timeout"
		}
		return &RelayError{Kind: kind, Err: e}
	}
	defer resp.Body.Close()
	// 202 means the asynchronous OAuth handoff has not completed yet. Treat it
	// as an explicit retryable condition rather than decoding an error envelope
	// as empty credentials and prematurely stopping desktop polling.
	if p == "/api/v1/auth/sso/callback" && resp.StatusCode == http.StatusAccepted {
		return &RelayError{Kind: "oauth_pending", Status: resp.StatusCode, Err: errors.New("oauth authorization is pending")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var detail struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&detail)
		message := detail.Error.Message
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		// Access tokens are short-lived while the desktop refresh credential is
		// durable. Refresh once and replay the authenticated request transparently.
		if authenticated && resp.StatusCode == http.StatusUnauthorized && p != "/api/v1/auth/refresh" && ctx.Value(refreshAttemptKey{}) == nil {
			if _, refreshErr := r.RefreshToken(ctx); refreshErr == nil {
				return r.doRequest(context.WithValue(ctx, refreshAttemptKey{}, true), m, p, body, out, authenticated)
			}
		}
		return &RelayError{Kind: "http", Status: resp.StatusCode, Err: fmt.Errorf("%s", message)}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
func (r *RelayClient) PairingCode() (string, error) {
	var v struct {
		Code string `json:"code"`
	}
	e := r.do(context.Background(), http.MethodPost, "/api/v1/pair", nil, &v)
	return v.Code, e
}

// ConfirmPairing exchanges a pairing code for desktop credentials when the server supports it.
// It deliberately requires both device_id and token before mutating local credentials.
func (r *RelayClient) ConfirmPairing(ctx context.Context, code string) (string, error) {
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("pairing code is required")
	}
	// Include the desktop identity fields required by the legacy server protocol. Older
	// server versions may ignore the additional fields, so this remains wire-compatible
	// with implementations that only consume the pairing code.
	body, err := json.Marshal(struct {
		Code       string `json:"code"`
		DeviceName string `json:"deviceName"`
		DeviceType string `json:"deviceType"`
	}{Code: code, DeviceName: "Alice-EVE Desktop", DeviceType: "desktop"})
	if err != nil {
		return "", err
	}
	var v struct {
		// token was used by the original desktop client contract. The server
		// currently returns deviceToken; accepting both keeps both contracts
		// interoperable during rollout.
		Token       string `json:"token"`
		DeviceToken string `json:"deviceToken"`
		DeviceID    string `json:"deviceId"`
	}
	if err = r.do(ctx, http.MethodPost, "/api/v1/pair/confirm", body, &v); err != nil {
		return "", err
	}
	if v.Token == "" {
		v.Token = v.DeviceToken
	}
	if v.Token == "" || v.DeviceID == "" {
		return "", fmt.Errorf("pairing response missing credentials")
	}
	if err = r.SetTokenSecure(v.Token); err != nil {
		return "", err
	}
	return v.DeviceID, nil
}
func (r *RelayClient) HasToken() bool { r.mu.RLock(); defer r.mu.RUnlock(); return r.token != "" }
func (r *RelayClient) PublishEnvelope(ctx context.Context, env protocol.EventEnvelope) error {
	if env.MessageID == "" {
		return fmt.Errorf("message id is required")
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	var ack struct {
		Accepted bool   `json:"accepted"`
		ID       string `json:"id"`
	}
	if err = r.do(ctx, http.MethodPost, "/api/v1/messages", b, &ack); err != nil {
		return err
	}
	if !ack.Accepted || ack.ID != env.MessageID {
		return fmt.Errorf("relay ACK mismatch for %s", env.MessageID)
	}
	return nil
}
func (r *RelayClient) Publish(ctx context.Context, a protocol.IntelAlert) error {
	id := make([]byte, 16)
	if _, e := rand.Read(id); e != nil {
		return e
	}
	env := protocol.EventEnvelope{Version: 1, MessageID: hex.EncodeToString(id), Sender: "desktop", Type: protocol.EventIntelAlert, CreatedAt: time.Now().UTC(), Payload: a}
	return r.PublishEnvelope(ctx, env)
}
