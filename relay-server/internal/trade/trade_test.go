package trade

import (
	"relay-server/internal/esi"
	"testing"
	"time"
)

func TestEvaluateUsesStationDepthVWAPAndFees(t *testing.T) {
	src, _ := HubByCode("jita")
	dst, _ := HubByCode("dodixie")
	now := time.Now()
	so := []esi.MarketOrder{{LocationID: src.StationID, Price: 100, VolumeRemain: 60}, {LocationID: src.StationID, Price: 110, VolumeRemain: 40}, {LocationID: 999, Price: 1, VolumeRemain: 1000}}
	do := []esi.MarketOrder{{LocationID: dst.StationID, IsBuyOrder: true, Price: 150, VolumeRemain: 50}, {LocationID: dst.StationID, IsBuyOrder: true, Price: 140, VolumeRemain: 50}}
	o := Evaluate(34, src, dst, so, do, Params{Quantity: 100, SalesTaxRate: .02, BrokerRate: .01, ItemVolumeM3: .01, TransportCostPerM3: 10, CargoM3: 10, AverageDailyVolume: 1000, HistoryDays: 30, Now: now, FetchedAt: now, RouteJumps: 15})
	if o.AskVWAP != 104 || o.BidVWAP != 145 {
		t.Fatalf("vwap %+v", o)
	}
	if o.Quantity != 100 || o.Capital != 10400 {
		t.Fatalf("qty/capital %+v", o)
	}
	if o.NetProfit <= 0 || o.Score <= 0 {
		t.Fatalf("profit/score %+v", o)
	}
}
func TestEvaluateRepricesAtCommonExecutableQuantity(t *testing.T) {
	s, _ := HubByCode("jita")
	d, _ := HubByCode("dodixie")
	src := []esi.MarketOrder{{LocationID: s.StationID, Price: 100, VolumeRemain: 1}, {LocationID: s.StationID, Price: 1000, VolumeRemain: 99}}
	dst := []esi.MarketOrder{{LocationID: d.StationID, IsBuyOrder: true, Price: 200, VolumeRemain: 1}}
	o := Evaluate(34, s, d, src, dst, Params{Quantity: 100, AverageDailyVolume: 100, HistoryDays: 30})
	if o.Quantity != 1 || o.AskVWAP != 100 || o.BidVWAP != 200 || o.GrossProfit != 100 {
		t.Fatalf("final quantity not repriced: %+v", o)
	}
}
func TestEvaluateHonorsBuyMinVolume(t *testing.T) {
	s, _ := HubByCode("jita")
	d, _ := HubByCode("dodixie")
	src := []esi.MarketOrder{{LocationID: s.StationID, Price: 100, VolumeRemain: 5}}
	dst := []esi.MarketOrder{{LocationID: d.StationID, IsBuyOrder: true, Price: 200, VolumeRemain: 100, MinVolume: 10}}
	o := Evaluate(34, s, d, src, dst, Params{Quantity: 5, AverageDailyVolume: 100, HistoryDays: 30})
	if o.Quantity != 0 {
		t.Fatalf("min_volume order must not execute: %+v", o)
	}
}
func TestEvaluateRejectsThinDepthAndCargo(t *testing.T) {
	s, _ := HubByCode("jita")
	d, _ := HubByCode("amarr")
	o := Evaluate(1, s, d, []esi.MarketOrder{{LocationID: s.StationID, Price: 10, VolumeRemain: 10}}, []esi.MarketOrder{{LocationID: d.StationID, IsBuyOrder: true, Price: 100, VolumeRemain: 10}}, Params{Quantity: 100, ItemVolumeM3: 10, CargoM3: 20, AverageDailyVolume: 1, HistoryDays: 2})
	if o.Eligible {
		t.Fatal("thin opportunity must be rejected")
	}
	want := map[string]bool{"insufficient_depth": false, "history_insufficient": false, "cargo_exceeded": false}
	for _, r := range o.Reasons {
		if _, ok := want[r]; ok {
			want[r] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Fatalf("missing %s in %v", k, o.Reasons)
		}
	}
}
func TestHubRegistryUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, h := range Hubs {
		if seen[h.Code] || h.RegionID <= 0 || h.StationID <= 0 {
			t.Fatalf("bad hub %+v", h)
		}
		seen[h.Code] = true
	}
	if len(Hubs) != 5 {
		t.Fatalf("hubs=%d", len(Hubs))
	}
}
