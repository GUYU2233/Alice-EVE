package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

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

type RelayClient struct {
	mu         sync.RWMutex
	url, token string
	client     *http.Client
	secrets    storage.SecretStore
}

func NewRelayClient() *RelayClient {
	return &RelayClient{client: &http.Client{Timeout: 8 * time.Second}}
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
func (r *RelayClient) SetURL(u string) error {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
		return fmt.Errorf("invalid relay URL")
	}
	r.mu.Lock()
	r.url = u
	r.mu.Unlock()
	return nil
}
func (r *RelayClient) URL() string       { r.mu.RLock(); defer r.mu.RUnlock(); return r.url }
func (r *RelayClient) SetToken(t string) { r.mu.Lock(); r.token = t; r.mu.Unlock() }
func (r *RelayClient) auth(req *http.Request) {
	r.mu.RLock()
	t := r.token
	r.mu.RUnlock()
	if t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
}
func (r *RelayClient) Health(ctx context.Context) error {
	return r.do(ctx, http.MethodGet, "/health", nil, nil)
}
func (r *RelayClient) do(ctx context.Context, m, p string, body []byte, out interface{}) error {
	u := r.URL()
	if u == "" {
		return &RelayError{Kind: "config", Err: fmt.Errorf("relay URL is not configured")}
	}
	req, e := http.NewRequestWithContext(ctx, m, u+p, bytes.NewReader(body))
	if e != nil {
		return &RelayError{Kind: "request", Err: e}
	}
	req.Header.Set("Content-Type", "application/json")
	r.auth(req)
	resp, e := r.client.Do(req)
	if e != nil {
		kind := "network"
		if ctx.Err() != nil {
			kind = "timeout"
		}
		return &RelayError{Kind: kind, Err: e}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &RelayError{Kind: "http", Status: resp.StatusCode}
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

// ConfirmPairing exchanges a pairing code for desktop credentials when the relay supports it.
// It deliberately requires both device_id and token before mutating local credentials.
func (r *RelayClient) ConfirmPairing(ctx context.Context, code string) (string, error) {
	if strings.TrimSpace(code) == "" {
		return "", fmt.Errorf("pairing code is required")
	}
	body, err := json.Marshal(struct {
		Code string `json:"code"`
	}{Code: code})
	if err != nil {
		return "", err
	}
	var v struct {
		Token    string `json:"token"`
		DeviceID string `json:"deviceId"`
	}
	if err = r.do(ctx, http.MethodPost, "/api/v1/pair/confirm", body, &v); err != nil {
		return "", err
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
