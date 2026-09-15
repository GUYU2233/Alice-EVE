// Package notifications contains reminder preferences and push-token lifecycle
// abstractions. Raw provider tokens are accepted only at the registration
// boundary and are never retained by the service.
package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidDevice   = errors.New("device id is required")
	ErrInvalidToken    = errors.New("push token is invalid")
	ErrInvalidProvider = errors.New("push provider is invalid")
	ErrInvalidPlatform = errors.New("push platform is invalid for provider")
	ErrConflict        = errors.New("notification preferences are stale")
	ErrTokenNotFound   = errors.New("push token not found")
	// ErrProviderNotConfigured is returned when dispatch is requested but no
	// provider adapter has been configured. This package never makes vendor calls.
	ErrProviderNotConfigured = errors.New("push provider is not configured")
)

const (
	ProviderFCM     = "fcm"
	ProviderXiaomi  = "xiaomi"
	ProviderHuawei  = "huawei"
	ProviderOPPO    = "oppo"
	ProviderVivo    = "vivo"
	ProviderAPNs    = "apns"
	ProviderWebPush = "webpush"
)

// ProviderMetadata describes the supported provider/platform contract.
type ProviderMetadata struct {
	Provider           string   `json:"provider"`
	Platforms          []string `json:"platforms"`
	RequiresDispatcher bool     `json:"requiresDispatcher"`
	Configured         bool     `json:"configured"`
}

var providerPlatforms = map[string][]string{
	ProviderFCM: {"android", "ios", "web"}, ProviderXiaomi: {"android"},
	ProviderHuawei: {"android"}, ProviderOPPO: {"android"}, ProviderVivo: {"android"},
	ProviderAPNs: {"ios"}, ProviderWebPush: {"web"},
}

func ProviderMetadataList() []ProviderMetadata {
	providers := []string{ProviderFCM, ProviderXiaomi, ProviderHuawei, ProviderOPPO, ProviderVivo, ProviderAPNs, ProviderWebPush}
	out := make([]ProviderMetadata, 0, len(providers))
	for _, p := range providers {
		out = append(out, ProviderMetadata{Provider: p, Platforms: append([]string(nil), providerPlatforms[p]...), RequiresDispatcher: true})
	}
	return out
}

// Delivery is provider-neutral. Dispatchers receive only the already-hashed token.
type Delivery struct {
	Token PushToken         `json:"token"`
	Title string            `json:"title,omitempty"`
	Body  string            `json:"body,omitempty"`
	Data  map[string]string `json:"data,omitempty"`
}

type Dispatcher interface {
	Dispatch(context.Context, Delivery) error
}

type ReminderPreferences struct {
	Enabled             bool   `json:"enabled"`
	IntelAlerts         bool   `json:"intelAlerts"`
	MarketAlerts        bool   `json:"marketAlerts"`
	ConversationUpdates bool   `json:"conversationUpdates"`
	QuietHoursStart     string `json:"quietHoursStart,omitempty"`
	QuietHoursEnd       string `json:"quietHoursEnd,omitempty"`
	Timezone            string `json:"timezone,omitempty"`
	Sound               bool   `json:"sound"`
	Vibration           bool   `json:"vibration"`
}

func DefaultPreferences() ReminderPreferences {
	return ReminderPreferences{Enabled: true, IntelAlerts: true, MarketAlerts: true, ConversationUpdates: true, Sound: true, Vibration: true}
}

type PreferencesDocument struct {
	DeviceID    string              `json:"deviceId"`
	Version     int64               `json:"version"`
	ETag        string              `json:"etag"`
	Preferences ReminderPreferences `json:"preferences"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type UpdateRequest struct {
	DeviceID    string
	Preferences ReminderPreferences
	BaseVersion int64
	IfMatch     string
}

type PushToken struct {
	ID         string     `json:"id"`
	DeviceID   string     `json:"deviceId"`
	Provider   string     `json:"provider"`
	Platform   string     `json:"platform"`
	TokenHash  string     `json:"-"`
	AppVersion string     `json:"appVersion,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type TokenRegistration struct {
	DeviceID   string
	Provider   string
	Platform   string
	Token      string
	AppVersion string
}

