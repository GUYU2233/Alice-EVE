package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
)

type RelayClient struct {
	mu     sync.RWMutex
	url    string
	client *http.Client
}

func NewRelayClient() *RelayClient {
	return &RelayClient{client: &http.Client{Timeout: 8 * time.Second}}
}
func (r *RelayClient) SetURL(url string) error {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	if url == "" {
		return fmt.Errorf("relay URL cannot be empty")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("relay URL must start with http:// or https://")
	}
	r.mu.Lock()
	r.url = url
	r.mu.Unlock()
	return nil
}
func (r *RelayClient) URL() string { r.mu.RLock(); defer r.mu.RUnlock(); return r.url }
func (r *RelayClient) Health(ctx context.Context) error {
	u := r.URL()
	if u == "" {
		return fmt.Errorf("relay URL is not configured")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u+"/health", nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("relay health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("relay health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}
func (r *RelayClient) PairingCode() (string, error) {
	u := r.URL()
	if u == "" {
		return "", fmt.Errorf("relay URL is not configured")
	}
	req, e := http.NewRequest(http.MethodPost, u+"/api/v1/pair", nil)
	if e != nil {
		return "", e
	}
	resp, e := r.client.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pairing returned HTTP %d", resp.StatusCode)
	}
	var v struct {
		Code string `json:"code"`
	}
	if e = json.NewDecoder(resp.Body).Decode(&v); e != nil {
		return "", e
	}
	return v.Code, nil
}
func (r *RelayClient) Publish(ctx context.Context, alert protocol.IntelAlert) error {
	u := r.URL()
	if u == "" {
		return fmt.Errorf("relay URL is not configured")
	}
	data, _ := json.Marshal(alert)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, u+"/api/alerts", bytes.NewReader(data))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := r.client.Do(req)
	if e != nil {
		return fmt.Errorf("publish alert failed: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("publish alert returned HTTP %d", resp.StatusCode)
	}
	return nil
}
