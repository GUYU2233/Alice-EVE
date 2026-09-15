package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidState         = errors.New("invalid or expired oauth state")
	ErrInvalidRedirectURI   = errors.New("redirect URI is not allowed")
	ErrIdentityInvalid      = errors.New("oauth identity is invalid")
	ErrAuthorizationPending = errors.New("oauth authorization is pending")
)

// OAuthConfig contains public OAuth parameters. Client credentials are not
// supported: EVE SSO uses a public PKCE client and no client secret is ever
// read, logged or persisted by this service.
type OAuthConfig struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
	ClientID              string
	// ClientSecret is optional. Confidential EVE applications authenticate the
	// token request with HTTP Basic; public/native applications leave it empty
	// and rely on PKCE. It must come from deployment secret storage.
	ClientSecret string
	RedirectURI  string
	// DeepLinkURI is an exact, operator-configured post-login destination.
	// Request parameters are never used as a redirect target.
	DeepLinkURI string
	Provider    string
	Scopes      []string
	StateTTL    time.Duration
	HTTPClient  *http.Client
}

func (c OAuthConfig) validate() error {
	if c.AuthorizationEndpoint == "" || c.ClientID == "" || c.RedirectURI == "" {
		return errors.New("oauth endpoint, client id and redirect URI are required")
	}
	for _, endpoint := range []string{c.AuthorizationEndpoint, c.TokenEndpoint, c.UserinfoEndpoint} {
		if endpoint == "" {
			continue
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return errors.New("oauth endpoints must be HTTPS")
		}
	}
	r, err := url.Parse(c.RedirectURI)
	if err != nil || (r.Scheme != "https" && r.Scheme != "http") || r.Host == "" {
		return ErrInvalidRedirectURI
	}
	return nil
}

type StateStore struct {
	mu      sync.Mutex
	records map[string]stateRecord
	now     func() time.Time
}
type stateRecord struct {
	challenge, codeVerifier, nonceHash, nonce, redirectURI string
	deviceName, deviceType, publicKey                      string
	expiresAt                                              time.Time
	browserHash                                            string
	// authorizationCode is staged by the HTTPS callback for a native PKCE
	// transaction. It remains server-side until the initiating client proves
	// possession of the verifier, and is consumed exactly once.
	authorizationCode string
}
type CallbackState struct {
	RedirectURI string
	DeviceName  string
	DeviceType  string
	PublicKey   string
	Nonce       string
}

type BrowserCallback struct {
	CallbackState
	CodeVerifier string
}

type StateOptions struct {
	RedirectURI, DeviceName, DeviceType, PublicKey string
	// BrowserBinding is a random cookie value hashed before storage. It binds
	// the authorization transaction to the browser that initiated it.
	BrowserBinding string
}

