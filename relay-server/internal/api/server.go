package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"relay-server/internal/accounts"
	"relay-server/internal/auth"
	"relay-server/internal/authn"
	"relay-server/internal/conversations"
	"relay-server/internal/httpapi"
	"relay-server/internal/notifications"
	"relay-server/internal/realtime"
	"relay-server/internal/store"
)

type Envelope struct {
	Version   int       `json:"v"`
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	TS        time.Time `json:"ts"`
	Sender    string    `json:"sender"`
	Recipient string    `json:"recipient,omitempty"`
	Payload   any       `json:"payload"`
}
type PairResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type PairConfirmRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
	PublicKey  string `json:"publicKey,omitempty"`
}
type DeviceResponse struct {
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken"`
}
type Server struct {
	mu               sync.Mutex
	store            store.Store
	seen             map[string]bool
	pairs            map[string]PairResponse
	mux              *http.ServeMux
	oauth            *auth.Service
	accountRepo      accounts.AccountRepository
	sessionRepo      accounts.SessionRepository
	accountService   *accounts.Service
	notifications    *notifications.Service
	conversationRepo conversations.ConversationRepository
	conversationAuth *conversationAuthorizer
	conversationSvc  *conversations.Service
	realtimeHub      realtime.DeliveryHub
	outboxWorker     *realtime.OutboxWorker
	outboxNotifier   realtime.Notifier
	challengeRepo    authn.ChallengeRepository
	deviceChallenges map[string]deviceChallenge
	handler          http.Handler
	handlerOnce      sync.Once
}

type syncResponse struct {
	Messages []Envelope `json:"messages"`
	Cursor   int64      `json:"cursor"`
	HasMore  bool       `json:"hasMore,omitempty"`
}

func (s *Server) ConfigureFCM(dispatcher notifications.Dispatcher) error {
	if dispatcher == nil {
		return nil
	}
	return s.notifications.SetDispatcher(notifications.ProviderFCM, dispatcher)
}

