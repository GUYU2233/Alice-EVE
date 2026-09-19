package marketplan

import (
	"context"
	"relay-server/internal/marketdata"
	"relay-server/internal/routeplanner"
	"testing"
)

type candidateSourceStub struct{ items []marketdata.TradeCandidate }

func (s candidateSourceStub) SearchCandidates(context.Context, marketdata.CandidateSearch) (marketdata.CandidatePage, error) {
	return marketdata.CandidatePage{Items: s.items}, nil
}

type routeSourceStub struct{}

func (routeSourceStub) Routes(_ context.Context, q []routeplanner.Query) ([]routeplanner.Result, error) {
	out := make([]routeplanner.Result, len(q))
	for i, x := range q {
		out[i] = routeplanner.Result{Status: "ready", From: x.From, To: x.To, Jumps: 2, MinSecurity: .8, Systems: []routeplanner.System{{ID: x.From, Name: "A", Security: 1}, {ID: x.To, Name: "B", Security: .8}}}
	}
	return out, nil
}
func TestPlanningEngineProducesPersistentBasketBatch(t *testing.T) {
	x := marketdata.TradeCandidate{TypeID: 34, SourceRegionID: 1, SourceLocationID: 10, SourceSystemID: 100, DestinationRegionID: 2, DestinationLocationID: 20, DestinationSystemID: 200, BuyPrice: 1, SellPrice: 2, ItemVolumeM3: 1, Quantity: 100, NetProfit: 96}
	e := PlanningEngine{Candidates: candidateSourceStub{[]marketdata.TradeCandidate{x}}, Routes: routeSourceStub{}}
	c, err := e.Step(context.Background(), Job{Mode: "basket", SourceRegionIDs: []int64{1}, DestinationScope: "all_collected_regions", Constraints: Constraints{Budget: 100, BudgetReserve: 1, CargoM3: 100, MinSecurity: .5, MaxJumps: 10, TargetLoadFactor: .9}})
	if err != nil {
		t.Fatal(err)
	}
	if c.State != "watching" || len(c.Results) != 1 {
		t.Fatalf("commit=%+v", c)
	}
}
func TestPlanningEngineSingleCarriesFullRoute(t *testing.T) {
	x := marketdata.TradeCandidate{TypeID: 34, SourceRegionID: 1, SourceLocationID: 10, SourceSystemID: 100, DestinationRegionID: 2, DestinationLocationID: 20, DestinationSystemID: 200, BuyPrice: 1, SellPrice: 2, ItemVolumeM3: 1, Quantity: 10, NetProfit: 9}
	e := PlanningEngine{Candidates: candidateSourceStub{[]marketdata.TradeCandidate{x}}, Routes: routeSourceStub{}}
	c, err := e.Step(context.Background(), Job{Mode: "single", SourceRegionIDs: []int64{1}, DestinationScope: "all_collected_regions", Constraints: Constraints{Budget: 100, CargoM3: 100, MinSecurity: .5, MaxJumps: 10, TargetLoadFactor: .9}})
	if err != nil || len(c.Results) != 1 {
		t.Fatalf("%+v %v", c, err)
	}
	if string(c.Results[0].Payload) == "" {
		t.Fatal("missing payload")
	}
}
