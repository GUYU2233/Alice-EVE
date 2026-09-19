package marketplan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"relay-server/internal/marketdata"
	"relay-server/internal/routeplanner"
	"relay-server/internal/trade"
)

type CandidateSource interface {
	SearchCandidates(context.Context, marketdata.CandidateSearch) (marketdata.CandidatePage, error)
}
type RouteSource interface {
	Routes(context.Context, []routeplanner.Query) ([]routeplanner.Result, error)
}
type PlanningEngine struct {
	Candidates CandidateSource
	Routes     RouteSource
}

func (e PlanningEngine) Step(ctx context.Context, j Job) (Commit, error) {
	if e.Candidates == nil || e.Routes == nil {
		return Commit{}, fmt.Errorf("planning dependencies unavailable")
	}
	limit, perType := 500, 8
	if j.Mode == "chain" {
		limit, perType = 120, 3
	}
	page, err := e.Candidates.SearchCandidates(ctx, marketdata.CandidateSearch{SourceRegionIDs: j.SourceRegionIDs, DestinationRegionIDs: j.DestinationRegionIDs, DestinationScope: j.DestinationScope, PerTypeLocations: perType, Budget: j.Constraints.Budget - j.Constraints.BudgetReserve, CargoM3: j.Constraints.CargoM3, SalesTaxRate: .036, MinSecurity: j.Constraints.MinSecurity, MaxJumps: j.Constraints.MaxJumps, IncludeDepth: j.Mode != "single", Limit: limit})
	if err != nil {
		return Commit{}, err
	}
	queries := make([]routeplanner.Query, len(page.Items))
	for i, x := range page.Items {
		queries[i] = routeplanner.Query{From: x.SourceSystemID, To: x.DestinationSystemID, MinSecurity: j.Constraints.MinSecurity, MaxJumps: j.Constraints.MaxJumps}
	}
	if len(queries) == 0 {
		progress, _ := json.Marshal(map[string]any{"phase": "watching", "candidates": 0, "results": 0})
		return Commit{State: "watching", Progress: progress}, nil
	}
	routes, err := e.Routes.Routes(ctx, queries)
	if err != nil {
		return Commit{}, err
	}
	cs := []trade.Candidate{}
	candidateRoutes := []routeplanner.Result{}
	single := []PendingResult{}
	for i, x := range page.Items {
		r := routes[i]
		if r.Status != "ready" {
			continue
		}
		score := 30*math.Tanh(x.NetProfit/1e7) + 25*math.Tanh(x.ProfitRate*5) + 20*math.Tanh((x.NetProfit/math.Max(.01, x.CargoUsedM3))/5000) + 15*math.Exp(-float64(r.Jumps)/18) + 5*math.Max(0, math.Min(1, (r.MinSecurity+1)/2))
		score = math.Max(0, math.Min(100, score))
		c := trade.Candidate{TypeID: x.TypeID, From: trade.Hub{Code: fmt.Sprint(x.SourceLocationID), Name: fmt.Sprint(x.SourceLocationID), NameZH: fmt.Sprint(x.SourceLocationID), RegionID: x.SourceRegionID, StationID: x.SourceLocationID, SystemID: x.SourceSystemID}, To: trade.Hub{Code: fmt.Sprint(x.DestinationLocationID), Name: fmt.Sprint(x.DestinationLocationID), NameZH: fmt.Sprint(x.DestinationLocationID), RegionID: x.DestinationRegionID, StationID: x.DestinationLocationID, SystemID: x.DestinationSystemID}, MaxQuantity: x.Quantity, UnitVolume: x.ItemVolumeM3, UnitCost: x.BuyPrice, UnitReturn: x.SellPrice, NetPerUnit: x.NetProfit / math.Max(1, float64(x.Quantity)), SalesTaxRate: .036, Score: score, Confidence: .7, Jumps: r.Jumps, MinSecurity: r.MinSecurity}
		for _, l := range x.AskLevels {
			c.AskLevels = append(c.AskLevels, trade.PriceLevel{Price: l.Price, Volume: l.Volume, MinimumVolume: l.MinimumVolume})
		}
		for _, l := range x.BidLevels {
			c.BidLevels = append(c.BidLevels, trade.PriceLevel{Price: l.Price, Volume: l.Volume, MinimumVolume: l.MinimumVolume})
		}
		cs = append(cs, c)
		candidateRoutes = append(candidateRoutes, r)
		if j.Mode == "single" {
			x.RouteSafetyStatus = "ready"
			payload, _ := json.Marshal(struct {
				marketdata.TradeCandidate
				Score       float64               `json:"score"`
				Jumps       int                   `json:"jumps"`
				MinSecurity float64               `json:"minSecurity"`
				Systems     []routeplanner.System `json:"systems"`
			}{x, score, r.Jumps, r.MinSecurity, r.Systems})
			single = append(single, PendingResult{StableKey: fmt.Sprintf("%d/%d/%d", x.TypeID, x.SourceLocationID, x.DestinationLocationID), Score: score, Payload: payload})
		}
	}
	constraint := trade.Constraint{Budget: j.Constraints.Budget, BudgetReserve: j.Constraints.BudgetReserve, CargoM3: j.Constraints.CargoM3, MaxItemConcentration: .25, TargetLoadFactor: j.Constraints.TargetLoadFactor, MinSecurity: j.Constraints.MinSecurity, MaxStops: 4, MaxLegs: 3, BeamWidth: 48, MaxItemsPerLeg: 32, JumpPenalty: 0, StopPenalty: 0, OptimizationStates: 1024}
	if j.Mode == "chain" {
		constraint.BeamWidth, constraint.MaxItemsPerLeg, constraint.OptimizationStates = 12, 8, 192
	}
	out := single
	if j.Mode == "basket" {
		plans, e2 := trade.PackByRoute(cs, constraint, 50)
		if e2 != nil {
			return Commit{}, e2
		}
		for _, p := range plans {
			b, _ := json.Marshal(p)
			out = append(out, PendingResult{StableKey: fmt.Sprintf("%d/%d", p.From.StationID, p.To.StationID), Score: p.NetProfit + p.LoadFactor*1e6, Payload: b})
		}
	} else if j.Mode == "chain" && len(cs) > 0 {
		edges := []trade.Route{}
		seen := map[string]bool{}
		for i, c := range cs {
			k := fmt.Sprintf("%d/%d", c.From.StationID, c.To.StationID)
			if seen[k] {
				continue
			}
			seen[k] = true
			r := candidateRoutes[i]
			systems := make([]trade.RouteSystem, len(r.Systems))
			for z, s := range r.Systems {
				systems[z] = trade.RouteSystem{SystemID: s.ID, Name: s.Name, SecurityStatus: s.Security}
			}
			edges = append(edges, trade.Route{From: c.From, To: c.To, Jumps: r.Jumps, MinSecurity: r.MinSecurity, Systems: systems})
		}
		plans, e2 := trade.SearchPickupDeliveryPlansContext(ctx, cs[0].From, edges, cs, constraint)
		if e2 != nil {
			return Commit{}, e2
		}
		for i, p := range plans {
			b, _ := json.Marshal(p)
			out = append(out, PendingResult{StableKey: fmt.Sprintf("chain/%d", i), Score: p.RealizedProfit, Payload: b})
		}
	}
	progress, _ := json.Marshal(map[string]any{"phase": "watching", "candidates": len(cs), "results": len(out)})
	return Commit{State: "watching", Progress: progress, Results: out}, nil
}