func NewServer(st ...store.Store) *Server {
	var backend store.Store = store.NewMemory()
	if len(st) > 0 && st[0] != nil {
		backend = st[0]
	}
	var accountRepo accounts.AccountRepository = accounts.NewMemoryRepository()
	var sessionRepo accounts.SessionRepository = accountRepo.(accounts.SessionRepository)
	// Keep account/session state on the same durable backend in production. The
	// in-memory repository remains the default for local development and tests.
	if pg, ok := backend.(*store.PostgresStore); ok {
		if durableRepo, repoErr := accounts.NewPostgresRepository(pg.Pool); repoErr == nil {
			accountRepo, sessionRepo = durableRepo, durableRepo
		}
	}
	accountService, err := accounts.NewService(accountRepo, sessionRepo, accounts.Config{})
	if err != nil {
		// NewService only rejects nil repositories; keeping this guard makes the
		// constructor robust if that validation grows in the future.
		panic(err)
	}
	var convRepo conversations.ConversationRepository = conversations.NewMemoryRepository()
	if pg, ok := backend.(*store.PostgresStore); ok {
		if durable, repoErr := conversations.NewPostgresRepository(pg.Pool); repoErr == nil {
			convRepo = durable
		}
	}
	convAuth := &conversationAuthorizer{server: nil}
	challengeRepo := authn.ChallengeRepository(authn.NewMemoryChallengeRepository())
	notificationService := notifications.NewService()
	if pg, ok := backend.(*store.PostgresStore); ok {
		if durable, repoErr := authn.NewPostgresChallengeRepository(pg.Pool); repoErr == nil {
			challengeRepo = durable
		}
		if durable, repoErr := notifications.NewPostgresRepository(pg.Pool); repoErr == nil {
			notificationService = notifications.NewServiceWithRepository(durable)
		}
	}
	var realtimeHub *realtime.Hub
	var outboxWorker *realtime.OutboxWorker
	var outboxNotifier realtime.Notifier
	if pg, ok := backend.(*store.PostgresStore); ok {
		if delivery, repoErr := realtime.NewPostgresDeliveryRepository(pg.Pool); repoErr == nil {
			// LISTEN/NOTIFY has one consumer per notifier. Use separate dedicated
			// connections for hub replay and outbox dispatch wakeups.
			var hubNotifier realtime.Notifier
			if notifier, notifyErr := realtime.NewPostgresNotifier(context.Background(), pg.Pool); notifyErr == nil {
				hubNotifier = notifier
			}
			if notifier, notifyErr := realtime.NewPostgresNotifier(context.Background(), pg.Pool); notifyErr == nil {
				outboxNotifier = notifier
			}
			if hubNotifier != nil {
				realtimeHub = realtime.NewHubWithConfigRepositoryAndNotifier(realtime.DefaultConfig(), delivery, hubNotifier)
			} else {
				realtimeHub = realtime.NewHubWithDeliveryRepository(delivery)
			}
			if durableOutbox, ok := convRepo.(*conversations.PostgresRepository); ok {
				dispatcher := &realtime.OutboxDispatcher{Outbox: durableOutbox, Delivery: delivery, Notifier: outboxNotifier}
				outboxWorker = realtime.NewOutboxWorker(dispatcher, func() <-chan realtime.Wakeup {
					if outboxNotifier == nil {
						return nil
					}
					return outboxNotifier.Wakeups()
				}(), time.Second)
				outboxWorker.Start(context.Background())
			}
		}
	}
	if realtimeHub == nil {
		realtimeHub = realtime.NewHub()
	}
	s := &Server{store: backend, seen: map[string]bool{}, pairs: map[string]PairResponse{}, deviceChallenges: map[string]deviceChallenge{}, challengeRepo: challengeRepo, mux: http.NewServeMux(), accountRepo: accountRepo, accountService: accountService, notifications: notificationService, conversationRepo: convRepo, conversationAuth: convAuth, conversationSvc: conversations.NewService(convRepo, convAuth), realtimeHub: realtimeHub, outboxWorker: outboxWorker, outboxNotifier: outboxNotifier}
	convAuth.server = s
	s.conversationSvc.SetRunEventPublisher(&conversationRunPublisher{server: s})
	// OAuth is opt-in. The server validates state/PKCE but never exchanges or stores EVE tokens.
	if endpoint := strings.TrimSpace(os.Getenv("EVE_SSO_AUTHORIZATION_ENDPOINT")); endpoint != "" {
		if oauthService, err := auth.NewService(auth.OAuthConfig{
			AuthorizationEndpoint: endpoint,
			TokenEndpoint:         os.Getenv("EVE_SSO_TOKEN_ENDPOINT"),
			UserinfoEndpoint:      os.Getenv("EVE_SSO_USERINFO_ENDPOINT"),
			ClientID:              os.Getenv("EVE_SSO_CLIENT_ID"),
			RedirectURI:           os.Getenv("EVE_SSO_REDIRECT_URI"),
			DeepLinkURI:           os.Getenv("EVE_SSO_DEEP_LINK_URI"),
			Provider:              "eve",
			Scopes:                strings.Fields(os.Getenv("EVE_SSO_SCOPES")),
		}); err == nil {
			s.oauth = oauthService
		}
	}
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/api/v1/auth/oauth/authorize", s.oauthAuthorize)
	s.mux.HandleFunc("/api/v1/auth/oauth/callback", s.oauthCallback)
	s.mux.HandleFunc("/api/v1/auth/sso/start", s.ssoStart)
	s.mux.HandleFunc("/api/v1/auth/sso/callback", s.ssoCallback)
	s.mux.HandleFunc("/api/v1/auth/device/start", s.deviceAuthStart)
	s.mux.HandleFunc("/api/v1/auth/device/complete", s.deviceAuthComplete)
	s.mux.HandleFunc("/api/v1/auth/device/revoke", s.revokeAccountDevice)
	s.mux.HandleFunc("/api/v1/account/devices", s.accountDevices)
	s.mux.HandleFunc("/api/v1/account/devices/revoke", s.revokeAccountDeviceByID)
	s.mux.HandleFunc("/api/v1/account/devices/revoke-all", s.revokeAllAccountDevices)
	s.mux.HandleFunc("/api/v1/auth/refresh", s.refreshSession)
	s.mux.HandleFunc("/api/v1/auth/token/refresh", s.refreshSession)
	s.mux.HandleFunc("/api/v1/pair", s.pair)
	s.mux.HandleFunc("/api/v1/pair/confirm", s.confirmPair)
	s.mux.HandleFunc("/api/v1/events", s.eventsStream)
	s.mux.HandleFunc("/api/v1/alerts", s.alerts)
	s.mux.HandleFunc("/api/v1/outbox", s.outbox)
	s.mux.HandleFunc("/api/v1/agent/conversations", s.agentConversations)
	s.mux.HandleFunc("/api/v1/agent/conversations/messages", s.agentConversationMessages)
	s.mux.HandleFunc("/api/v1/conversations", s.conversationsCollection)
	s.mux.HandleFunc("/api/v1/conversations/reconcile", s.conversationReconcile)
	s.mux.HandleFunc("/api/v1/conversations/", s.conversationHandler)
	s.mux.HandleFunc("/api/v1/realtime/snapshot", s.realtimeSnapshot)
	s.mux.HandleFunc("/api/v2/realtime/snapshot", s.realtimeSnapshot)
	s.mux.HandleFunc("/api/v1/realtime", s.realtimeWebSocket)
	s.mux.HandleFunc("/api/v1/ws", s.realtimeWebSocket)
	s.mux.HandleFunc("/api/v1/sync", s.sync)
	s.mux.HandleFunc("/api/v1/messages", s.publish)
	s.mux.HandleFunc("/api/v1/messages/ack", s.ack)
	s.mux.HandleFunc("/api/v1/devices/revoke", s.revoke)
	s.mux.HandleFunc("/api/v1/notifications/preferences", s.notificationPreferences)
	s.mux.HandleFunc("/api/v1/notifications/providers", s.notificationProviders)
	s.mux.HandleFunc("/api/v1/notifications/tokens", s.notificationTokens)
	return s
}

