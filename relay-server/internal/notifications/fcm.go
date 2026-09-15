package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// FCMAccessTokenSource supplies short-lived OAuth access tokens. Production
// implementations should use workload identity or a Secret Manager-backed
// provider; this interface deliberately does not accept service-account JSON.
type FCMAccessTokenSource interface {
	AccessToken(context.Context) (string, error)
}

// FCMTokenResolver resolves a token hash to a provider token at send time. Raw
// FCM tokens must live in a dedicated secret/token vault, not this repository's
// database. Implementations must not log or persist the returned value.
type FCMTokenResolver interface {
	ResolveToken(context.Context, string) (string, error)
}

// Metrics is intentionally small so callers can bridge it to Prometheus,
// OpenTelemetry, or a hosted metrics service without coupling this package to a
// vendor SDK. Label values must not contain token material or request bodies.
type Metrics interface {
	Inc(name string, labels map[string]string)
	Observe(name string, value time.Duration, labels map[string]string)
}

type nopMetrics struct{}

func (nopMetrics) Inc(string, map[string]string)                    {}
func (nopMetrics) Observe(string, time.Duration, map[string]string) {}

// FCMConfig contains non-secret runtime configuration. Credentials are never
// represented in this struct. FCM_PROJECT_ID is required when FCM is enabled.
type FCMConfig struct {
	ProjectID      string
	Endpoint       string
	Timeout        time.Duration
	MaxAttempts    int
	InitialBackoff time.Duration
}

var (
	ErrFCMInvalidConfig    = errors.New("invalid fcm configuration")
	ErrFCMCredentials      = errors.New("fcm credentials unavailable")
	ErrFCMTokenUnavailable = errors.New("fcm token unavailable")
	ErrFCMRejected         = errors.New("fcm rejected notification")
)

func defaultFCMConfig() FCMConfig {
	return FCMConfig{Endpoint: "https://fcm.googleapis.com", Timeout: 8 * time.Second, MaxAttempts: 3, InitialBackoff: 200 * time.Millisecond}
}

// FCMConfigFromEnv reads only non-secret settings. It intentionally rejects
// FCM_ACCESS_TOKEN and GOOGLE_SERVICE_ACCOUNT_JSON so credentials cannot be
// accidentally configured through a checked-in env file. Secrets are injected
// through FCMAccessTokenSource instead.
func FCMConfigFromEnv(getenv func(string) string) (FCMConfig, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	// Reject credential-shaped environment variables even when FCM is disabled.
	// This prevents accidental secret injection through dotenv files or unit
	// files and keeps all credential acquisition behind explicit interfaces.
	for _, key := range []string{"FCM_ACCESS_TOKEN", "FCM_CLIENT_SECRET", "GOOGLE_SERVICE_ACCOUNT_JSON", "GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CREDENTIALS"} {
		if strings.TrimSpace(getenv(key)) != "" {
			return FCMConfig{}, fmt.Errorf("%w: credentials must be injected through Secret Manager/workload identity", ErrFCMInvalidConfig)
		}
	}
	cfg := defaultFCMConfig()
	cfg.ProjectID = strings.TrimSpace(getenv("FCM_PROJECT_ID"))
	enabledValue := strings.TrimSpace(getenv("FCM_ENABLED"))
	enabled := strings.EqualFold(enabledValue, "1") || strings.EqualFold(enabledValue, "true")
	if enabledValue == "" {
		// FCM is opt-in. A project id alone must not activate a provider during
		// a partial deployment or local development.
		return FCMConfig{}, nil
	}
	if !enabled {
		if !strings.EqualFold(enabledValue, "0") && !strings.EqualFold(enabledValue, "false") {
			return FCMConfig{}, fmt.Errorf("%w: FCM_ENABLED must be true or false", ErrFCMInvalidConfig)
		}
		return FCMConfig{}, nil
	}
	if cfg.ProjectID == "" {
		return FCMConfig{}, fmt.Errorf("%w: FCM_PROJECT_ID is required when FCM_ENABLED is true", ErrFCMInvalidConfig)
	}
	if strings.TrimSpace(getenv("FCM_ACCESS_TOKEN")) != "" || strings.TrimSpace(getenv("GOOGLE_SERVICE_ACCOUNT_JSON")) != "" {
		return FCMConfig{}, fmt.Errorf("%w: credentials must be injected through Secret Manager/workload identity", ErrFCMInvalidConfig)
	}
	if v := strings.TrimSpace(getenv("FCM_ENDPOINT")); v != "" {
		u, err := url.Parse(v)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return FCMConfig{}, fmt.Errorf("%w: FCM_ENDPOINT must be an https URL", ErrFCMInvalidConfig)
		}
		cfg.Endpoint = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(getenv("FCM_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 || d > 60*time.Second {
			return FCMConfig{}, fmt.Errorf("%w: invalid FCM_TIMEOUT", ErrFCMInvalidConfig)
		}
		cfg.Timeout = d
	}
	if v := strings.TrimSpace(getenv("FCM_MAX_ATTEMPTS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5 {
			return FCMConfig{}, fmt.Errorf("%w: invalid FCM_MAX_ATTEMPTS", ErrFCMInvalidConfig)
		}
		cfg.MaxAttempts = n
	}
	if v := strings.TrimSpace(getenv("FCM_INITIAL_BACKOFF")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 || d > 10*time.Second {
			return FCMConfig{}, fmt.Errorf("%w: invalid FCM_INITIAL_BACKOFF", ErrFCMInvalidConfig)
		}
		cfg.InitialBackoff = d
	}
	return cfg, nil
}

