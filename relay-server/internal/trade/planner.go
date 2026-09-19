package trade

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"relay-server/internal/esi"
)

const (
	MaxPlanStops = 4
	MaxPlanLegs  = 3
)

// Constraint contains the hard limits and portfolio controls used by the planner.
type Constraint struct {
	Budget               float64 `json:"budget"`
	BudgetReserve        float64 `json:"budgetReserve"`
	CargoM3              float64 `json:"cargoM3"`
	MaxItemConcentration float64 `json:"maxItemConcentration"`
	MinSecurity          float64 `json:"minSecurity"`
	MaxStops             int     `json:"maxStops"`
	MaxLegs              int     `json:"maxLegs"`
	BeamWidth            int     `json:"beamWidth"`
	MaxItemsPerLeg       int     `json:"maxItemsPerLeg"`
	TransportCostPerM3   float64 `json:"transportCostPerM3"`
	JumpPenalty          float64 `json:"jumpPenalty"`
	StopPenalty          float64 `json:"stopPenalty"`
	OptimizationStates   int     `json:"optimizationStates"`
	TargetLoadFactor     float64 `json:"targetLoadFactor"`
}

// Snapshot identifies the immutable market view from which a plan was built.
type Snapshot struct {
	AsOf       time.Time `json:"asOf"`
	FetchedAt  time.Time `json:"fetchedAt"`
	RegionIDs  []int64   `json:"regionIds,omitempty"`
	OrderCount int       `json:"orderCount"`
}