// Close releases background realtime resources. It does not close the backing
// store; the process owner remains responsible for that lifecycle.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.outboxWorker != nil {
		_ = s.outboxWorker.Close()
	}
	if s.outboxNotifier != nil {
		_ = s.outboxNotifier.Close()
	}
	if hub, ok := s.realtimeHub.(*realtime.Hub); ok {
		return hub.Close()
	}
	return nil
}

func (s *Server) Handler() http.Handler {
	s.handlerOnce.Do(func() {
		origins := strings.FieldsFunc(os.Getenv("CORS_ALLOWED_ORIGINS"), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
		s.handler = httpapi.RequestID(httpapi.CORS(httpapi.CORSOptions{
			AllowedOrigins: origins,
			AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowedHeaders: []string{"Accept", "Content-Type", "Authorization", "X-Device-Id", httpapi.RequestIDHeader},
			ExposeHeaders:  []string{httpapi.RequestIDHeader},
		})(httpapi.BodyLimit(httpapi.MaxBodyBytes)(httpapi.RateLimitByIP(httpapi.NewRateLimiter(240, time.Minute))(s.mux))))
	})
	return s.handler
}

type OAuthAuthorizeResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
	State            string `json:"state"`
	CodeVerifier     string `json:"codeVerifier"`
}

type OAuthCallbackRequest struct {
	State        string `json:"state"`
	Code         string `json:"code"`
	CodeVerifier string `json:"codeVerifier"`
}

