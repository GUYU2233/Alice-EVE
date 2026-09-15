package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestExchangeUsesPKCEAndDoesNotExposeUpstreamToken(t *testing.T) {
	var gotVerifier string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			gotVerifier = r.Form.Get("code_verifier")
			if r.Form.Get("client_secret") != "" {
				t.Error("client secret sent")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "upstream-secret"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Error("missing upstream bearer")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "123", "name": "Capsuleer"})
	}))
	defer srv.Close()
	s, err := NewService(OAuthConfig{AuthorizationEndpoint: "https://login.example/authorize", TokenEndpoint: srv.URL + "/token", UserinfoEndpoint: srv.URL + "/userinfo", ClientID: "public", RedirectURI: "https://client.example/cb", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := NewPKCE()
	a, err := s.BeginWithChallenge(c.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	_, id, err := s.Exchange(context.Background(), a.State, "code", c.Verifier)
	if err != nil {
		t.Fatal(err)
	}
	if id.Provider != "eve" || id.Subject != "123" || id.DisplayName != "Capsuleer" || gotVerifier != c.Verifier {
		t.Fatalf("identity=%+v verifier=%q", id, gotVerifier)
	}
	if strings.Contains(a.URL, url.QueryEscape("upstream-secret")) {
		t.Fatal("upstream token leaked")
	}
}

func TestExchangeUsesBasicAuthForConfidentialClientAndPKCE(t *testing.T) {
	var sawBasic, sawVerifier bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			user, password, ok := r.BasicAuth()
			sawBasic = ok && user == "client-id" && password == "deployment-secret"
			_ = r.ParseForm()
			sawVerifier = r.Form.Get("code_verifier") != "" && r.Form.Get("client_secret") == ""
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "upstream"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "123", "name": "Pilot"})
	}))
	defer srv.Close()
	s, err := NewService(OAuthConfig{AuthorizationEndpoint: "https://login.example/authorize", TokenEndpoint: srv.URL + "/token", UserinfoEndpoint: srv.URL + "/userinfo", ClientID: "client-id", ClientSecret: "deployment-secret", RedirectURI: "https://client.example/cb", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	pkce, _ := NewPKCE()
	started, _ := s.BeginWithChallenge(pkce.Challenge)
	if _, _, err := s.Exchange(context.Background(), started.State, "code", pkce.Verifier); err != nil {
		t.Fatal(err)
	}
	if !sawBasic || !sawVerifier {
		t.Fatalf("basic=%v verifier=%v", sawBasic, sawVerifier)
	}
}
