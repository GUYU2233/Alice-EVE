package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"relay-server/internal/auth"
)

func newNativeOAuthTestServer(t *testing.T) (*Server, *httptest.Server, *bool) {
	t.Helper()
	upstreamTokenSeen := false
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
	s := NewServer()
	var err error
	s.oauth, err = auth.NewService(auth.OAuthConfig{AuthorizationEndpoint: "https://login.example/authorize", TokenEndpoint: upstream.URL + "/token", UserinfoEndpoint: upstream.URL + "/userinfo", ClientID: "public", RedirectURI: "https://desktop.example/cb", HTTPClient: upstream.Client()})
	if err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	return s, upstream, &upstreamTokenSeen
}

func startNativeOAuth(t *testing.T, s *Server, pkce auth.Challenge) SSOStartResponse {
	t.Helper()
	startBody, _ := json.Marshal(SSOStartRequest{CodeChallenge: pkce.Challenge, DeviceName: "desktop", DeviceType: "desktop"})
	start := httptest.NewRecorder()
	s.Handler().ServeHTTP(start, httptest.NewRequest("POST", "/api/v1/auth/sso/start", bytes.NewReader(startBody)))
	if start.Code != http.StatusOK {
		t.Fatalf("start=%d %s", start.Code, start.Body.String())
	}
	var result SSOStartResponse
	if err := json.NewDecoder(start.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func completeNativeOAuth(t *testing.T, s *Server, state, verifier string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(SSOCallbackRequest{State: state, CodeVerifier: verifier})
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/auth/sso/callback", bytes.NewReader(body)))
	return response
}

func TestNativeSSOCallbackStagesThenCompletesOnce(t *testing.T) {
	s, upstream, upstreamTokenSeen := newNativeOAuthTestServer(t)
	defer upstream.Close()
	pkce, _ := auth.NewPKCE()
	started := startNativeOAuth(t, s, pkce)

	pending := completeNativeOAuth(t, s, started.State, pkce.Verifier)
	if pending.Code != http.StatusAccepted || !strings.Contains(pending.Body.String(), "authorization_pending") {
		t.Fatalf("pending=%d %s", pending.Code, pending.Body.String())
	}

	callback := httptest.NewRecorder()
	callbackURL := "/api/v1/auth/sso/callback?state=" + url.QueryEscape(started.State) + "&code=one-time"
	s.Handler().ServeHTTP(callback, httptest.NewRequest("GET", callbackURL, nil))
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "登录授权已完成") {
		t.Fatalf("browser callback=%d %s", callback.Code, callback.Body.String())
	}

	completed := completeNativeOAuth(t, s, started.State, pkce.Verifier)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete=%d %s", completed.Code, completed.Body.String())
	}
	var response map[string]any
	_ = json.NewDecoder(completed.Body).Decode(&response)
	for _, key := range []string{"accessToken", "refreshToken", "deviceToken", "deviceId", "accountId"} {
		if response[key] == nil || response[key] == "" {
			t.Fatalf("missing %s: %v", key, response)
		}
	}
	if response["accessToken"] == "never-returned" || !*upstreamTokenSeen {
		t.Fatal("identity exchange did not use ephemeral upstream token safely")
	}
	if _, err := s.accountService.AuthenticateAccess(context.Background(), response["accessToken"].(string)); err != nil {
		t.Fatal(err)
	}

	replay := completeNativeOAuth(t, s, started.State, pkce.Verifier)
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
}

func TestNativeSSOWrongVerifierDoesNotConsumeTransaction(t *testing.T) {
	s, upstream, _ := newNativeOAuthTestServer(t)
	defer upstream.Close()
	pkce, _ := auth.NewPKCE()
	wrong, _ := auth.NewPKCE()
	started := startNativeOAuth(t, s, pkce)
	callback := httptest.NewRecorder()
	s.Handler().ServeHTTP(callback, httptest.NewRequest("GET", "/api/v1/auth/sso/callback?state="+url.QueryEscape(started.State)+"&code=one-time", nil))
	if callback.Code != http.StatusOK {
		t.Fatalf("callback=%d %s", callback.Code, callback.Body.String())
	}
	bad := completeNativeOAuth(t, s, started.State, wrong.Verifier)
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong verifier=%d %s", bad.Code, bad.Body.String())
	}
	good := completeNativeOAuth(t, s, started.State, pkce.Verifier)
	if good.Code != http.StatusOK {
		t.Fatalf("correct verifier after attack=%d %s", good.Code, good.Body.String())
	}
}