func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || s.oauth == nil {
		http.Error(w, "oauth unavailable", http.StatusNotFound)
		return
	}
	authorization, err := s.oauth.Begin()
	if err != nil {
		http.Error(w, "oauth unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(OAuthAuthorizeResponse{AuthorizationURL: authorization.URL, State: authorization.State, CodeVerifier: authorization.CodeVerifier})
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || s.oauth == nil {
		http.Error(w, "oauth unavailable", http.StatusNotFound)
		return
	}
	defer r.Body.Close()
	var req OAuthCallbackRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
		http.Error(w, "invalid oauth callback", http.StatusBadRequest)
		return
	}
	callback, err := s.oauth.Complete(req.State, req.Code, req.CodeVerifier)
	if err != nil {
		http.Error(w, "invalid oauth callback", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "validated", "redirectUri": callback.RedirectURI})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		http.Error(w, "random unavailable", 500)
		return
	}
	code := fmt.Sprintf("%06d", (int64(raw[0])<<16|int64(raw[1])<<8|int64(raw[2]))%1000000)
	response := PairResponse{Code: code, ExpiresAt: time.Now().Add(5 * time.Minute).UTC()}
	if err := s.store.AddPair(r.Context(), store.PairCode{Code: code, ExpiresAt: response.ExpiresAt}); err != nil {
		http.Error(w, "pairing unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
func (s *Server) confirmPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	defer r.Body.Close()
	var req PairConfirmRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil || len(req.Code) != 6 || !pairingCode(req.Code) || req.DeviceName == "" || len(req.DeviceName) > 128 || (req.DeviceType != "desktop" && req.DeviceType != "mobile") || len(req.PublicKey) > 4096 {
		http.Error(w, "invalid pairing", 400)
		return
	}
	if _, ok := s.store.ConsumePair(r.Context(), req.Code); !ok {
		http.Error(w, "pairing code expired or not found", 401)
		return
	}
	deviceID := store.NewDeviceID()
	rawToken := store.NewDeviceID() + store.NewDeviceID()
	digest := sha256.Sum256([]byte(rawToken))
	if err := s.store.AddDevice(store.Device{ID: deviceID, AccountID: "legacy", TokenHash: hex.EncodeToString(digest[:]), Name: req.DeviceName, Type: req.DeviceType, PublicKey: req.PublicKey, CreatedAt: time.Now().UTC()}); err != nil {
		http.Error(w, "device registration unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(DeviceResponse{DeviceID: deviceID, DeviceToken: rawToken})
}
func bearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
func (s *Server) authDevice(r *http.Request, typ string) (string, bool) {
	token := bearerToken(r)
	if token == "" {
		return "", false
	}
	// Modern access tokens are account sessions. Keep the old stable device
	// token projection for compatibility with existing clients.
	if s.accountService != nil {
		if session, err := s.accountService.AuthenticateAccess(r.Context(), token); err == nil && session.DeviceID != "" {
			if deviceType, ok := s.store.DeviceType(session.DeviceID); ok && deviceType == typ {
				return session.DeviceID, true
			}
		}
	}
	if !s.store.VerifyTokenType(token, typ) {
		return "", false
	}
	id, ok := s.store.DeviceIDForToken(token)
	return id, ok
}

// authAnyDevice authenticates a bearer credential issued to either a mobile or
// desktop device. The returned device ID is always derived from the token;
// callers must not trust a device ID supplied in query parameters or bodies.
func (s *Server) authAnyDevice(r *http.Request) (string, bool) {
	// Keep the accepted credential classes explicit. VerifyToken alone would
	// also admit future/non-device credentials, which are not valid for this
	// device-scoped compatibility projection.
	for _, typ := range []string{"mobile", "desktop"} {
		if id, ok := s.authDevice(r, typ); ok {
			return id, true
		}
	}
	return "", false
}
func pairingCode(code string) bool {
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(code) == 6
}

func recipientDevice(recipient string) string {
	for _, prefix := range []string{"mobile:", "device:"} {
		if strings.HasPrefix(recipient, prefix) {
			return strings.TrimPrefix(recipient, prefix)
		}
	}
	return ""
}
func (s *Server) publish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	senderDeviceID, ok := s.authDevice(r, "desktop")
	if !ok {
		http.Error(w, "invalid device token", 401)
		return
	}
	accountID, _ := s.store.DeviceAccountID(senderDeviceID)
	// Legacy /pair devices have no account association yet. Keep their
	// compatibility queue usable; account-bound sessions still carry accountID.
	if r.ContentLength > 1<<20 {
		http.Error(w, "request too large", 413)
		return
	}
	defer r.Body.Close()
	var env Envelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&env); err != nil || env.Version != 1 || env.ID == "" || env.Type == "" || env.Sender != "desktop" {
		http.Error(w, "invalid envelope", 400)
		return
	}
	owner := recipientDevice(env.Recipient)
	// Legacy clients may omit recipient; preserve compatibility by assigning
	// those messages to the authenticated desktop device.
	if owner == "" {
		owner, _ = s.store.DeviceIDForToken(bearerToken(r))
	}
	if owner == "" {
		http.Error(w, "recipient device not found", http.StatusNotFound)
		return
	}
	if accountID != "" {
		if _, ok := s.store.GetDeviceForAccount(accountID, owner); !ok {
			http.Error(w, "recipient device not found", http.StatusNotFound)
			return
		}
	} else if !s.store.DeviceExists(owner) {
		http.Error(w, "recipient device not found", http.StatusNotFound)
		return
	}
	body, err := json.Marshal(env)
	if err != nil {
		http.Error(w, "invalid envelope", 400)
		return
	}
	exists, err := s.store.HasMessage(r.Context(), accountID, env.ID)
	if err != nil {
		http.Error(w, "store unavailable", 503)
		return
	}
	duplicate := exists
	s.mu.Lock()
	if !duplicate {
		s.seen[env.ID] = true
	}
	s.mu.Unlock()
	if !duplicate {
		if err := s.store.PutMessage(r.Context(), store.Message{ID: env.ID, AccountID: accountID, Body: string(body), OwnerDeviceID: owner}); err != nil {
			s.mu.Lock()
			delete(s.seen, env.ID)
			s.mu.Unlock()
			http.Error(w, "store unavailable", 503)
			return
		}
		// The compatibility SSE endpoint consumes this per-device hub stream.
		// There is intentionally no process-wide event channel to drain. A
		// non-fatal publish error is recoverable through /sync after persistence.
		_, _ = s.realtimeHub.Publish(r.Context(), accountID, owner, realtime.Event{ID: env.ID, Type: "legacy.envelope", Payload: body})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "duplicate": duplicate, "id": env.ID})
}
func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	deviceID, ok := s.authDevice(r, "mobile")
	if !ok {
		http.Error(w, "invalid device token", 401)
		return
	}
	defer r.Body.Close()
	var req struct {
		ID           string `json:"id"`
		MessageID    string `json:"messageId"`
		AckMessageID string `json:"ackMessageId"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		http.Error(w, "invalid id", 400)
		return
	}
	id := req.ID
	if id == "" {
		id = req.MessageID
	}
	if id == "" {
		id = req.AckMessageID
	}
	if id == "" {
		http.Error(w, "invalid id", 400)
		return
	}
	accountID, _ := s.store.DeviceAccountID(deviceID)
	acked, err := s.store.AckMessage(r.Context(), accountID, id, deviceID)
	if err != nil {
		http.Error(w, "store unavailable", 503)
		return
	}
	if !acked {
		http.Error(w, "message not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"acked": true, "id": id})
}

func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var v struct {
		DeviceID string `json:"deviceId"`
	}
	if token == "" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&v) != nil || !s.store.OwnsToken(token, v.DeviceID) || !s.store.RevokeDevice(v.DeviceID) {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) alerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	deviceID, ok := s.authDevice(r, "mobile")
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid device token")
		return
	}
	s.writeSync(w, r, deviceID)
}
func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	deviceID, ok := s.authAnyDevice(r)
	if !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid device token")
		return
	}
	s.writeSync(w, r, deviceID)
}
func (s *Server) writeSync(w http.ResponseWriter, r *http.Request, deviceID string) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	accountID, _ := s.store.DeviceAccountID(deviceID)
	rows, err := s.store.ListMessages(r.Context(), accountID, deviceID, after, limit+1)
	if err != nil {
		http.Error(w, "store unavailable", 503)
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]Envelope, 0, len(rows))
	cursor := after
	for _, m := range rows {
		var env Envelope
		if json.Unmarshal([]byte(m.Body), &env) == nil {
			out = append(out, env)
			if m.Cursor > cursor {
				cursor = m.Cursor
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(syncResponse{Messages: out, Cursor: cursor, HasMore: hasMore})
}

// eventsStream is the deprecated compatibility SSE projection. New clients
// should use /api/v1/realtime (WebSocket) or /api/v1/sync. It subscribes to a
// private per-device cursor stream; it never drains a process-wide channel.
func (s *Server) eventsStream(w http.ResponseWriter, r *http.Request) {
	deviceID, ok := s.authDevice(r, "mobile")
	if !ok {
		http.Error(w, "invalid device token", http.StatusUnauthorized)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	}
	if after < 0 {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}
	accountID, ok := s.store.DeviceAccountID(deviceID)
	if !ok || accountID == "" {
		http.Error(w, "device account unavailable", http.StatusUnauthorized)
		return
	}
	sub, err := s.realtimeHub.Subscribe(r.Context(), accountID, deviceID, after)
	if err != nil {
		if errors.Is(err, realtime.ErrCursorExpired) {
			http.Error(w, "cursor expired; use /api/v1/sync", http.StatusConflict)
			return
		}
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	defer sub.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "2026-12-31")
	w.Header().Set("Link", "</api/v1/realtime>; rel=\"successor\"")
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	ready, _ := json.Marshal(Envelope{Version: 1, Type: "ready", TS: time.Now().UTC(), Sender: "relay"})
	_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", after, ready)
	f.Flush()
	for {
		select {
		case event, open := <-sub.Events():
			if !open {
				return
			}
			var env Envelope
			if err := json.Unmarshal(event.Payload, &env); err != nil {
				continue
			}
			b, _ := json.Marshal(env)
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.Cursor, b)
			f.Flush()
		case <-sub.Done():
			return
		case <-r.Context().Done():
			return
		}
	}
}
