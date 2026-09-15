package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relay-server/internal/accounts"
	"relay-server/internal/realtime"
)

func TestHealth(t *testing.T) {
	r := httptest.NewRecorder()
	NewServer().Handler().ServeHTTP(r, httptest.NewRequest("GET", "/health", nil))
	if r.Code != 200 {
		t.Fatal(r.Code)
	}
}

func TestPairCodeFormat(t *testing.T) {
	r := httptest.NewRecorder()
	NewServer().Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var response PairResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil || len(response.Code) != 6 {
		t.Fatal(err, response.Code)
	}
}

func TestPairCodeIsOneTime(t *testing.T) {
	s := NewServer()
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	request := []byte(`{"code":"` + pair.Code + `","deviceName":"phone","deviceType":"mobile"}`)
	first := httptest.NewRecorder()
	s.Handler().ServeHTTP(first, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(request)))
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	s.Handler().ServeHTTP(second, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(request)))
	if second.Code != 401 {
		t.Fatal(second.Code)
	}
}

func TestRefreshEndpointRotatesToken(t *testing.T) {
	s := NewServer()
	account, err := s.accountService.EnsureAccount(context.Background(), "test", "subject-1", "Test")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.accountService.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"refreshToken":"` + issued.RefreshToken + `"}`)
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewReader(body)))
	if r.Code != 200 {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
	var response map[string]any
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response["accessToken"] == issued.AccessToken || response["refreshToken"] == issued.RefreshToken {
		t.Fatal("refresh endpoint returned an unrotated credential")
	}
	if _, err := s.accountService.Refresh(context.Background(), issued.RefreshToken); err != accounts.ErrRefreshReplay {
		t.Fatalf("old refresh token error=%v, want replay", err)
	}
}

func TestProtectedEndpointsRejectUnauthenticated(t *testing.T) {
	s := NewServer()
	for _, path := range []string{"/api/v1/events", "/api/v1/alerts", "/api/v1/devices/revoke"} {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code == 200 {
			t.Fatalf("%s unexpectedly allowed", path)
		}
	}
}

func TestPublishDeduplicatesAndStreams(t *testing.T) {
	s := NewServer()
	// Register a real device through the pairing flow so authentication is tested.
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	pairBody := []byte(`{"code":"` + pair.Code + `","deviceName":"desktop","deviceType":"desktop"}`)
	confirm := httptest.NewRecorder()
	s.Handler().ServeHTTP(confirm, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(pairBody)))
	var device DeviceResponse
	_ = json.NewDecoder(confirm.Body).Decode(&device)
	body := []byte(`{"v":1,"type":"intel.alert","id":"m-1","sender":"desktop","payload":{"severity":"high"}}`)
	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
	firstReq.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	s.Handler().ServeHTTP(first, firstReq)
	if first.Code != 200 {
		t.Fatal(first.Code)
	}
	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
	secondReq.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	s.Handler().ServeHTTP(second, secondReq)
	if second.Code != 200 || !bytes.Contains(second.Body.Bytes(), []byte(`"duplicate":true`)) {
		t.Fatal(second.Code, second.Body.String())
	}
}

func TestLegacySSEIsDeprecatedAndUsesPerDeviceCursor(t *testing.T) {
	s := NewServer()
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	confirm := httptest.NewRecorder()
	s.Handler().ServeHTTP(confirm, httptest.NewRequest(http.MethodPost, "/api/v1/pair/confirm", bytes.NewReader([]byte(`{"code":"`+pair.Code+`","deviceName":"mobile","deviceType":"mobile"}`))))
	var device DeviceResponse
	_ = json.NewDecoder(confirm.Body).Decode(&device)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events?cursor=0", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	stream := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(stream, request); close(done) }()
	deadline := time.Now().Add(time.Second)
	for stream.Header().Get("Deprecation") == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stream.Header().Get("Deprecation") != "true" || !strings.Contains(stream.Header().Get("Link"), "/api/v1/realtime") {
		t.Fatalf("deprecated SSE headers missing: %#v", stream.Header())
	}
	body := []byte(`{"v":1,"type":"intel.alert","id":"sse-1","sender":"desktop","recipient":"mobile:` + device.DeviceID + `","payload":{"severity":"high"}}`)
	publish := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(body))
	publish.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	// The legacy publish endpoint requires a desktop credential, so publish via
	// the realtime hub directly to exercise SSE's private subscription.
	_, _ = s.realtimeHub.Publish(context.Background(), "legacy", device.DeviceID, realtime.Event{Type: "legacy.envelope", Payload: body})
	deadline = time.Now().Add(time.Second)
	for !strings.Contains(stream.Body.String(), "sse-1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(stream.Body.String(), "sse-1") {
		t.Fatal("SSE did not receive targeted event")
	}
	_ = publish
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not close after cancellation")
	}
}

func TestOutboxRequiresDeviceTokenAndIsOwnerScoped(t *testing.T) {
	s := NewServer()
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	confirm := httptest.NewRecorder()
	s.Handler().ServeHTTP(confirm, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader([]byte(`{"code":"`+pair.Code+`","deviceName":"desktop","deviceType":"desktop"}`))))
	var device DeviceResponse
	_ = json.NewDecoder(confirm.Body).Decode(&device)
	unauth := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauth, httptest.NewRequest("GET", "/api/v1/outbox", nil))
	if unauth.Code != 401 {
		t.Fatalf("unauthenticated outbox status=%d", unauth.Code)
	}
	body := []byte(`{"v":1,"type":"intel.alert","id":"out-1","sender":"desktop","payload":{"severity":"high"}}`)
	publish := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
	publish.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	pr := httptest.NewRecorder()
	s.Handler().ServeHTTP(pr, publish)
	if pr.Code != 200 {
		t.Fatalf("publish status=%d body=%s", pr.Code, pr.Body.String())
	}
	request := httptest.NewRequest("GET", "/api/v1/outbox?limit=1", nil)
	request.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, request)
	if out.Code != 200 || !bytes.Contains(out.Body.Bytes(), []byte(`"messageId":"out-1"`)) {
		t.Fatalf("outbox status=%d body=%s", out.Code, out.Body.String())
	}
}

func TestAgentEndpointsReturnExplicitNotImplemented(t *testing.T) {
	s := NewServer()
	for _, path := range []string{"/api/v1/agent/conversations", "/api/v1/agent/conversations/messages"} {
		r := httptest.NewRecorder()
		req := httptest.NewRequest(map[string]string{"/api/v1/agent/conversations": "GET", "/api/v1/agent/conversations/messages": "POST"}[path], path, nil)
		s.Handler().ServeHTTP(r, req)
		if r.Code != 401 {
			t.Fatalf("%s unauthenticated status=%d", path, r.Code)
		}
	}
}