// LoadItem is an executable quantity of one type on a leg.
type LoadItem struct {
	TypeID     int64   `json:"typeId"`
	Quantity   int64   `json:"quantity"`
	UnitVolume float64 `json:"unitVolume"`
	UnitCost   float64 `json:"unitCost"`
	UnitReturn float64 `json:"unitReturn"`
	Capital    float64 `json:"capital"`
	VolumeM3   float64 `json:"volumeM3"`
	NetProfit  float64 `json:"netProfit"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
}

// Stop describes one station visit. Loads are purchases made at this stop.
type Stop struct {
	Hub   Hub        `json:"hub"`
	Loads []LoadItem `json:"loads,omitempty"`
}

// Leg moves a selected load between two stops.
type Leg struct {
	From        Hub        `json:"from"`
	To          Hub        `json:"to"`
	Items       []LoadItem `json:"items"`
	Jumps       int        `json:"jumps"`
	MinSecurity float64    `json:"minSecurity"`
	Capital     float64    `json:"capital"`
	VolumeM3    float64    `json:"volumeM3"`
	NetProfit   float64    `json:"netProfit"`
	Score       float64    `json:"score"`
	Confidence  float64    `json:"confidence"`
}

// PriceLevel is one bounded order-book price segment. Levels must be supplied
// in execution priority (asks ascending, bids descending).
type PriceLevel struct {
	Price         float64 `json:"price"`
	Volume        int64   `json:"volume"`
	MinimumVolume int64   `json:"minimumVolume,omitempty"`
}

// Candidate is a single-item, single-route opportunity before portfolio selection.
// AskLevels/BidLevels allow quantities to consume multiple price levels. Legacy
// constant unit prices remain supported when curves are omitted.
type Candidate struct {
	TypeID       int64
	From, To     Hub
	MaxQuantity  int64
	UnitVolume   float64
	UnitCost     float64
	UnitReturn   float64
	NetPerUnit   float64
	AskLevels    []PriceLevel `json:"askLevels,omitempty"`
	BidLevels    []PriceLevel `json:"bidLevels,omitempty"`
	SalesTaxRate float64      `json:"salesTaxRate,omitempty"`
	BrokerRate   float64      `json:"brokerRate,omitempty"`
	Score        float64
	Confidence   float64
	Jumps        int
	MinSecurity  float64
}

// Route describes an allowed directed station-to-station movement.
type PackPlan struct {
	From           Hub        `json:"from"`
	To             Hub        `json:"to"`
	Items          []LoadItem `json:"items"`
	Capital        float64    `json:"capital"`
	VolumeM3       float64    `json:"volumeM3"`
	CargoCapacity  float64    `json:"cargoCapacity"`
	LoadFactor     float64    `json:"loadFactor"`
	NetProfit      float64    `json:"netProfit"`
	Jumps          int        `json:"jumps"`
	MinSecurity    float64    `json:"minSecurity"`
	UnfilledReason string     `json:"unfilledReason,omitempty"`
}

// PackFixedRoute enforces one purchase location and one sale location before
// optimizing quantities. It returns an executable whole-load summary.
func PackFixedRoute(candidates []Candidate, c Constraint) (PackPlan, error) {
	var out PackPlan
	if len(candidates) == 0 {
		return out, nil
	}
	out.From, out.To = candidates[0].From, candidates[0].To
	filtered := make([]Candidate, 0, len(candidates))
	for _, x := range candidates {
		if x.From.Code != out.From.Code || x.To.Code != out.To.Code || x.From.StationID != out.From.StationID || x.To.StationID != out.To.StationID {
			continue
		}
		filtered = append(filtered, x)
	}
	items, err := PackCandidates(filtered, c)
	if err != nil {
		return out, err
	}
	out.Items = items
	out.CargoCapacity = c.CargoM3
	for _, it := range items {
		out.Capital += it.Capital
		out.VolumeM3 += it.VolumeM3
		out.NetProfit += it.NetProfit
	}
	if c.CargoM3 > 0 {
		out.LoadFactor = out.VolumeM3 / c.CargoM3
	}
	for _, x := range filtered {
		if x.Jumps > out.Jumps {
			out.Jumps = x.Jumps
		}
		if out.MinSecurity == 0 || x.MinSecurity < out.MinSecurity {
			out.MinSecurity = x.MinSecurity
		}
	}
	if out.LoadFactor < .999999 {
		spendable := c.Budget - c.BudgetReserve
		switch {
		case out.Capital >= spendable-1e-6:
			out.UnfilledReason = "budget_exhausted"
		case len(items) == 0:
			out.UnfilledReason = "no_profitable_depth"
		default:
			out.UnfilledReason = "profitable_depth_or_volume_fragment_exhausted"
		}
	}
	return out, nil
}

// PackByRoute groups candidates by exact purchase/sale station pair and returns
// the best bounded whole-load plans across those executable pairs.
func PackByRoute(candidates []Candidate, c Constraint, limit int) ([]PackPlan, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	groups := map[string][]Candidate{}
	for _, x := range candidates {
		if x.From.StationID <= 0 || x.To.StationID <= 0 || x.From.StationID == x.To.StationID {
			continue
		}
		k := fmt.Sprintf("%d/%d", x.From.StationID, x.To.StationID)
		groups[k] = append(groups[k], x)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PackPlan, 0, len(keys))
	for _, k := range keys {
		p, err := PackFixedRoute(groups[k], c)
		if err != nil {
			return nil, err
		}
		if len(p.Items) > 0 {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NetProfit != out[j].NetProfit {
			return out[i].NetProfit > out[j].NetProfit
		}
		if out[i].LoadFactor != out[j].LoadFactor {
			return out[i].LoadFactor > out[j].LoadFactor
		}
		return out[i].From.StationID < out[j].From.StationID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type RouteSystem struct {
	SystemID       int64   `json:"systemId"`
	Name           string  `json:"name"`
	SecurityStatus float64 `json:"securityStatus"`
}
type Route struct {
	From, To    Hub
	Jumps       int
	MinSecurity float64
	Systems     []RouteSystem `json:"systems,omitempty"`
}

// OrderScope pins an order set to the requested region, station, type and side.
type OrderScope struct {
	RegionID  int64
	StationID int64
	TypeID    int64
	Buy       bool
}

// ValidateOrderScope rejects mixed/wrong-station data before it can influence planning.
func ValidateOrderScope(scope OrderScope, orders []esi.MarketOrder) error {
	if scope.RegionID <= 0 || scope.StationID <= 0 || scope.TypeID <= 0 {
		return errors.New("invalid order scope")
	}
	for _, o := range orders {
		if o.TypeID != 0 && o.TypeID != scope.TypeID {
			return errors.New("order type outside scope")
		}
		if o.LocationID != scope.StationID {
			return errors.New("order location outside scope")
		}
		if o.IsBuyOrder != scope.Buy {
			return errors.New("order side outside scope")
		}
		if o.Price <= 0 || o.VolumeRemain <= 0 {
			return errors.New("invalid order in scope")
		}
	}
	return nil
}

func normalizedConstraint(c Constraint) (Constraint, error) {
	if c.Budget < 0 || c.BudgetReserve < 0 || c.CargoM3 < 0 || c.MinSecurity < -1 || c.MinSecurity > 1 {
		return c, errors.New("invalid constraint")
	}
	if c.BudgetReserve > c.Budget {
		return c, errors.New("budget reserve exceeds budget")
	}
	if c.MaxItemConcentration == 0 {
		c.MaxItemConcentration = 1
	}
	if c.TargetLoadFactor == 0 {
		c.TargetLoadFactor = .9
	}
	if c.TargetLoadFactor <= 0 || c.TargetLoadFactor > 1 {
		return c, errors.New("invalid target load factor")
	}
	if c.MaxItemConcentration <= 0 || c.MaxItemConcentration > 1 {
		return c, errors.New("invalid item concentration")
	}
	if c.MaxStops == 0 || c.MaxStops > MaxPlanStops {
		c.MaxStops = MaxPlanStops
	}
	if c.MaxLegs == 0 || c.MaxLegs > MaxPlanLegs {
		c.MaxLegs = MaxPlanLegs
	}
	if c.BeamWidth <= 0 {
		c.BeamWidth = 16
	}
	if c.MaxItemsPerLeg <= 0 {
		c.MaxItemsPerLeg = 12
	}
	if c.OptimizationStates <= 0 {
		c.OptimizationStates = 256
	}
	if c.OptimizationStates > 4096 {
		c.OptimizationStates = 4096
	}
	if c.JumpPenalty < 0 || c.StopPenalty < 0 || c.TransportCostPerM3 < 0 {
		return c, errors.New("invalid planning penalty")
	}
	return c, nil
}

// PackCandidates uses a bounded multiple-choice knapsack search. It considers
// order-depth quantity breakpoints instead of greedily exhausting the first item,
// which lets capital and cargo compete on total net profit. Candidates sharing a
// destination naturally batch into one stop; additional destinations and jumps
// carry explicit last-mile penalties.
func PackCandidates(candidates []Candidate, c Constraint) ([]LoadItem, error) {
	c, err := normalizedConstraint(c)
	if err != nil {
		return nil, err
	}
	caps := []float64{c.MaxItemConcentration}
	for _, cap := range []float64{.4, .6, 1} {
		if cap > caps[len(caps)-1]+1e-9 {
			caps = append(caps, cap)
		}
	}
	var best []LoadItem
	bestVolume, bestProfit := 0.0, -math.MaxFloat64
	for _, cap := range caps {
		cc := c
		cc.MaxItemConcentration = cap
		items, e := packCandidatesAtConcentration(candidates, cc)
		if e != nil {
			return nil, e
		}
		v, p := 0.0, 0.0
		for _, it := range items {
			v += it.VolumeM3
			p += it.NetProfit
		}
		if v > bestVolume+1e-9 || (math.Abs(v-bestVolume) < 1e-9 && p > bestProfit) {
			best, bestVolume, bestProfit = items, v, p
		}
		if c.CargoM3 > 0 && v/c.CargoM3 >= c.TargetLoadFactor {
			return items, nil
		}
	}
	return best, nil
}

func packCandidatesAtConcentration(candidates []Candidate, c Constraint) ([]LoadItem, error) {
	spendable := c.Budget - c.BudgetReserve
	if spendable <= 0 || c.CargoM3 <= 0 {
		return []LoadItem{}, nil
	}
	cs := append([]Candidate(nil), candidates...)
	sort.SliceStable(cs, func(i, j int) bool {
		pi := cs[i].NetPerUnit / math.Max(cs[i].UnitCost, 1e-12)
		pj := cs[j].NetPerUnit / math.Max(cs[j].UnitCost, 1e-12)
		if pi != pj {
			return pi > pj
		}
		if cs[i].TypeID != cs[j].TypeID {
			return cs[i].TypeID < cs[j].TypeID
		}
		return cs[i].To.Code < cs[j].To.Code
	})
	if len(cs) > c.MaxItemsPerLeg {
		cs = cs[:c.MaxItemsPerLeg]
	}
	type packState struct {
		items                            []LoadItem
		capital, volume, profit, utility float64
		destinations                     map[string]struct{}
	}
	states := []packState{{destinations: map[string]struct{}{}}}
	perItemCap := spendable * c.MaxItemConcentration
	for _, x := range cs {
		if x.TypeID <= 0 || x.MaxQuantity <= 0 || x.UnitVolume <= 0 || (len(x.AskLevels) == 0 && (x.UnitCost <= 0 || x.NetPerUnit <= 0)) {
			continue
		}
		minCost := x.UnitCost
		if len(x.AskLevels) > 0 {
			minCost = x.AskLevels[0].Price
		}
		if minCost <= 0 {
			continue
		}
		maxQ := min64(x.MaxQuantity, min64(int64(math.Floor(perItemCap/minCost)), int64(math.Floor(c.CargoM3/x.UnitVolume))))
		if maxQ <= 0 {
			continue
		}
		quantities := quantityChoicesForCandidate(x, maxQ)
		next := make([]packState, 0, len(states)*len(quantities))
		for _, s := range states {
			for _, q := range quantities {
				capital, revenue, net, ok := candidateValue(x, q)
				volume := float64(q) * x.UnitVolume
				if !ok || s.capital+capital > spendable+1e-9 || s.volume+volume > c.CargoM3+1e-9 {
					continue
				}
				ns := packState{items: append([]LoadItem(nil), s.items...), capital: s.capital + capital, volume: s.volume + volume, profit: s.profit + net, destinations: cloneSet(s.destinations)}
				if q > 0 {
					ns.items = append(ns.items, LoadItem{TypeID: x.TypeID, Quantity: q, UnitVolume: x.UnitVolume, UnitCost: capital / float64(q), UnitReturn: revenue / float64(q), Capital: capital, VolumeM3: volume, NetProfit: net, Score: x.Score, Confidence: clamp(x.Confidence)})
					ns.destinations[x.To.Code] = struct{}{}
				}
				ns.utility = ns.profit - float64(x.Jumps)*c.JumpPenalty - float64(maxInt(0, len(ns.destinations)-1))*c.StopPenalty
				next = append(next, ns)
			}
		}
		sort.SliceStable(next, func(i, j int) bool {
			if next[i].utility != next[j].utility {
				return next[i].utility > next[j].utility
			}
			if next[i].profit != next[j].profit {
				return next[i].profit > next[j].profit
			}
			return packKey(next[i].items) < packKey(next[j].items)
		})
		if len(next) > c.OptimizationStates {
			next = next[:c.OptimizationStates]
		}
		states = next
	}
	if len(states) == 0 {
		return []LoadItem{}, nil
	}
	return states[0].items, nil
}

func consumeLevels(levels []PriceLevel, q int64) (float64, bool) {
	if q == 0 {
		return 0, true
	}
	left, total := q, 0.0
	for _, l := range levels {
		if l.Price <= 0 || l.Volume <= 0 {
			continue
		}
		take := min64(left, l.Volume)
		if take > 0 && l.MinimumVolume > 1 && take < l.MinimumVolume {
			continue
		}
		total += float64(take) * l.Price
		left -= take
		if left == 0 {
			return total, true
		}
	}
	return 0, false
}

func candidateValue(x Candidate, q int64) (capital, revenue, net float64, ok bool) {
	if q == 0 {
		return 0, 0, 0, true
	}
	// Legacy candidates are already fee-valued by the candidate service.
	if len(x.AskLevels) == 0 && len(x.BidLevels) == 0 {
		capital, net = float64(q)*x.UnitCost, float64(q)*x.NetPerUnit
		return capital, capital + net, net, x.UnitCost > 0 && net > 0
	}
	if len(x.AskLevels) > 0 {
		capital, ok = consumeLevels(x.AskLevels, q)
	} else {
		capital, ok = float64(q)*x.UnitCost, x.UnitCost > 0
	}
	if !ok {
		return 0, 0, 0, false
	}
	if len(x.BidLevels) > 0 {
		revenue, ok = consumeLevels(x.BidLevels, q)
	} else {
		revenue, ok = float64(q)*x.UnitReturn, x.UnitReturn > 0
	}
	if !ok {
		return 0, 0, 0, false
	}
	fees := revenue*x.SalesTaxRate + capital*x.BrokerRate
	net = revenue - capital - fees
	return capital, revenue, net, net > 0
}

func quantityChoicesForCandidate(x Candidate, maxQ int64) []int64 {
	values := quantityChoices(maxQ)
	for _, levels := range [][]PriceLevel{x.AskLevels, x.BidLevels} {
		var cumulative int64
		for _, l := range levels {
			cumulative += l.Volume
			if cumulative <= maxQ {
				values = append(values, cumulative)
			}
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	out := values[:0]
	var last int64 = -1
	for _, q := range values {
		if q != last {
			out = append(out, q)
			last = q
		}
	}
	return out
}

func quantityChoices(maxQ int64) []int64 {
	values := []int64{0, 1, maxQ / 4, maxQ / 2, (3 * maxQ) / 4, maxQ}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(values))
	for _, q := range values {
		if q >= 0 && q <= maxQ && !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
	}
	return out
}
func cloneSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in)+1)
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
func packKey(items []LoadItem) string {
	x := ""
	for _, it := range items {
		x += fmt.Sprintf("/%d:%d", it.TypeID, it.Quantity)
	}
	return x
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type CargoLot struct {
	Item LoadItem `json:"item"`
	From Hub      `json:"from"`
	To   Hub      `json:"to"`
}
type ChainStop struct {
	Hub           Hub        `json:"hub"`
	ArriveVia     *Route     `json:"arriveVia,omitempty"`
	Loads         []CargoLot `json:"loads,omitempty"`
	Unloads       []CargoLot `json:"unloads,omitempty"`
	CashBefore    float64    `json:"cashBefore"`
	SaleRevenue   float64    `json:"saleRevenue"`
	PurchaseCost  float64    `json:"purchaseCost"`
	CashAfter     float64    `json:"cashAfter"`
	CargoBeforeM3 float64    `json:"cargoBeforeM3"`
	UnloadedM3    float64    `json:"unloadedM3"`
	LoadedM3      float64    `json:"loadedM3"`
	CargoAfterM3  float64    `json:"cargoAfterM3"`
}
type PickupDeliveryPlan struct {
	Stops          []ChainStop `json:"stops"`
	Items          []LoadItem  `json:"items,omitempty"`
	Inventory      []CargoLot  `json:"inventory,omitempty"`
	RealizedProfit float64     `json:"realizedProfit"`
	Capital        float64     `json:"capital"`
	VolumeM3       float64     `json:"volumeM3"`
	TotalJumps     int         `json:"totalJumps"`
	MinSecurity    float64     `json:"minSecurity"`
}

// SimulatePickupDelivery verifies station actions sequentially. Capital and cargo
// are released only when a carried lot reaches its committed sale station.
func SimulatePickupDelivery(start Hub, visits []Hub, candidates []Candidate, c Constraint) (PickupDeliveryPlan, error) {
	c, err := normalizedConstraint(c)
	if err != nil {
		return PickupDeliveryPlan{}, err
	}
	cash := c.Budget - c.BudgetReserve
	if cash < 0 {
		return PickupDeliveryPlan{}, errors.New("negative spendable cash")
	}
	plan := PickupDeliveryPlan{}
	inventory := []CargoLot{}
	at := start
	for _, visit := range visits {
		st := ChainStop{Hub: visit, CashBefore: cash}
		for _, lot := range inventory {
			st.CargoBeforeM3 += lot.Item.VolumeM3
		}
		remaining := inventory[:0]
		for _, lot := range inventory {
			if lot.To.StationID == visit.StationID {
				st.Unloads = append(st.Unloads, lot)
				revenue := lot.Item.Capital + lot.Item.NetProfit
				cash += revenue
				st.SaleRevenue += revenue
				st.UnloadedM3 += lot.Item.VolumeM3
				plan.RealizedProfit += lot.Item.NetProfit
			} else {
				remaining = append(remaining, lot)
			}
		}
		inventory = remaining
		var eligible []Candidate
		for _, x := range candidates {
			if x.From.StationID == visit.StationID && x.To.StationID != visit.StationID {
				eligible = append(eligible, x)
			}
		}
		used := st.CargoBeforeM3 - st.UnloadedM3
		legC := c
		legC.Budget = cash
		legC.BudgetReserve = 0
		legC.CargoM3 = c.CargoM3 - used
		loads, e := PackCandidates(eligible, legC)
		if e != nil {
			return PickupDeliveryPlan{}, e
		}
		for _, it := range loads {
			plan.Items = append(plan.Items, it)
			plan.Capital += it.Capital
			var dst Hub
			for _, x := range eligible {
				if x.TypeID == it.TypeID {
					dst = x.To
					break
				}
			}
			cash -= it.Capital
			st.PurchaseCost += it.Capital
			st.LoadedM3 += it.VolumeM3
			lot := CargoLot{Item: it, From: visit, To: dst}
			st.Loads = append(st.Loads, lot)
			inventory = append(inventory, lot)
		}
		if cash < -1e-6 || used+st.LoadedM3 > c.CargoM3+1e-6 {
			return PickupDeliveryPlan{}, errors.New("infeasible chain state")
		}
		st.CashAfter = cash
		st.CargoAfterM3 = used + st.LoadedM3
		if st.CargoAfterM3 > plan.VolumeM3 {
			plan.VolumeM3 = st.CargoAfterM3
		}
		plan.Stops = append(plan.Stops, st)
		at = visit
		_ = at
	}
	plan.Inventory = inventory
	return plan, nil
}

type deliveryBeamState struct {
	visits      []Hub
	plan        PickupDeliveryPlan
	utility     float64
	jumps       int
	minSecurity float64
}

// SearchPickupDeliveryPlans expands real station visits and re-simulates cash,
// inventory and cargo at every step. Only realized delivery profit is scored.
func SearchPickupDeliveryPlans(start Hub, routes []Route, candidates []Candidate, c Constraint) ([]PickupDeliveryPlan, error) {
	return SearchPickupDeliveryPlansContext(context.Background(), start, routes, candidates, c)
}
func SearchPickupDeliveryPlansContext(ctx context.Context, start Hub, routes []Route, candidates []Candidate, c Constraint) ([]PickupDeliveryPlan, error) {
	c, err := normalizedConstraint(c)
	if err != nil {
		return nil, err
	}
	beam := []deliveryBeamState{{visits: []Hub{start}, minSecurity: 1}}
	var completed []deliveryBeamState
	for depth := 0; depth < c.MaxStops-1; depth++ {
		var next []deliveryBeamState
		for _, s := range beam {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			at := s.visits[len(s.visits)-1]
			for _, r := range routes {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if r.From.StationID != at.StationID || r.To.StationID == at.StationID || r.MinSecurity < c.MinSecurity {
					continue
				}
				seen := false
				for _, v := range s.visits {
					if v.StationID == r.To.StationID {
						seen = true
						break
					}
				}
				if seen {
					continue
				}
				visits := append(append([]Hub(nil), s.visits...), r.To)
				plan, e := SimulatePickupDelivery(start, visits, candidates, c)
				if e != nil {
					continue
				}
				for i := 1; i < len(visits) && i < len(plan.Stops); i++ {
					for _, edge := range routes {
						if edge.From.StationID == visits[i-1].StationID && edge.To.StationID == visits[i].StationID {
							leg := edge
							plan.Stops[i].ArriveVia = &leg
							break
						}
					}
				}
				jumps := s.jumps + r.Jumps
				if jumps > 256 {
					continue
				}
				utility := plan.RealizedProfit - float64(jumps)*c.JumpPenalty - float64(len(visits)-1)*c.StopPenalty
				ns := deliveryBeamState{visits: visits, plan: plan, utility: utility, jumps: jumps, minSecurity: math.Min(s.minSecurity, r.MinSecurity)}
				next = append(next, ns)
				if plan.RealizedProfit > 0 {
					completed = append(completed, ns)
				}
			}
		}
		sort.SliceStable(next, func(i, j int) bool {
			if next[i].utility != next[j].utility {
				return next[i].utility > next[j].utility
			}
			return deliveryVisitKey(next[i].visits) < deliveryVisitKey(next[j].visits)
		})
		if len(next) > c.BeamWidth {
			next = next[:c.BeamWidth]
		}
		beam = next
		if len(beam) == 0 {
			break
		}
	}
	sort.SliceStable(completed, func(i, j int) bool {
		if completed[i].utility != completed[j].utility {
			return completed[i].utility > completed[j].utility
		}
		return deliveryVisitKey(completed[i].visits) < deliveryVisitKey(completed[j].visits)
	})
	out := make([]PickupDeliveryPlan, 0, len(completed))
	for _, s := range completed {
		p := s.plan
		p.TotalJumps = s.jumps
		p.MinSecurity = s.minSecurity
		out = append(out, p)
	}
	return out, nil
}
func deliveryVisitKey(v []Hub) string {
	k := ""
	for _, h := range v {
		k += fmt.Sprintf("/%d", h.StationID)
	}
	return k
}

type beamState struct {
	stops      []Stop
	legs       []Leg
	profit     float64
	score      float64
	confidence float64
}

// SearchPlans performs a deterministic bounded beam search. Candidates must be
// precomputed single-item opportunities; only routes satisfying MinSecurity enter.
func SearchPlans(start Hub, routes []Route, candidates []Candidate, snapshot Snapshot, constraint Constraint) ([]Plan, error) {
	c, err := normalizedConstraint(constraint)
	if err != nil {
		return nil, err
	}
	beam := []beamState{{stops: []Stop{{Hub: start}}, confidence: 1}}
	var completed []beamState
	for depth := 0; depth < c.MaxLegs; depth++ {
		var next []beamState
		for _, s := range beam {
			at := s.stops[len(s.stops)-1].Hub
			for _, r := range routes {
				if r.From.Code != at.Code || r.To.Code == at.Code || r.MinSecurity < c.MinSecurity {
					continue
				}
				seen := false
				for _, stop := range s.stops {
					if stop.Hub.Code == r.To.Code {
						seen = true
						break
					}
				}
				if seen || len(s.stops) >= c.MaxStops {
					continue
				}
				var eligible []Candidate
				for _, x := range candidates {
					if x.From.Code == r.From.Code && x.To.Code == r.To.Code && x.MinSecurity >= c.MinSecurity {
						eligible = append(eligible, x)
					}
				}
				// Every destination sale releases cargo and principal; realized profit is
				// available to fund later legs while the configured reserve remains locked.
				legConstraint := c
				legConstraint.Budget = c.Budget + s.profit
				items, packErr := PackCandidates(eligible, legConstraint)
				if packErr != nil || len(items) == 0 {
					continue
				}
				leg := summarizeLeg(r, items, c.TransportCostPerM3)
				leg.NetProfit -= float64(r.Jumps)*c.JumpPenalty + c.StopPenalty
				if leg.NetProfit <= 0 {
					continue
				}
				ns := beamState{stops: append(append([]Stop(nil), s.stops...), Stop{Hub: r.To}), legs: append(append([]Leg(nil), s.legs...), leg), profit: s.profit + leg.NetProfit, score: s.score + leg.Score, confidence: math.Min(s.confidence, leg.Confidence)}
				ns.stops[len(ns.stops)-2].Loads = append([]LoadItem(nil), items...)
				next = append(next, ns)
				completed = append(completed, ns)
			}
		}
		sortStates(next)
		if len(next) > c.BeamWidth {
			next = next[:c.BeamWidth]
		}
		beam = next
		if len(beam) == 0 {
			break
		}
	}
	sortStates(completed)
	plans := make([]Plan, 0, len(completed))
	for _, s := range completed {
		plans = append(plans, Plan{Stops: s.stops, Legs: s.legs, Snapshot: snapshot, Constraint: c, NetProfit: s.profit, PlanScore: s.score, PlanConfidence: clamp(s.confidence)})
	}
	return plans, nil
}

func summarizeLeg(r Route, items []LoadItem, transportPerM3 float64) Leg {
	l := Leg{From: r.From, To: r.To, Items: items, Jumps: r.Jumps, MinSecurity: r.MinSecurity, Confidence: 1}
	var weighted float64
	for _, x := range items {
		l.Capital += x.Capital
		l.VolumeM3 += x.VolumeM3
		l.NetProfit += x.NetProfit
		weighted += x.Score * x.Capital
		l.Confidence = math.Min(l.Confidence, x.Confidence)
	}
	l.NetProfit -= l.VolumeM3 * transportPerM3
	if l.Capital > 0 {
		l.Score = weighted / l.Capital
	}
	return l
}

func sortStates(s []beamState) {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].score != s[j].score {
			return s[i].score > s[j].score
		}
		if s[i].profit != s[j].profit {
			return s[i].profit > s[j].profit
		}
		return pathKey(s[i].stops) < pathKey(s[j].stops)
	})
}

func pathKey(stops []Stop) string {
	var x string
	for _, s := range stops {
		x += "/" + s.Hub.Code
	}
	return x
}
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
