package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"relay-server/internal/auth"
)

func TestNativeSSOGrantPersistenceFailsClosed(t *testing.T) {
	s, upstream, _ := newNativeOAuthTestServer(t)
	defer upstream.Close()
	s.eveGrants = nil
	pkce, _ := auth.NewPKCE()
	started := startNativeOAuth(t, s, pkce)
	callback := httptest.NewRecorder()
	s.Handler().ServeHTTP(callback, httptest.NewRequest("GET", "/api/v1/auth/sso/callback?state="+url.QueryEscape(started.State)+"&code=one-time", nil))
	response := completeNativeOAuth(t, s, started.State, pkce.Verifier)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "never-returned-refresh") || strings.Contains(body, "accessToken") || strings.Contains(body, "refreshToken") {
		t.Fatal("credentials leaked when grant persistence was unavailable")
	}
}
