package esipublic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relay-server/internal/esi"
	"relay-server/internal/esidata"
)

func TestStatusCachesETagAndServesStaleOnError(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("ETag", `"s1"`)
		w.Header().Set("Cache-Control", "max-age=10")
		_, _ = w.Write([]byte(`{"players":42,"server_version":"x","start_time":"2024-01-01T00:00:00Z","vip":false}`))
	}))
	defer upstream.Close()
	gateway, err := esi.NewGateway(&esi.Client{BaseURL: upstream.URL, UserAgent: "test", HTTP: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	repo := esidata.NewMemoryRepository()
	svc, _ := New(gateway, repo)
	now := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	gateway.Client.Now = func() time.Time { return now }
	first, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Entry.ETag != `"s1"` || calls != 1 {
		t.Fatalf("first=%+v calls=%d", first, calls)
	}
	if _, err := svc.Status(context.Background()); err != nil || calls != 1 {
		t.Fatalf("cache hit err=%v calls=%d", err, calls)
	}
	now = now.Add(11 * time.Second)
	stale, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || calls != 2 || string(stale.Entry.Payload) == "" {
		t.Fatalf("stale=%+v calls=%d", stale, calls)
	}
}

func TestResolversUsePublicCacheAndPreserveMetadata(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("ETag", `"entity-v1"`)
		w.Header().Set("Cache-Control", "max-age=3600")
		switch r.URL.Path {
		case "/corporations/7/":
			w.Write([]byte(`{"name":"Corp Seven"}`))
		case "/alliances/8/":
			w.Write([]byte(`{"name":"Alliance Eight"}`))
		case "/universe/stations/9/":
			w.Write([]byte(`{"station_id":9,"name":"Station Nine"}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	gateway, _ := esi.NewGateway(&esi.Client{BaseURL: upstream.URL, UserAgent: "test", HTTP: upstream.Client()})
	repo := esidata.NewMemoryRepository()
	svc, _ := New(gateway, repo)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	gateway.Client.Now = func() time.Time { return now }
	corp, err := svc.Corporation(context.Background(), 7)
	if err != nil || corp.Entry.ETag != `"entity-v1"` || corp.Entry.Kind != KindCorporation {
		t.Fatalf("corp=%+v err=%v", corp, err)
	}
	if _, err = svc.Corporation(context.Background(), 7); err != nil || calls != 1 {
		t.Fatalf("cache calls=%d err=%v", calls, err)
	}
	if alliance, err := svc.Alliance(context.Background(), 8); err != nil || alliance.Entry.Kind != KindAlliance {
		t.Fatalf("alliance=%+v err=%v", alliance, err)
	}
	if station, err := svc.UniverseStation(context.Background(), 9); err != nil || station.Entry.Kind != KindUniverseStation {
		t.Fatalf("station=%+v err=%v", station, err)
	}
}

func TestOnDemandBoundsPreventRequests(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	gateway, _ := esi.NewGateway(&esi.Client{BaseURL: upstream.URL, UserAgent: "test", HTTP: upstream.Client()})
	svc, _ := New(gateway, esidata.NewMemoryRepository())
	if _, err := svc.UniverseType(context.Background(), MaxUniverseID+1); err == nil {
		t.Fatal("expected bounds error")
	}
	if _, err := svc.RegionOrders(context.Background(), 1, "bogus", 0); err == nil {
		t.Fatal("expected order type error")
	}
	if calls != 0 {
		t.Fatalf("made %d upstream calls", calls)
	}
}
