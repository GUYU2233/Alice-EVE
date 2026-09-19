package trade

import (
	"testing"
	"time"

	"relay-server/internal/esi"
)

func candidate(id int64, from, to Hub, score, confidence float64) Candidate {
	return Candidate{TypeID: id, From: from, To: to, MaxQuantity: 100, UnitVolume: 1, UnitCost: 10, UnitReturn: 15, NetPerUnit: 5, Score: score, Confidence: confidence, MinSecurity: 0.8}
}

func TestPackCandidatesHonorsReserveCargoAndConcentration(t *testing.T) {
	jita, _ := HubByCode("jita")
	amarr, _ := HubByCode("amarr")
	items, err := PackCandidates([]Candidate{candidate(2, jita, amarr, 80, .9), candidate(1, jita, amarr, 80, .8)}, Constraint{Budget: 1000, BudgetReserve: 200, CargoM3: 100, MaxItemConcentration: .5})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].TypeID != 1 {
		t.Fatalf("determinism/items: %+v", items)
	}
	var capital, volume float64
	for _, x := range items {
		capital += x.Capital
		volume += x.VolumeM3
		if x.Capital > 400 {
			t.Fatalf("concentration exceeded: %+v", x)
		}
	}
	if capital != 800 || volume != 80 {
		t.Fatalf("limits not filled: capital=%v volume=%v", capital, volume)
	}
}

func TestPackCandidatesRelaxesConcentrationToReachTargetLoad(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	x := candidate(1, j, a, 1, .9)
	x.MaxQuantity = 100
	x.UnitCost = 1
	x.NetPerUnit = 1
	x.UnitVolume = 1
	items, err := PackCandidates([]Candidate{x}, Constraint{Budget: 100, CargoM3: 100, MaxItemConcentration: .25, TargetLoadFactor: .9})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Quantity < 90 {
		t.Fatalf("adaptive concentration did not fill cargo: %+v", items)
	}
}