func NewStateStore() *StateStore {
	return &StateStore{records: make(map[string]stateRecord), now: time.Now}
}
func (s *StateStore) Begin(challenge, redirectURI string, ttl time.Duration) (string, error) {
	return s.BeginWithOptions(challenge, StateOptions{RedirectURI: redirectURI}, ttl)
}
func (s *StateStore) BeginWithOptions(challenge string, options StateOptions, ttl time.Duration) (string, error) {
	return s.begin(challenge, "", "", options, ttl)
}
func (s *StateStore) begin(challenge, verifier, nonce string, options StateOptions, ttl time.Duration) (string, error) {
	if !validChallenge(challenge) {
		return "", errors.New("invalid PKCE challenge")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if options.RedirectURI == "" {
		return "", ErrInvalidRedirectURI
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", err
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	if verifier != "" && !validVerifier(verifier) {
		return "", errors.New("invalid PKCE verifier")
	}
	if nonce != "" && len(nonce) > 256 {
		return "", errors.New("invalid nonce")
	}
	now := s.now()
	s.mu.Lock()
	nonceHash := ""
	if nonce != "" {
		h := sha256.Sum256([]byte(nonce))
		nonceHash = hexHash(h[:])
	}
	browserHash := ""
	if options.BrowserBinding != "" {
		h := sha256.Sum256([]byte(options.BrowserBinding))
		browserHash = hexHash(h[:])
	}
	s.records[state] = stateRecord{challenge: challenge, codeVerifier: verifier, nonceHash: nonceHash, nonce: nonce, browserHash: browserHash, redirectURI: options.RedirectURI, deviceName: options.DeviceName, deviceType: options.DeviceType, publicKey: options.PublicKey, expiresAt: now.Add(ttl)}
	s.mu.Unlock()
	return state, nil
}
func (s *StateStore) StageAuthorizationCode(state, code string) error {
	if state == "" || code == "" || len(code) > 2048 {
		return ErrInvalidState
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[state]
	if !ok || !now.Before(record.expiresAt) || record.browserHash != "" || record.codeVerifier != "" || record.authorizationCode != "" {
		return ErrInvalidState
	}
	record.authorizationCode = code
	s.records[state] = record
	return nil
}

func (s *StateStore) ConsumeStaged(state, verifier string) (CallbackState, string, error) {
	if state == "" || !validVerifier(verifier) {
		return CallbackState{}, "", ErrInvalidState
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[state]
	if !ok || !now.Before(record.expiresAt) || record.browserHash != "" || record.codeVerifier != "" || VerifyPKCE(verifier, record.challenge) != nil {
		return CallbackState{}, "", ErrInvalidState
	}
	if record.authorizationCode == "" {
		return CallbackState{}, "", ErrAuthorizationPending
	}
	delete(s.records, state)
	return CallbackState{RedirectURI: record.redirectURI, DeviceName: record.deviceName, DeviceType: record.deviceType, PublicKey: record.publicKey}, record.authorizationCode, nil
}

func (s *StateStore) Consume(state, verifier string) (CallbackState, error) {
	return s.ConsumeBound(state, verifier, "", "")
}
func (s *StateStore) ConsumeBound(state, verifier, nonce, browserBinding string) (CallbackState, error) {
	if state == "" {
		return CallbackState{}, ErrInvalidState
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[state]
	if !ok || !now.Before(record.expiresAt) || VerifyPKCE(verifier, record.challenge) != nil {
		return CallbackState{}, ErrInvalidState
	}
	if record.codeVerifier != "" && subtle.ConstantTimeCompare([]byte(record.codeVerifier), []byte(verifier)) != 1 {
		return CallbackState{}, ErrInvalidState
	}
	if record.nonceHash != "" {
		if nonce == "" {
			nonce = record.nonce
		}
		h := sha256.Sum256([]byte(nonce))
		if subtle.ConstantTimeCompare([]byte(record.nonceHash), []byte(hexHash(h[:]))) != 1 {
			return CallbackState{}, ErrInvalidState
		}
	}
	if record.browserHash != "" {
		h := sha256.Sum256([]byte(browserBinding))
		if subtle.ConstantTimeCompare([]byte(record.browserHash), []byte(hexHash(h[:]))) != 1 {
			return CallbackState{}, ErrInvalidState
		}
	}
	// Consume only after every capability check succeeds. Invalid verifiers,
	// nonces, or browser bindings must never burn a legitimate transaction.
	delete(s.records, state)
	return CallbackState{RedirectURI: record.redirectURI, DeviceName: record.deviceName, DeviceType: record.deviceType, PublicKey: record.publicKey, Nonce: nonce}, nil
}
func hexHash(b []byte) string { return hex.EncodeToString(b) }

type Authorization struct{ URL, State, CodeVerifier, CodeChallenge, Nonce string }
type Service struct {
	Config OAuthConfig
	States *StateStore
}

func NewService(config OAuthConfig) (*Service, error) {
	if config.StateTTL <= 0 {
		config.StateTTL = 10 * time.Minute
	}
	if config.Provider == "" {
		config.Provider = "eve"
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	if len(config.Scopes) == 0 {
		config.Scopes = []string{"esi-location.read_location.v1"}
	}
	return &Service{Config: config, States: NewStateStore()}, nil
}
func (s *Service) Begin() (Authorization, error) {
	pkce, err := NewPKCE()
	if err != nil {
		return Authorization{}, err
	}
	return s.BeginWithPKCE(pkce, StateOptions{RedirectURI: s.Config.RedirectURI})
}
func (s *Service) BeginWithPKCE(pkce Challenge, options StateOptions) (Authorization, error) {
	if options.RedirectURI == "" {
		options.RedirectURI = s.Config.RedirectURI
	}
	if options.RedirectURI != s.Config.RedirectURI {
		return Authorization{}, ErrInvalidRedirectURI
	}
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return Authorization{}, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	state, err := s.States.begin(pkce.Challenge, pkce.Verifier, nonce, options, s.Config.StateTTL)
	if err != nil {
		return Authorization{}, err
	}
	u, err := url.Parse(s.Config.AuthorizationEndpoint)
	if err != nil {
		return Authorization{}, err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.Config.ClientID)
	q.Set("redirect_uri", options.RedirectURI)
	q.Set("scope", strings.Join(s.Config.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("nonce", nonce)
	u.RawQuery = q.Encode()
	return Authorization{URL: u.String(), State: state, CodeVerifier: pkce.Verifier, CodeChallenge: pkce.Challenge, Nonce: nonce}, nil
}
func (s *Service) BeginWithChallenge(challenge string) (Authorization, error) {
	return s.BeginWithOptions(challenge, StateOptions{RedirectURI: s.Config.RedirectURI})
}
func (s *Service) BeginWithOptions(challenge string, options StateOptions) (Authorization, error) {
	if options.RedirectURI == "" {
		options.RedirectURI = s.Config.RedirectURI
	}
	if options.RedirectURI != s.Config.RedirectURI {
		return Authorization{}, ErrInvalidRedirectURI
	}
	state, err := s.States.BeginWithOptions(challenge, options, s.Config.StateTTL)
	if err != nil {
		return Authorization{}, err
	}
	u, err := url.Parse(s.Config.AuthorizationEndpoint)
	if err != nil {
		return Authorization{}, err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.Config.ClientID)
	q.Set("redirect_uri", options.RedirectURI)
	q.Set("scope", strings.Join(s.Config.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return Authorization{URL: u.String(), State: state, CodeChallenge: challenge}, nil
}
func (s *Service) Complete(state, code, verifier string) (CallbackState, error) {
	if code == "" || len(code) > 2048 {
		return CallbackState{}, errors.New("authorization code is required")
	}
	return s.States.Consume(state, verifier)
}
func (s *Service) CompleteBound(state, code, verifier, nonce, browserBinding string) (CallbackState, error) {
	if code == "" || len(code) > 2048 {
		return CallbackState{}, errors.New("authorization code is required")
	}
	return s.States.ConsumeBound(state, verifier, nonce, browserBinding)
}

// ExchangeBrowser consumes a browser-bound transaction. The verifier remains
// server-side and is never accepted from the browser callback request.
func (s *Service) ExchangeBrowser(ctx context.Context, state, code, nonce, browserBinding string) (CallbackState, Identity, error) {
	if code == "" || len(code) > 2048 {
		return CallbackState{}, Identity{}, errors.New("authorization code is required")
	}
	s.States.mu.Lock()
	record, ok := s.States.records[state]
	if ok {
		recordVerifier := record.codeVerifier
		s.States.mu.Unlock()
		if recordVerifier == "" {
			return CallbackState{}, Identity{}, ErrInvalidState
		}
		callback, err := s.CompleteBound(state, code, recordVerifier, nonce, browserBinding)
		if err != nil {
			return CallbackState{}, Identity{}, err
		}
		return s.exchangeIdentity(ctx, callback, code, recordVerifier)
	}
	s.States.mu.Unlock()
	return CallbackState{}, Identity{}, ErrInvalidState
}

type Identity struct{ Provider, Subject, DisplayName string }
type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// Exchange consumes state and exchanges the short-lived authorization code for
// the minimum upstream identity. The upstream access token is held in memory
// only for this request and is never returned, logged or persisted.
func (s *Service) StageNativeCallback(state, code string) error {
	return s.States.StageAuthorizationCode(state, code)
}

func (s *Service) CompleteNative(ctx context.Context, state, verifier string) (CallbackState, Identity, error) {
	callback, code, err := s.States.ConsumeStaged(state, verifier)
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	return s.exchangeIdentity(ctx, callback, code, verifier)
}

func (s *Service) Exchange(ctx context.Context, state, code, verifier string) (CallbackState, Identity, error) {
	callback, err := s.Complete(state, code, verifier)
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	return s.exchangeIdentity(ctx, callback, code, verifier)
}
func (s *Service) exchangeIdentity(ctx context.Context, callback CallbackState, code, verifier string) (CallbackState, Identity, error) {
	if s.Config.TokenEndpoint == "" || s.Config.UserinfoEndpoint == "" {
		return CallbackState{}, Identity{}, errors.New("oauth identity exchange is not configured")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback.RedirectURI}, "client_id": {s.Config.ClientID}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if s.Config.ClientSecret != "" {
		req.SetBasicAuth(s.Config.ClientID, s.Config.ClientSecret)
	}
	resp, err := s.Config.HTTPClient.Do(req)
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	defer resp.Body.Close()
	var token tokenResponse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&token) != nil || token.AccessToken == "" || len(token.AccessToken) > 4096 {
		return CallbackState{}, Identity{}, errors.New("oauth token exchange failed")
	}
	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Config.UserinfoEndpoint, nil)
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	infoReq.Header.Set("Accept", "application/json")
	infoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	infoResp, err := s.Config.HTTPClient.Do(infoReq)
	if err != nil {
		return CallbackState{}, Identity{}, err
	}
	defer infoResp.Body.Close()
	if infoResp.StatusCode < 200 || infoResp.StatusCode >= 300 {
		return CallbackState{}, Identity{}, errors.New("oauth identity lookup failed")
	}
	var raw map[string]any
	if err := json.NewDecoder(io.LimitReader(infoResp.Body, 64<<10)).Decode(&raw); err != nil {
		return CallbackState{}, Identity{}, ErrIdentityInvalid
	}
	subject := firstString(raw, "sub", "subject", "character_id", "characterId", "CharacterID", "id")
	if subject == "" {
		return CallbackState{}, Identity{}, ErrIdentityInvalid
	}
	provider := s.Config.Provider
	if provider == "" {
		provider = "eve"
	}
	return callback, Identity{Provider: provider, Subject: subject, DisplayName: firstString(raw, "name", "character_name", "characterName", "CharacterName")}, nil
}
func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" && len(x) <= 512 {
					return strings.TrimSpace(x)
				}
			case float64:
				if x >= 0 && x <= 9e18 {
					return strconv.FormatInt(int64(x), 10)
				}
			}
		}
	}
	return ""
}
func validChallenge(challenge string) bool {
	if len(challenge) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil
}
func (c OAuthConfig) String() string { return fmt.Sprintf("oauth client %q", c.ClientID) }
