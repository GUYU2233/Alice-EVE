package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTypedPlannerResponsesPreserveNestedDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trade/plans/compose" {
			json.NewEncoder(w).Encode([]any{map[string]any{"from": map[string]any{"StationID": 1}, "to": map[string]any{"StationID": 2}, "items": []any{map[string]any{"typeId": 34, "quantity": 5, "capital": 100, "volumeM3": 5}}, "capital": 100, "volumeM3": 5, "netProfit": 50, "jumps": 2, "minSecurity": .8}})
			return
		}
		json.NewEncoder(w).Encode([]any{map[string]any{"stops": []any{map[string]any{"hub": map[string]any{"StationID": 1}, "loads": []any{map[string]any{"item": map[string]any{"typeId": 34, "quantity": 5}}}}}, "items": []any{map[string]any{"typeId": 34, "quantity": 5}}, "realizedProfit": 50, "capital": 100, "volumeM3": 5, "totalJumps": 2, "minSecurity": .8}})
	}))
	defer srv.Close()
	r := NewRelayClient()
	if e := r.SetURL(srv.URL); e != nil {
		t.Fatal(e)
	}
	r.mu.Lock()
	r.accessToken = "test"
	r.mu.Unlock()
	packs, e := r.ComposeTradePlan(context.Background(), TradePackRequest{Candidates: []TradeCandidate{{TypeID: 34}}, Constraint: TradeConstraint{}})
	if e != nil || len(packs) != 1 || packs[0].Items[0].Quantity != 5 || packs[0].From.StationID != 1 {
		t.Fatalf("pack=%+v err=%v", packs, e)
	}
	chains, e := r.ChainTradePlans(context.Background(), TradeChainRequest{Candidates: []TradeCandidate{{TypeID: 34}}})
	if e != nil || len(chains) != 1 || chains[0].Stops[0].Loads[0].Item.TypeID != 34 || chains[0].TotalJumps != 2 {
		t.Fatalf("chain=%+v err=%v", chains, e)
	}
}
