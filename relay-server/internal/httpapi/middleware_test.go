package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRequestIDAndErrorEnvelope(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromRequest(r); got != "client-id" {
			t.Fatalf("request id = %q", got)
		}
		WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid request")
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(RequestIDHeader, "client-id")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Header().Get(RequestIDHeader) != "client-id" || rr.Code != http.StatusBadRequest {
		t.Fatalf("status/id = %d/%q", rr.Code, rr.Header().Get(RequestIDHeader))
	}
	var response ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil || response.Error.RequestID != "client-id" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
}

func TestRequestIDRejectsHeaderInjection(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromRequest(r) == "bad\nvalue" {
			t.Fatal("accepted an invalid request ID")
		}
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(RequestIDHeader, "bad-value")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Header().Get(RequestIDHeader) != "bad-value" {
		t.Fatal("valid ID was not propagated")
	}
}

func TestCORSAllowlistAndPreflight(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := CORS(CORSOptions{AllowedOrigins: []string{"https://app.example"}, AllowedMethods: []string{"POST"}, AllowedHeaders: []string{"Authorization"}, MaxAge: 300})(next)
	allowed := httptest.NewRequest(http.MethodOptions, "/", nil)
	allowed.Header.Set("Origin", "https://app.example")
	allowed.Header.Set("Access-Control-Request-Method", "POST")
	allowed.Header.Set("Access-Control-Request-Headers", "Authorization")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, allowed)
	if rr.Code != http.StatusNoContent || rr.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("preflight = %d, headers = %#v", rr.Code, rr.Header())
	}
	denied := httptest.NewRequest(http.MethodGet, "/", nil)
	denied.Header.Set("Origin", "https://evil.example")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, denied)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("denied origin status = %d", rr.Code)
	}
}

func TestCORSNoOriginPassesThrough(t *testing.T) {
	called := false
	h := CORS(CORSOptions{})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(204) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called || rr.Code != 204 {
		t.Fatal("same-origin request did not pass through")
	}
}

func TestCORSNormalizesOptionsWithoutOrigin(t *testing.T) {
	h := CORS(CORSOptions{
		AllowedOrigins: []string{" https://app.example/ "},
		AllowedMethods: []string{" post ", "", "GET"},
		AllowedHeaders: []string{" Authorization ", ""},
		ExposeHeaders:  []string{" X-Request-ID ", ""},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "https://app.example")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "authorization")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("normalized preflight status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "POST, GET" {
		t.Fatalf("allow methods = %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Authorization" {
		t.Fatalf("allow headers = %q", got)
	}
}

func TestCORSWildcardDoesNotAllowCredentials(t *testing.T) {
	h := CORS(CORSOptions{AllowedOrigins: []string{"*"}, AllowCredentials: true})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://app.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wildcard credential status = %d", rr.Code)
	}
}

func TestRateLimitByIP(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	h := RateLimitByIP(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	second := httptest.NewRequest(http.MethodGet, "/", nil)
	second.RemoteAddr = "192.0.2.10:9999"
	for i, r := range []*http.Request{first, second} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if want := []int{204, 429}[i]; rr.Code != want {
			t.Fatalf("request %d status = %d", i, rr.Code)
		}
	}
}

func TestBodyLimitStreams(t *testing.T) {
	h := BodyLimit(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 8)
		_, err := r.Body.Read(buf)
		if err == nil {
			t.Fatal("expected MaxBytesReader error")
		}
		w.WriteHeader(204)
	}))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345"))
	r.ContentLength = -1
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != 204 {
		t.Fatal(rr.Code)
	}
}

func TestRedaction(t *testing.T) {
	headers := http.Header{"Authorization": {"Bearer secret"}, "X-Trace": {"ok"}}
	redacted := RedactHeaders(headers)
	if redacted["Authorization"][0] == "Bearer secret" || headers.Get("Authorization") != "Bearer secret" {
		t.Fatal("header redaction mutated or leaked secret")
	}
	u, _ := url.Parse("https://example.test/callback?code=secret&ok=1")
	if got := RedactURL(u); strings.Contains(got, "secret") || !strings.Contains(got, "ok=1") {
		t.Fatal("url was not redacted: " + got)
	}
}