type ConflictError struct{ Current PreferencesDocument }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%v: current version %d", ErrConflict, e.Current.Version)
}
func (e *ConflictError) Unwrap() error { return ErrConflict }

// Service is a concurrency-safe in-process implementation suitable for tests
// and single-instance deployments. Its interfaces are intentionally narrow so
// a durable repository can be substituted without changing the HTTP contract.
type Service struct {
	mu          sync.Mutex
	preferences map[string]PreferencesDocument
	tokens      map[string]PushToken
	dispatchers map[string]Dispatcher
	repo        Repository
	now         func() time.Time
}

func NewService() *Service {
	return &Service{preferences: map[string]PreferencesDocument{}, tokens: map[string]PushToken{}, dispatchers: map[string]Dispatcher{}, now: time.Now}
}

// NewServiceWithRepository uses durable storage while retaining the same
// concurrency-safe in-memory implementation for tests and local development.
func NewServiceWithRepository(repo Repository) *Service {
	s := NewService()
	s.repo = repo
	return s
}

// SetDispatcher configures the provider adapter. Passing nil removes it.
// Adapters must be provider-specific and must not require a raw token: the
// service only supplies Delivery.Token.TokenHash.
func (s *Service) SetDispatcher(provider string, d Dispatcher) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if _, ok := providerPlatforms[provider]; !ok {
		return ErrInvalidProvider
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d == nil {
		delete(s.dispatchers, provider)
	} else {
		s.dispatchers[provider] = d
	}
	return nil
}

// ProviderMetadata returns support and runtime configuration state.
func (s *Service) ProviderMetadata() []ProviderMetadata {
	out := ProviderMetadataList()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range out {
		out[i].Configured = s.dispatchers[out[i].Provider] != nil
	}
	return out
}

