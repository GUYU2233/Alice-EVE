package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCharacterDataUsesRelayAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account-token" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/eve/characters/9001":
			_, _ = w.Write([]byte(`[{"AccountID":"acct","CharacterID":9001,"Domain":"skills","Payload":{"total_sp":42}}]`))
		case "/api/v1/eve/characters/9001/skills":
			_, _ = w.Write([]byte(`{"AccountID":"acct","CharacterID":9001,"Domain":"skills","Payload":{"total_sp":42}}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewRelayClient()
	_ = client.SetURL(server.URL)
	client.SetToken("account-token")
	list, err := client.FetchCharacterSnapshots(context.Background(), 9001)
	if err != nil || len(list) != 1 || list[0].Domain != "skills" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	var payload struct {
		TotalSP int64 `json:"total_sp"`
	}
	if err := json.Unmarshal(list[0].Payload, &payload); err != nil || payload.TotalSP != 42 {
		t.Fatalf("payload=%+v err=%v", payload, err)
	}
	snapshot, err := client.FetchCharacterSnapshot(context.Background(), 9001, "skills")
	if err != nil || snapshot.CharacterID != 9001 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
}

func TestPublicEVEEndpointsDoNotSendRelayBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public endpoint leaked bearer %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/eve/public/universe/names":
			if r.Method != http.MethodPost {
				t.Fatalf("method=%s", r.Method)
			}
			var ids []int64
			_ = json.NewDecoder(r.Body).Decode(&ids)
			if len(ids) != 2 {
				t.Fatalf("ids=%v", ids)
			}
			_, _ = w.Write([]byte(`[{"id":34,"name":"Tritanium","category":"inventory_type"}]`))
		case "/api/v1/eve/public/status":
			_, _ = w.Write([]byte(`{"players":123,"server_version":"v1"}`))
		case "/api/v1/eve/public/market/prices":
			_, _ = w.Write([]byte(`[{"type_id":34,"average_price":5.5}]`))
		case "/api/v1/eve/public/corporations/98608844":
			_, _ = w.Write([]byte(`{"name":"Acme Corp","ticker":"ACME","alliance_id":9901}`))
		case "/api/v1/eve/public/alliances/9901":
			_, _ = w.Write([]byte(`{"name":"Acme Alliance","ticker":"AA"}`))
		case "/api/v1/eve/public/universe/stations/60003760":
			_, _ = w.Write([]byte(`{"station_id":60003760,"name":"Jita IV - Moon 4","system_id":30000142}`))
		case "/api/v1/eve/public/universe/types/34":
			_, _ = w.Write([]byte(`{"type_id":34,"name":"Tritanium"}`))
		case "/api/v1/eve/public/universe/systems/30000142":
			_, _ = w.Write([]byte(`{"system_id":30000142,"name":"Jita"}`))
		case "/api/v1/eve/public/markets/10000002/orders":
			if r.URL.Query().Get("order_type") != "sell" || r.URL.Query().Get("type_id") != "34" {
				t.Fatalf("orders query=%v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`[{"order_id":7,"type_id":34,"price":5.1}]`))
		case "/api/v1/eve/public/markets/10000002/history":
			if r.URL.Query().Get("type_id") != "34" {
				t.Fatalf("history query=%v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`[{"date":"2026-01-01","average":5.2}]`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := NewRelayClient()
	_ = client.SetURL(server.URL)
	client.SetToken("secret-relay-token")
	ctx := context.Background()
	names, err := client.FetchEVEUniverseNames(ctx, []int64{34, 34, 30000142})
	if err != nil || len(names) != 1 || names[0].Name != "Tritanium" {
		t.Fatalf("names=%+v err=%v", names, err)
	}
	status, err := client.FetchEVEStatus(ctx)
	if err != nil || status.Players != 123 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	prices, err := client.FetchEVEMarketPrices(ctx)
	if err != nil || len(prices) != 1 || prices[0].TypeID != 34 {
		t.Fatalf("prices=%+v err=%v", prices, err)
	}
	corp, err := client.FetchEVECorporation(ctx, 98608844)
	if err != nil || corp.Name != "Acme Corp" || corp.AllianceID != 9901 {
		t.Fatalf("corporation=%+v err=%v", corp, err)
	}
	alliance, err := client.FetchEVEAlliance(ctx, 9901)
	if err != nil || alliance.Name != "Acme Alliance" {
		t.Fatalf("alliance=%+v err=%v", alliance, err)
	}
	station, err := client.FetchEVEUniverseStation(ctx, 60003760)
	if err != nil || station.Name != "Jita IV - Moon 4" {
		t.Fatalf("station=%+v err=%v", station, err)
	}
	typ, err := client.FetchEVEUniverseType(ctx, 34)
	if err != nil || typ.Name != "Tritanium" {
		t.Fatalf("type=%+v err=%v", typ, err)
	}
	system, err := client.FetchEVEUniverseSystem(ctx, 30000142)
	if err != nil || system.Name != "Jita" {
		t.Fatalf("system=%+v err=%v", system, err)
	}
	orders, err := client.FetchEVEMarketOrders(ctx, 10000002, "sell", 34)
	if err != nil || len(orders) != 1 || orders[0].OrderID != 7 {
		t.Fatalf("orders=%+v err=%v", orders, err)
	}
	history, err := client.FetchEVEMarketHistory(ctx, 10000002, 34)
	if err != nil || len(history) != 1 || history[0].Date != "2026-01-01" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestEVEClientValidatesIdentifiersAndOrderType(t *testing.T) {
	client := NewRelayClient()
	if _, err := client.FetchCharacterSnapshot(context.Background(), 1, "wallet/journal"); err == nil {
		t.Fatal("accepted path-like domain")
	}
	if _, err := client.FetchEVECorporation(context.Background(), 0); err == nil {
		t.Fatal("accepted zero corporation id")
	}
	if _, err := client.FetchEVEAlliance(context.Background(), 0); err == nil {
		t.Fatal("accepted zero alliance id")
	}
	if _, err := client.FetchEVEUniverseStation(context.Background(), 0); err == nil {
		t.Fatal("accepted zero station id")
	}
	if _, err := client.FetchEVEUniverseType(context.Background(), 0); err == nil {
		t.Fatal("accepted zero type id")
	}
	if _, err := client.FetchEVEMarketOrders(context.Background(), 1, "invalid", 0); err == nil {
		t.Fatal("accepted invalid order type")
	}
	if _, err := client.FetchEVEMarketHistory(context.Background(), 1, 0); err == nil {
		t.Fatal("accepted zero history type id")
	}
}
