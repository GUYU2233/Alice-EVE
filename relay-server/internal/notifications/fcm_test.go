package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeAccessTokens struct {
	token string
	err   error
}

func (f fakeAccessTokens) AccessToken(context.Context) (string, error) { return f.token, f.err }

type fakeTokenResolver struct {
	token string
	hash  string
	err   error
}

func (f *fakeTokenResolver) ResolveToken(_ context.Context, hash string) (string, error) {
	f.hash = hash
	return f.token, f.err
}

type testMetrics struct {
	sent        atomic.Int32
	retries     atomic.Int32
	unavailable atomic.Int32
}

func (m *testMetrics) Inc(name string, labels map[string]string) {
	switch name {
	case "push_dispatch_retries_total":
		m.retries.Add(1)
	case "push_dispatch_total":
		if labels["result"] == "sent" {
			m.sent.Add(1)
		}
		if labels["result"] == "token_unavailable" {
			m.unavailable.Add(1)
		}
	}
}
func (*testMetrics) Observe(string, time.Duration, map[string]string) {}

func TestFCMConfigFromEnvRequiresSecretInjection(t *testing.T) {
	env := map[string]string{"FCM_ENABLED": "true", "FCM_PROJECT_ID": "eve-prod"}
	cfg, err := FCMConfigFromEnv(func(k string) string { return env[k] })
	if err != nil || cfg.ProjectID != "eve-prod" || cfg.Timeout <= 0 || cfg.MaxAttempts != 3 {
		t.Fatalf("config: %#v %v", cfg, err)
	}
	env["FCM_ACCESS_TOKEN"] = "should-not-be-configured"
	if _, err := FCMConfigFromEnv(func(k string) string { return env[k] }); !errors.Is(err, ErrFCMInvalidConfig) {
		t.Fatalf("expected credential boundary error, got %v", err)
	}
	delete(env, "FCM_ACCESS_TOKEN")
	env["FCM_ENABLED"] = "true"
	delete(env, "FCM_PROJECT_ID")
	if _, err := FCMConfigFromEnv(func(k string) string { return env[k] }); !errors.Is(err, ErrFCMInvalidConfig) {
		t.Fatalf("expected project id error, got %v", err)
	}
}

func TestFCMDispatcherSendsHTTPv1AndResolvesHashOnly(t *testing.T) {
	const raw = "fcm-raw-token-never-in-errors"
	var got fcmRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/v1/projects/eve/messages:send") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer oauth-token" {
			t.Errorf("missing auth")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	resolver := &fakeTokenResolver{token: raw}
	metrics := &testMetrics{}
	d, err := NewFCMDispatcher(FCMConfig{ProjectID: "eve", Endpoint: srv.URL, Timeout: time.Second, MaxAttempts: 1}, fakeAccessTokens{token: "oauth-token"}, resolver, srv.Client(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	hash := "hash-not-raw"
	err = d.Dispatch(context.Background(), Delivery{Token: PushToken{Provider: ProviderFCM, TokenHash: hash}, Title: "Alert", Body: "Body", Data: map[string]string{"kind": "intel"}})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.hash != hash || got.Message.Token != raw || got.Message.Notification.Title != "Alert" || got.Message.Data["kind"] != "intel" {
		t.Fatalf("unexpected resolver/request: %#v %#v", resolver.hash, got)
	}
	if metrics.sent.Load() != 1 {
		t.Fatalf("metrics sent=%d", metrics.sent.Load())
	}
}

func TestFCMDispatcherRetriesTemporaryFailuresAndHonorsContext(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metrics := &testMetrics{}
	d, err := NewFCMDispatcher(FCMConfig{ProjectID: "eve", Endpoint: srv.URL, Timeout: time.Second, MaxAttempts: 3, InitialBackoff: 0}, fakeAccessTokens{token: "oauth"}, &fakeTokenResolver{token: "raw-token"}, srv.Client(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Dispatch(context.Background(), Delivery{Token: PushToken{Provider: ProviderFCM, TokenHash: "hash"}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || metrics.retries.Load() != 2 {
		t.Fatalf("calls=%d retries=%d", calls.Load(), metrics.retries.Load())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Dispatch(ctx, Delivery{Token: PushToken{Provider: ProviderFCM, TokenHash: "hash"}}); err == nil {
		t.Fatal("expected canceled context")
	}
}

func TestFCMDispatcherDoesNotLeakRawTokenInErrors(t *testing.T) {
	const raw = "super-secret-fcm-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer srv.Close()
	d, err := NewFCMDispatcher(FCMConfig{ProjectID: "eve", Endpoint: srv.URL, Timeout: time.Second, MaxAttempts: 1}, fakeAccessTokens{token: "oauth"}, &fakeTokenResolver{token: raw}, srv.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	err = d.Dispatch(context.Background(), Delivery{Token: PushToken{Provider: ProviderFCM, TokenHash: "hash"}})
	if !errors.Is(err, ErrFCMRejected) || strings.Contains(err.Error(), raw) {
		t.Fatalf("unexpected/leaky error: %v", err)
	}
}
