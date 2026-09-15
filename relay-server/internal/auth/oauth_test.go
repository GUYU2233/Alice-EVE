package auth

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func testOAuthService(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(OAuthConfig{
		AuthorizationEndpoint: "https://login.eveonline.com/v2/oauth/authorize",
		ClientID:              "client-id",
		RedirectURI:           "https://desktop.example.test/oauth/callback",
		Scopes:                []string{"esi-location.read_location.v1", "esi-ui.open_window.v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAuthorizationURLContainsPKCEAndState(t *testing.T) {
	s := testOAuthService(t)
	a, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"response_type": "code", "client_id": "client-id", "redirect_uri": s.Config.RedirectURI,
		"state": a.State, "code_challenge": a.CodeChallenge, "code_challenge_method": "S256",
	} {
		if q.Get(key) != want {
			t.Fatalf("%s = %q, want %q", key, q.Get(key), want)
		}
	}
	if q.Get("scope") != strings.Join(s.Config.Scopes, " ") {
		t.Fatal("scopes not encoded")
	}
	if a.CodeVerifier == "" || a.CodeVerifier == a.CodeChallenge {
		t.Fatal("invalid verifier")
	}
}

func TestOAuthStateIsOneTimeAndPKCEBound(t *testing.T) {
	s := testOAuthService(t)
	a, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(a.State, "auth-code", a.CodeVerifier+"bad"); err == nil {
		t.Fatal("wrong verifier accepted")
	}
	if _, err := s.Complete(a.State, "auth-code", a.CodeVerifier); err == nil {
		t.Fatal("state reused after failed PKCE validation")
	}
}

func TestOAuthBrowserTransactionBindsNonceAndCookie(t *testing.T) {
	s := testOAuthService(t)
	pkce, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.BeginWithPKCE(pkce, StateOptions{RedirectURI: s.Config.RedirectURI, BrowserBinding: "cookie-binding"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("nonce") != a.Nonce || a.Nonce == "" {
		t.Fatalf("nonce not bound: %q", a.Nonce)
	}
	if _, err := s.CompleteBound(a.State, "code", pkce.Verifier, a.Nonce, "wrong-cookie"); err == nil {
		t.Fatal("wrong browser binding accepted")
	}
	if _, err := s.CompleteBound(a.State, "code", pkce.Verifier, a.Nonce, "cookie-binding"); err == nil {
		t.Fatal("consumed state accepted after failed binding")
	}
	b, err := s.BeginWithPKCE(pkce, StateOptions{RedirectURI: s.Config.RedirectURI, BrowserBinding: "cookie-binding"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteBound(b.State, "code", pkce.Verifier, "wrong-nonce", "cookie-binding"); err == nil {
		t.Fatal("wrong nonce accepted")
	}
}

func TestOAuthStateExpires(t *testing.T) {
	s := testOAuthService(t)
	s.States.now = func() time.Time { return time.Unix(100, 0) }
	a, err := s.States.Begin(NewChallenge(t), s.Config.RedirectURI, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	s.States.now = func() time.Time { return time.Unix(102, 0) }
	if _, err := s.States.Consume(a, NewVerifier(t)); err == nil {
		t.Fatal("expired state accepted")
	}
}

func NewChallenge(t *testing.T) string {
	t.Helper()
	c, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	return c.Challenge
}

func NewVerifier(t *testing.T) string {
	t.Helper()
	c, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	return c.Verifier
}
