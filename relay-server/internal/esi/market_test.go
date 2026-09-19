package esi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicResolverEndpointPaths(t *testing.T) {
	want := map[string]string{
		"/corporations/98608844/":      `{"name":"Acme Corp"}`,
		"/alliances/9901/":             `{"name":"Acme Alliance"}`,
		"/universe/stations/60003760/": `{"station_id":60003760,"name":"Jita IV - Moon 4"}`,
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := want[r.URL.Path]
		if !ok {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("public resolver sent bearer")
		}
		w.Header().Set("ETag", `"resolver-v1"`)
		w.Write([]byte(body))
	}))
	defer s.Close()
	g, err := NewGateway(&Client{BaseURL: s.URL, UserAgent: "test", HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	corp, meta, err := g.Corporation(ctx, 98608844)
	if err != nil || corp.Name != "Acme Corp" || meta.ETag != `"resolver-v1"` {
		t.Fatalf("corp=%+v meta=%+v err=%v", corp, meta, err)
	}
	alliance, _, err := g.Alliance(ctx, 9901)
	if err != nil || alliance.Name != "Acme Alliance" {
		t.Fatalf("alliance=%+v err=%v", alliance, err)
	}
	station, _, err := g.UniverseStation(ctx, 60003760)
	if err != nil || station.Name != "Jita IV - Moon 4" {
		t.Fatalf("station=%+v err=%v", station, err)
	}
}

func TestAdditionalAuthenticatedEndpointPaths(t *testing.T) {
	want := map[string]bool{
		"/characters/7/skillqueue/": false, "/characters/7/wallet/journal/": false,
		"/characters/7/assets/": false, "/characters/7/orders/": false,
		"/characters/7/orders/history/": false, "/characters/7/killmails/recent/": false,
		"/characters/7/notifications/": false,
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := want[r.URL.Path]; !ok {
			t.Errorf("unexpected path %s", r.URL.Path)
		} else {
			want[r.URL.Path] = true
		}
		w.Write([]byte(`[]`))
	}))
	defer s.Close()
	g, err := NewGateway(&Client{BaseURL: s.URL, UserAgent: "test", HTTP: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	g.CharacterSkillQueue(ctx, 7, "token")
	g.CharacterWalletJournal(ctx, 7, "token")
	g.CharacterAssets(ctx, 7, "token")
	g.CharacterOrders(ctx, 7, "token", false)
	g.CharacterOrders(ctx, 7, "token", true)
	g.CharacterKillmails(ctx, 7, "token", 2)
	g.CharacterNotifications(ctx, 7, "token")
	for path, seen := range want {
		if !seen {
			t.Errorf("did not request %s", path)
		}
	}
}
