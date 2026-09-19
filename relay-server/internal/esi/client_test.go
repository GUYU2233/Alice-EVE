package esi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

type memoryCache struct{ entries map[string]CacheEntry }

func (m *memoryCache) Get(_ context.Context, key string) (CacheEntry, bool, error) {
	e, ok := m.entries[key]
	return e, ok, nil
}
func (m *memoryCache) Put(_ context.Context, key string, entry CacheEntry) error {
	if m.entries == nil {
		m.entries = map[string]CacheEntry{}
	}
	m.entries[key] = entry
	return nil
}

func testGateway(t *testing.T, h http.Handler) (*Gateway, *httptest.Server) {
	t.Helper()
	s := httptest.NewServer(h)
	g, err := NewGateway(&Client{BaseURL: s.URL, UserAgent: "Alice-EVE/test (+ops@example.test)", HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return g, s
}

func TestAuthenticatedHeaderAndRepresentativePaths(t *testing.T) {
	paths := []string{}
	g, s := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		switch r.URL.Path {
		case "/characters/42/wallet/":
			w.Write([]byte(`12.5`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer s.Close()
	ctx := context.Background()
	if _, _, err := g.CharacterOnline(ctx, 42, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.CharacterLocation(ctx, 42, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.CharacterShip(ctx, 42, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.CharacterSkills(ctx, 42, "secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.CharacterWallet(ctx, 42, "secret"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/characters/42/online/", "/characters/42/location/", "/characters/42/ship/", "/characters/42/skills/", "/characters/42/wallet/"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths=%v", paths)
	}
	if _, _, err := g.CharacterOnline(ctx, 0, "secret"); err == nil {
		t.Fatal("expected invalid character error")
	}
	if _, _, err := g.CharacterOnline(ctx, 42, " "); err == nil {
		t.Fatal("expected invalid token error")
	}
}

func TestETag304AndCacheExpiry(t *testing.T) {
	requests := 0
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := &memoryCache{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			if got := r.Header.Get("If-None-Match"); got != "v1" {
				t.Errorf("If-None-Match=%q", got)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "v1")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Write([]byte(`{"players":7}`))
	}))
	defer s.Close()
	g, _ := NewGateway(&Client{BaseURL: s.URL, UserAgent: "test", HTTP: s.Client(), Cache: cache, Now: func() time.Time { return now }})
	got, meta, err := g.Status(context.Background())
	if err != nil || got.Players != 7 || meta.ExpiresAt != now.Add(time.Minute) {
		t.Fatalf("got=%+v meta=%+v err=%v", got, meta, err)
	}
	got, meta, err = g.Status(context.Background())
	if err != nil || got.Players != 7 || !meta.FromCache {
		t.Fatalf("got=%+v meta=%+v err=%v", got, meta, err)
	}
}

func TestPagination(t *testing.T) {
	var pages []string
	g, s := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		w.Header().Set("X-Pages", "3")
		w.Write([]byte(`[{"item_id":` + r.URL.Query().Get("page") + `}]`))
	}))
	defer s.Close()
	got, meta, err := g.CharacterAssets(context.Background(), 9, "token")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].ItemID != 3 || meta.Pages != 3 || !reflect.DeepEqual(pages, []string{"1", "2", "3"}) {
		t.Fatalf("got=%+v meta=%+v pages=%v", got, meta, pages)
	}
}

func TestRetryMetadataFor420And429(t *testing.T) {
	for _, status := range []int{420, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			g, s := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "12")
				w.Header().Set("X-Esi-Error-Limit-Remain", "3")
				w.Header().Set("X-Esi-Error-Limit-Reset", "40")
				w.WriteHeader(status)
			}))
			defer s.Close()
			g.Client.Now = func() time.Time { return now }
			_, _, err := g.Status(context.Background())
			var ee *Error
			if !errors.As(err, &ee) {
				t.Fatalf("error=%v", err)
			}
			if ee.Status != status || ee.RetryAfter != 12*time.Second || ee.RetryAt != now.Add(12*time.Second) || ee.ErrorLimitRemain != 3 || ee.ErrorLimitReset != 40*time.Second {
				t.Fatalf("error=%+v", ee)
			}
		})
	}
}

func TestResponseSizeLimit(t *testing.T) {
	g, s := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"players":12345}`)) }))
	defer s.Close()
	g.Client.MaxBodyBytes = 5
	_, _, err := g.Status(context.Background())
	var ee *Error
	if !errors.As(err, &ee) || ee.Kind != ErrTooLarge {
		t.Fatalf("error=%v", err)
	}
}

func TestPublicEndpointPathsAndQueries(t *testing.T) {
	var seen []*url.URL
	g, s := testGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		seen = append(seen, &u)
		switch r.URL.Path {
		case "/status/":
			w.Write([]byte(`{}`))
		case "/universe/systems/30000142/":
			w.Write([]byte(`{"system_id":30000142}`))
		case "/universe/types/34/":
			w.Write([]byte(`{"type_id":34}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer s.Close()
	ctx := context.Background()
	g.Status(ctx)
	g.UniverseSystem(ctx, 30000142)
	g.UniverseType(ctx, 34)
	g.MarketPrices(ctx)
	g.RegionOrders(ctx, 10000002, "sell", 34)
	g.RegionHistory(ctx, 10000002, 34)
	want := []string{"/status/", "/universe/systems/30000142/", "/universe/types/34/", "/markets/prices/", "/markets/10000002/orders/", "/markets/10000002/history/"}
	for i, p := range want {
		if seen[i].Path != p {
			t.Errorf("path[%d]=%s", i, seen[i].Path)
		}
	}
	if seen[4].Query().Get("order_type") != "sell" || seen[4].Query().Get("type_id") != "34" {
		t.Errorf("orders query=%v", seen[4].Query())
	}
}
