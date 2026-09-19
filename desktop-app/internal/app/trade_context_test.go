package app

import (
	"errors"
	"testing"
	"time"
)

type cargoStub struct {
	m      float64
	source string
	exact  bool
	err    error
}

func (c cargoStub) ShipCargoCapacity(int64) (float64, string, bool, error) {
	return c.m, c.source, c.exact, c.err
}
func TestBuildCharacterTradeContext(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	snaps := []CharacterSnapshot{
		{CharacterID: 42, Domain: "location", Payload: []byte(`{"solar_system_id":30000142,"station_id":60003760}`), FetchedAt: now},
		{CharacterID: 42, Domain: "ship", Payload: []byte(`{"ship_item_id":99,"ship_type_id":20185,"ship_name":"Hauler"}`), FetchedAt: now.Add(time.Second)},
		{CharacterID: 42, Domain: "wallet", Payload: []byte(`1000`), FetchedAt: now, Stale: true},
	}
	max := 700.0
	got, err := BuildCharacterTradeContext(snaps, TradeBudget{MaximumISK: &max, ReserveISK: 100}, cargoStub{m: 3500, source: "esi universe type base capacity", exact: false})
	if err != nil {
		t.Fatal(err)
	}
	if got.SolarSystemID != 30000142 || got.ShipTypeID != 20185 || got.Cargo.M3 != 3500 || !got.Cargo.Estimated || got.AvailableBudgetISK != 700 || !got.Stale || !got.ObservedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("got=%+v", got)
	}
}
func TestBuildCharacterTradeContextRejectsMissingAndCapacityFailure(t *testing.T) {
	snaps := []CharacterSnapshot{{CharacterID: 1, Domain: "location", Payload: []byte(`{"solar_system_id":2}`)}, {CharacterID: 1, Domain: "ship", Payload: []byte(`{"ship_type_id":3}`)}, {CharacterID: 1, Domain: "wallet", Payload: []byte(`10`)}}
	if _, err := BuildCharacterTradeContext(snaps, TradeBudget{}, cargoStub{err: errors.New("no type")}); err == nil {
		t.Fatal("expected capacity error")
	}
	if _, err := BuildCharacterTradeContext(snaps[:2], TradeBudget{}, cargoStub{}); err == nil {
		t.Fatal("expected missing wallet")
	}
}
func TestTradeBudgetAvailable(t *testing.T) {
	max := 800.0
	got, err := (TradeBudget{MaximumISK: &max, ReserveISK: 300}).Available(1000)
	if err != nil || got != 700 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err = (TradeBudget{ReserveISK: -1}).Available(1); err == nil {
		t.Fatal("expected error")
	}
}