func (s *Service) GetPreferences(ctx context.Context, deviceID string) (PreferencesDocument, error) {
	if strings.TrimSpace(deviceID) == "" {
		return PreferencesDocument{}, ErrInvalidDevice
	}
	if s.repo != nil {
		return s.repo.GetPreferences(ctx, deviceID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.preferences[deviceID]; ok {
		return d, nil
	}
	now := s.now().UTC()
	p := DefaultPreferences()
	d := PreferencesDocument{DeviceID: deviceID, Version: 1, ETag: etag(1, p), Preferences: p, UpdatedAt: now}
	s.preferences[deviceID] = d
	return d, nil
}
func (s *Service) UpdatePreferences(ctx context.Context, req UpdateRequest) (PreferencesDocument, error) {
	if strings.TrimSpace(req.DeviceID) == "" {
		return PreferencesDocument{}, ErrInvalidDevice
	}
	if s.repo != nil {
		return s.repo.UpdatePreferences(ctx, req)
	}
	if req.BaseVersion < 1 {
		return PreferencesDocument{}, fmt.Errorf("%w: base version must be positive", ErrConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.preferences[req.DeviceID]
	if !ok {
		p := DefaultPreferences()
		cur = PreferencesDocument{DeviceID: req.DeviceID, Version: 1, ETag: etag(1, p), Preferences: p, UpdatedAt: s.now().UTC()}
		s.preferences[req.DeviceID] = cur
	}
	if cur.Version != req.BaseVersion || strings.Trim(strings.TrimSpace(req.IfMatch), "\"") != cur.ETag {
		return PreferencesDocument{}, &ConflictError{Current: cur}
	}
	cur.Version++
	cur.Preferences = req.Preferences
	cur.ETag = etag(cur.Version, cur.Preferences)
	cur.UpdatedAt = s.now().UTC()
	s.preferences[req.DeviceID] = cur
	return cur, nil
}
func etag(v int64, p ReminderPreferences) string {
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%d-%s", v, hex.EncodeToString(h[:]))
}

func (s *Service) RegisterToken(ctx context.Context, req TokenRegistration) (PushToken, error) {
	if strings.TrimSpace(req.DeviceID) == "" {
		return PushToken{}, ErrInvalidDevice
	}
	if s.repo != nil {
		return s.repo.RegisterToken(ctx, req)
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.Token = strings.TrimSpace(req.Token)
	platforms, ok := providerPlatforms[req.Provider]
	if !ok {
		return PushToken{}, ErrInvalidProvider
	}
	if len(req.Token) < 16 || len(req.Token) > 4096 {
		return PushToken{}, ErrInvalidToken
	}
	if req.Platform == "" {
		req.Platform = platforms[0]
	} else {
		valid := false
		for _, p := range platforms {
			if req.Platform == p {
				valid = true
				break
			}
		}
		if !valid {
			return PushToken{}, ErrInvalidPlatform
		}
	}
	h := sha256.Sum256([]byte(req.Token))
	hash := hex.EncodeToString(h[:])
	id := hash[:24]
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, t := range s.tokens {
		if t.DeviceID == req.DeviceID && t.TokenHash == hash && t.Provider == req.Provider {
			t.Platform = req.Platform
			t.AppVersion = strings.TrimSpace(req.AppVersion)
			t.UpdatedAt = now
			t.RevokedAt = nil
			s.tokens[key] = t
			return t, nil
		}
	}
	t := PushToken{ID: id, DeviceID: req.DeviceID, Provider: req.Provider, Platform: req.Platform, TokenHash: hash, AppVersion: strings.TrimSpace(req.AppVersion), CreatedAt: now, UpdatedAt: now}
	s.tokens[id] = t
	return t, nil
}
func (s *Service) RevokeToken(ctx context.Context, deviceID, tokenID, rawToken string) error {
	if strings.TrimSpace(deviceID) == "" {
		return ErrInvalidDevice
	}
	if s.repo != nil {
		return s.repo.RevokeToken(ctx, deviceID, tokenID, rawToken)
	}
	if strings.TrimSpace(tokenID) == "" && strings.TrimSpace(rawToken) != "" {
		h := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
		tokenID = hex.EncodeToString(h[:])[:24]
	}
	if strings.TrimSpace(tokenID) == "" {
		return ErrTokenNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok || t.DeviceID != deviceID || t.RevokedAt != nil {
		return ErrTokenNotFound
	}
	now := s.now().UTC()
	t.RevokedAt = &now
	t.UpdatedAt = now
	s.tokens[tokenID] = t
	return nil
}
func (s *Service) ListTokens(ctx context.Context, deviceID string) []PushToken {
	if s.repo != nil {
		out, _ := s.repo.ListTokens(ctx, deviceID)
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []PushToken{}
	for _, t := range s.tokens {
		if t.DeviceID == deviceID && t.RevokedAt == nil {
			// Do not expose token hashes, even to internal/API callers.
			t.TokenHash = ""
			out = append(out, t)
		}
	}
	return out
}

// Dispatch sends a provider-neutral payload through the configured provider
// adapter. It returns ErrProviderNotConfigured rather than pretending delivery
// occurred when credentials/adapter setup is absent.
func (s *Service) Dispatch(ctx context.Context, tokenID, deviceID, title, body string, data map[string]string) error {
	s.mu.Lock()
	t, ok := s.tokens[tokenID]
	d := s.dispatchers[t.Provider]
	s.mu.Unlock()
	if !ok || t.DeviceID != deviceID || t.RevokedAt != nil {
		return ErrTokenNotFound
	}
	if d == nil {
		return ErrProviderNotConfigured
	}
	// Deliberately pass only hash-backed identity; token hash remains private to
	// the service and is not serialized by PushToken's JSON contract.
	return d.Dispatch(ctx, Delivery{Token: t, Title: title, Body: body, Data: data})
}