// FCMDispatcher sends through Firebase HTTP v1. It is an adapter only: no
// Google SDK, credential file, or network call is needed to construct it.
type FCMDispatcher struct {
	cfg     FCMConfig
	tokens  FCMTokenResolver
	auth    FCMAccessTokenSource
	client  *http.Client
	metrics Metrics
	sleep   func(context.Context, time.Duration) error
}

// NewFCMDispatcherFromEnv builds an adapter from non-secret environment
// settings. A nil dispatcher means FCM is intentionally disabled.
func NewFCMDispatcherFromEnv(getenv func(string) string, auth FCMAccessTokenSource, tokens FCMTokenResolver, client *http.Client, metrics Metrics) (*FCMDispatcher, error) {
	cfg, err := FCMConfigFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	if cfg.ProjectID == "" {
		return nil, nil
	}
	return NewFCMDispatcher(cfg, auth, tokens, client, metrics)
}

func NewFCMDispatcher(cfg FCMConfig, auth FCMAccessTokenSource, tokens FCMTokenResolver, client *http.Client, metrics Metrics) (*FCMDispatcher, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" || auth == nil || tokens == nil {
		return nil, ErrFCMInvalidConfig
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultFCMConfig().Endpoint
	}
	u, endpointErr := url.Parse(cfg.Endpoint)
	if endpointErr != nil || u.Host == "" || (u.Scheme != "https" && !isLoopbackHost(u.Hostname())) {
		return nil, fmt.Errorf("%w: endpoint must use https (loopback is allowed for tests)", ErrFCMInvalidConfig)
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultFCMConfig().Timeout
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultFCMConfig().MaxAttempts
	}
	if cfg.MaxAttempts > 5 {
		return nil, fmt.Errorf("%w: max attempts exceeds 5", ErrFCMInvalidConfig)
	}
	if cfg.InitialBackoff < 0 {
		return nil, ErrFCMInvalidConfig
	}
	if client == nil {
		client = &http.Client{}
	}
	if metrics == nil {
		metrics = nopMetrics{}
	}
	return &FCMDispatcher{cfg: cfg, auth: auth, tokens: tokens, client: client, metrics: metrics, sleep: sleepContext}, nil
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}
type fcmMessage struct {
	Token        string            `json:"token"`
	Notification *fcmNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
}
type fcmNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// Dispatch resolves the raw token only immediately before transmission and
// never includes it in errors or metrics. Context timeout covers all attempts.
func (d *FCMDispatcher) Dispatch(ctx context.Context, delivery Delivery) error {
	if delivery.Token.Provider != ProviderFCM {
		return fmt.Errorf("%w: provider %q", ErrFCMInvalidConfig, delivery.Token.Provider)
	}
	if delivery.Token.TokenHash == "" {
		return ErrFCMTokenUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, d.cfg.Timeout)
	defer cancel()
	raw, err := d.tokens.ResolveToken(ctx, delivery.Token.TokenHash)
	if err != nil || strings.TrimSpace(raw) == "" {
		d.metrics.Inc("push_dispatch_total", map[string]string{"provider": "fcm", "result": "token_unavailable"})
		// Do not wrap resolver errors: implementations may accidentally include
		// the raw token in their error text.
		return ErrFCMTokenUnavailable
	}
	access, err := d.auth.AccessToken(ctx)
	if err != nil || strings.TrimSpace(access) == "" {
		d.metrics.Inc("push_dispatch_total", map[string]string{"provider": "fcm", "result": "credentials_unavailable"})
		return ErrFCMCredentials
	}
	body, _ := json.Marshal(fcmRequest{Message: fcmMessage{Token: raw, Notification: &fcmNotification{Title: delivery.Title, Body: delivery.Body}, Data: delivery.Data}})
	endpoint := strings.TrimRight(d.cfg.Endpoint, "/") + "/v1/projects/" + url.PathEscape(d.cfg.ProjectID) + "/messages:send"
	started := time.Now()
	for attempt := 1; attempt <= d.cfg.MaxAttempts; attempt++ {
		result, retryAfter, reqErr := d.sendOnce(ctx, endpoint, access, body)
		if reqErr == nil {
			d.metrics.Inc("push_dispatch_total", map[string]string{"provider": "fcm", "result": "sent"})
			d.metrics.Observe("push_dispatch_duration", time.Since(started), map[string]string{"provider": "fcm"})
			return nil
		}
		if !result.retryable || attempt == d.cfg.MaxAttempts {
			d.metrics.Inc("push_dispatch_total", map[string]string{"provider": "fcm", "result": result.result})
			d.metrics.Observe("push_dispatch_duration", time.Since(started), map[string]string{"provider": "fcm"})
			return reqErr
		}
		d.metrics.Inc("push_dispatch_retries_total", map[string]string{"provider": "fcm"})
		backoff := d.cfg.InitialBackoff << (attempt - 1)
		if retryAfter > backoff {
			backoff = retryAfter
		}
		if err := d.sleep(ctx, backoff); err != nil {
			return err
		}
	}
	return context.Canceled // unreachable
}

type fcmResult struct {
	retryable bool
	result    string
}

func (d *FCMDispatcher) sendOnce(ctx context.Context, endpoint, access string, body []byte) (fcmResult, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fcmResult{retryable: false, result: "request_error"}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return fcmResult{retryable: ctx.Err() == nil, result: "transport_error"}, 0, fmt.Errorf("fcm request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return fcmResult{}, 0, nil
	}
	retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 500 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
	if retry {
		return fcmResult{retryable: true, result: "retry_exhausted"}, retryAfter(resp.Header.Get("Retry-After")), fmt.Errorf("fcm temporary failure (status %d)", resp.StatusCode)
	}
	return fcmResult{retryable: false, result: "rejected"}, 0, fmt.Errorf("%w (status %d)", ErrFCMRejected, resp.StatusCode)
}

func retryAfter(v string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 && n <= 60 {
		return time.Duration(n) * time.Second
	}
	return 0
}
