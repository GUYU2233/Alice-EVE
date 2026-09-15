package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"relay-server/internal/auth"
)

func TestSSOCallbackIssuesAccountAndDeviceSession(t *testing.T) {
	var upstreamTokenSeen bool
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "never-returned"})
			return
		}
		if r.Header.Get("Authorization") == "Bearer never-returned" {
			upstreamTokenSeen = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "eve-char-1", "name": "Pilot"})
	}))
	defer upstream.Close()
	s := NewServer()
	s.oauth, _ = auth.NewService(auth.OAuthConfig{AuthorizationEndpoint: "https://login.example/authorize", TokenEndpoint: upstream.URL + "/token", UserinfoEndpoint: upstream.URL + "/userinfo", ClientID: "public", RedirectURI: "https://desktop.example/cb", HTTPClient: upstream.Client()})
	pkce, _ := auth.NewPKCE()
	startBody, _ := json.Marshal(SSOStartRequest{CodeChallenge: pkce.Challenge, DeviceName: "desktop", DeviceType: "desktop"})
	start := httptest.NewRecorder()
	s.Handler().ServeHTTP(start, httptest.NewRequest("POST", "/api/v1/auth/sso/start", bytes.NewReader(startBody)))
	if start.Code != http.StatusOK {
		t.Fatalf("start=%d %s", start.Code, start.Body.String())
	}
	var started SSOStartResponse
	_ = json.NewDecoder(start.Body).Decode(&started)
	callbackBody, _ := json.Marshal(SSOCallbackRequest{State: started.State, Code: "one-time", CodeVerifier: pkce.Verifier})
	callback := httptest.NewRecorder()
	s.Handler().ServeHTTP(callback, httptest.NewRequest("POST", "/api/v1/auth/sso/callback", bytes.NewReader(callbackBody)))
	if callback.Code != http.StatusOK {
		t.Fatalf("callback=%d %s", callback.Code, callback.Body.String())
	}
	var response map[string]any
	_ = json.NewDecoder(callback.Body).Decode(&response)
	for _, key := range []string{"accessToken", "refreshToken", "deviceToken", "deviceId", "accountId"} {
		if response[key] == nil || response[key] == "" {
			t.Fatalf("missing %s: %v", key, response)
		}
	}
	if response["accessToken"] == "never-returned" || !upstreamTokenSeen {
		t.Fatal("identity exchange did not use ephemeral upstream token safely")
	}
	if _, err := s.accountService.AuthenticateAccess(context.Background(), response["accessToken"].(string)); err != nil {
		t.Fatal(err)
	}
}
