package eve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestESIPathAllowList(t *testing.T) {
	c := NewESIClient()
	for _, p := range []string{"https://evil.test/x", "//evil.test/x", "/latest/characters/1/?x=1", "/latest/../admin"} {
		if _, err := c.validatePath(p); err == nil {
			t.Fatalf("path accepted: %s", p)
		}
	}
	if u, err := c.validatePath("/latest/characters/1/"); err != nil || u != "https://esi.evetech.net/latest/characters/1/" {
		t.Fatalf("valid path: %s %v", u, err)
	}
}

func TestESIETagAndNotModified(t *testing.T) {
	var requests int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.Header().Set("Cache-Control", "max-age=60")
			w.Header().Set("ETag", `"v1"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := NewESIClient()
	c.BaseURL = srv.URL
	c.Client = srv.Client()
	c.CacheTTL = 0 // force validation request while retaining cache
	first, err := c.Get(context.Background(), "/latest/test/")
	if err != nil || first.ETag != `"v1"` {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := c.Get(context.Background(), "/latest/test/")
	if err != nil || !second.FromCache || string(second.Body) != `{"ok":true}` {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected fresh cache hit, requests=%d", requests)
	}
	c.CacheTTL = -1
	third, err := c.Get(context.Background(), "/latest/test/")
	if err != nil || !third.FromCache {
		t.Fatalf("304=%+v err=%v", third, err)
	}
	if atomic.LoadInt32(&requests) != 2 {
		t.Fatalf("expected conditional request, requests=%d", requests)
	}
}

func TestESIRetryAfterAndCancellation(t *testing.T) {
	var attempts int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewESIClient()
	c.BaseURL = srv.URL
	c.Client = srv.Client()
	c.MaxRetries = 2
	c.RetryBaseDelay = time.Millisecond
	res, err := c.Get(context.Background(), "/latest/test/")
	if err == nil || !strings.Contains(err.Error(), "retry limit") || res.Status != 429 || attempts != 3 {
		t.Fatalf("res=%+v err=%v attempts=%d", res, err, attempts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.MaxRetries = 5
	_, err = c.Get(ctx, "/latest/test/")
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestESIResponseLimit(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("0123456789")) }))
	defer srv.Close()
	c := NewESIClient()
	c.BaseURL = srv.URL
	c.Client = srv.Client()
	c.MaxResponseBytes = 4
	if _, err := c.Get(context.Background(), "/latest/test/"); err != ErrESIResponseTooLarge {
		t.Fatalf("expected response limit, got %v", err)
	}
}