func TestPackCandidatesRejectsInvalidAndKeepsScoreConfidenceSeparate(t *testing.T) {
	jita, _ := HubByCode("jita")
	amarr, _ := HubByCode("amarr")
	a := candidate(1, jita, amarr, 90, .2)
	b := candidate(2, jita, amarr, 80, 1)
	items, err := PackCandidates([]Candidate{b, a}, Constraint{Budget: 100, CargoM3: 10, MaxItemConcentration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TypeID != 1 || items[0].Score != 90 || items[0].Confidence != .2 {
		t.Fatalf("score was confidence-adjusted: %+v", items)
	}
	if _, err = PackCandidates(nil, Constraint{Budget: 1, BudgetReserve: 2, CargoM3: 1}); err == nil {
		t.Fatal("expected invalid reserve")
	}
}

func TestSearchPlansBoundsSecurityAndBeamDeterminism(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	d, _ := HubByCode("dodixie")
	r, _ := HubByCode("rens")
	h, _ := HubByCode("hek")
	routes := []Route{{From: j, To: a, Jumps: 8, MinSecurity: .9}, {From: j, To: d, Jumps: 5, MinSecurity: .4}, {From: a, To: r, Jumps: 7, MinSecurity: .8}, {From: r, To: h, Jumps: 4, MinSecurity: .8}, {From: h, To: d, Jumps: 3, MinSecurity: .8}}
	cs := []Candidate{candidate(1, j, a, 90, .9), candidate(2, j, d, 100, 1), candidate(3, a, r, 80, .8), candidate(4, r, h, 70, .7), candidate(5, h, d, 60, .6)}
	plans, err := SearchPlans(j, routes, cs, Snapshot{AsOf: time.Unix(1, 0)}, Constraint{Budget: 100, CargoM3: 10, MinSecurity: .5, BeamWidth: 1, MaxStops: 99, MaxLegs: 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 {
		t.Fatalf("expected one plan per depth: %d", len(plans))
	}
	best := plans[0]
	if len(best.Legs) > 3 || len(best.Stops) > 4 {
		t.Fatalf("bounds violated: %+v", best)
	}
	for _, l := range best.Legs {
		if l.MinSecurity < .5 || l.To.Code == "dodixie" {
			t.Fatalf("unsafe route included: %+v", l)
		}
	}
	if best.PlanScore <= 0 || best.PlanConfidence != .7 {
		t.Fatalf("score/confidence: %+v", best)
	}
}

func TestPackCandidatesBoundedOptimizationBeatsGreedyScore(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	highScore := candidate(1, j, a, 100, 1)
	highScore.UnitCost, highScore.UnitVolume, highScore.NetPerUnit, highScore.MaxQuantity = 10, 10, 11, 1
	profitable := candidate(2, j, a, 10, 1)
	profitable.UnitCost, profitable.UnitVolume, profitable.NetPerUnit, profitable.MaxQuantity = 10, 5, 10, 2
	items, err := PackCandidates([]Candidate{highScore, profitable}, Constraint{Budget: 20, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TypeID != 2 || items[0].Quantity != 2 {
		t.Fatalf("expected profit-maximizing load, got %+v", items)
	}
}

func TestPackFixedRouteRejectsMixedLocationsAndExplainsUnusedCargo(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	r, _ := HubByCode("rens")
	x := candidate(1, j, a, 1, 1)
	x.MaxQuantity = 3
	x.UnitVolume = 1
	y := candidate(2, j, r, 1, 1)
	y.MaxQuantity = 100
	y.UnitVolume = 1
	p, err := PackFixedRoute([]Candidate{x, y}, Constraint{Budget: 1000, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || p.Items[0].TypeID != 1 {
		t.Fatalf("mixed destination admitted: %+v", p)
	}
	if p.LoadFactor != .3 || p.UnfilledReason == "" {
		t.Fatalf("missing utilization explanation: %+v", p)
	}
}

func TestPackByRouteRanksWholeLoadsAndNeverMixesStations(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	r, _ := HubByCode("rens")
	x := candidate(1, j, a, 1, 1)
	x.MaxQuantity = 2
	x.NetPerUnit = 10
	y := candidate(2, j, r, 1, 1)
	y.MaxQuantity = 2
	y.NetPerUnit = 20
	plans, err := PackByRoute([]Candidate{x, y}, Constraint{Budget: 100, CargoM3: 2}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].To.StationID != r.StationID || plans[0].NetProfit != 40 {
		t.Fatalf("bad route ranking: %+v", plans)
	}
	for _, p := range plans {
		if len(p.Items) != 1 {
			t.Fatalf("cross-route item mixing: %+v", p)
		}
	}
}

func TestPackCandidatesConsumesProfitableHigherAskLevels(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	x := candidate(1, j, a, 1, 1)
	x.MaxQuantity = 10
	x.UnitVolume = 1
	x.UnitCost = 100
	x.NetPerUnit = 40
	x.AskLevels = []PriceLevel{{Price: 100, Volume: 2}, {Price: 120, Volume: 8}}
	x.BidLevels = []PriceLevel{{Price: 150, Volume: 10}}
	x.SalesTaxRate = .1
	items, err := PackCandidates([]Candidate{x}, Constraint{Budget: 2000, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Quantity != 10 {
		t.Fatalf("expected full profitable depth, got %+v", items)
	}
	if items[0].UnitCost != 116 || items[0].NetProfit != 190 {
		t.Fatalf("weighted prices wrong: %+v", items[0])
	}
}

func TestPackCandidatesStopsBeforeUnprofitableAskLevel(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	x := candidate(1, j, a, 1, 1)
	x.MaxQuantity = 10
	x.UnitVolume = 1
	x.UnitCost = 100
	x.NetPerUnit = 40
	x.AskLevels = []PriceLevel{{Price: 100, Volume: 5}, {Price: 200, Volume: 5}}
	x.BidLevels = []PriceLevel{{Price: 150, Volume: 10}}
	items, err := PackCandidates([]Candidate{x}, Constraint{Budget: 2000, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Quantity != 5 {
		t.Fatalf("expected profitable first level only, got %+v", items)
	}
}

func TestSimulatePickupDeliveryCarriesInventoryAndReleasesCashOnlyAtDestination(t *testing.T) {
	a, _ := HubByCode("jita")
	b, _ := HubByCode("amarr")
	d, _ := HubByCode("rens")
	x := candidate(1, a, d, 1, 1)
	x.MaxQuantity = 1
	x.UnitCost = 100
	x.NetPerUnit = 50
	x.UnitVolume = 6
	z := candidate(2, b, d, 1, 1)
	z.MaxQuantity = 1
	z.UnitCost = 80
	z.NetPerUnit = 20
	z.UnitVolume = 4
	p, err := SimulatePickupDelivery(a, []Hub{a, b, d}, []Candidate{x, z}, Constraint{Budget: 180, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stops) != 3 || p.Stops[0].CashAfter != 80 || p.Stops[0].CargoAfterM3 != 6 {
		t.Fatalf("pickup state wrong: %+v", p)
	}
	if p.Stops[1].SaleRevenue != 0 || p.Stops[1].CashAfter != 0 || p.Stops[1].CargoBeforeM3 != 6 || p.Stops[1].CargoAfterM3 != 10 {
		t.Fatalf("in-transit state wrong: %+v", p.Stops[1])
	}
	if p.Stops[2].SaleRevenue != 250 || p.Stops[2].CargoAfterM3 != 0 || p.Stops[2].CashAfter != 250 || p.RealizedProfit != 70 {
		t.Fatalf("delivery state wrong: %+v", p)
	}
	if len(p.Items) != 2 || len(p.Stops[0].Loads) != 1 || len(p.Stops[1].Loads) != 1 || len(p.Stops[2].Unloads) != 2 || p.Capital != 180 || p.VolumeM3 != 10 {
		t.Fatalf("chain manifest or aggregates lost: %+v", p)
	}
}

func TestSearchPickupDeliveryPlansRequiresActualDelivery(t *testing.T) {
	a, _ := HubByCode("jita")
	b, _ := HubByCode("amarr")
	d, _ := HubByCode("rens")
	x := candidate(1, a, d, 1, 1)
	x.MaxQuantity = 1
	x.UnitCost = 100
	x.NetPerUnit = 50
	plans, err := SearchPickupDeliveryPlans(a, []Route{{From: a, To: b, Jumps: 1, MinSecurity: .8}, {From: b, To: d, Jumps: 1, MinSecurity: .8}}, []Candidate{x}, Constraint{Budget: 100, CargoM3: 1, BeamWidth: 4, MaxStops: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].RealizedProfit != 50 || len(plans[0].Stops) != 3 {
		t.Fatalf("undelivered cargo was scored or route missing: %+v", plans)
	}
	if plans[0].Stops[1].CargoAfterM3 != 1 || plans[0].Stops[1].SaleRevenue != 0 {
		t.Fatalf("cargo released before delivery: %+v", plans[0].Stops[1])
	}
	if plans[0].TotalJumps != 2 || plans[0].MinSecurity != .8 || len(plans[0].Items) != 1 {
		t.Fatalf("route metadata or manifest missing: %+v", plans[0])
	}
}

func TestSearchPlansReinvestsRealizedProfit(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	r, _ := HubByCode("rens")
	first := candidate(1, j, a, 10, 1)
	first.MaxQuantity = 1
	first.UnitCost = 100
	first.NetPerUnit = 100
	second := candidate(2, a, r, 10, 1)
	second.MaxQuantity = 1
	second.UnitCost = 150
	second.NetPerUnit = 50
	plans, err := SearchPlans(j, []Route{{From: j, To: a, Jumps: 1, MinSecurity: 1}, {From: a, To: r, Jumps: 1, MinSecurity: 1}}, []Candidate{first, second}, Snapshot{}, Constraint{Budget: 100, CargoM3: 10, BeamWidth: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) == 0 || len(plans[0].Legs) != 2 || plans[0].NetProfit != 150 {
		t.Fatalf("profit was not reinvested: %+v", plans)
	}
}

func TestSearchPlansNoRevisit(t *testing.T) {
	j, _ := HubByCode("jita")
	a, _ := HubByCode("amarr")
	plans, err := SearchPlans(j, []Route{{From: j, To: a, Jumps: 1, MinSecurity: 1}, {From: a, To: j, Jumps: 1, MinSecurity: 1}}, []Candidate{candidate(1, j, a, 1, 1), candidate(2, a, j, 1, 1)}, Snapshot{}, Constraint{Budget: 100, CargoM3: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || len(plans[0].Legs) != 1 {
		t.Fatalf("cycle admitted: %+v", plans)
	}
}

func TestValidateOrderScope(t *testing.T) {
	scope := OrderScope{RegionID: 1, StationID: 10, TypeID: 34, Buy: true}
	valid := []esi.MarketOrder{{LocationID: 10, TypeID: 34, IsBuyOrder: true, Price: 5, VolumeRemain: 1}}
	if err := ValidateOrderScope(scope, valid); err != nil {
		t.Fatal(err)
	}
	cases := []esi.MarketOrder{
		{LocationID: 11, TypeID: 34, IsBuyOrder: true, Price: 5, VolumeRemain: 1},
		{LocationID: 10, TypeID: 35, IsBuyOrder: true, Price: 5, VolumeRemain: 1},
		{LocationID: 10, TypeID: 34, IsBuyOrder: false, Price: 5, VolumeRemain: 1},
	}
	for _, o := range cases {
		if ValidateOrderScope(scope, []esi.MarketOrder{o}) == nil {
			t.Fatalf("accepted out-of-scope order: %+v", o)
		}
	}
}
