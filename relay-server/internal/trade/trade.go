// Package trade computes read-only, risk-adjusted regional trade opportunities.
package trade

import (
	"math"
	"sort"
	"time"

	"relay-server/internal/esi"
)

type Hub struct {
	Code, Name, NameZH            string
	RegionID, StationID, SystemID int64
}

var Hubs = []Hub{
	{"jita", "Jita", "吉他", 10000002, 60003760, 30000142},
	{"dodixie", "Dodixie", "多迪谢", 10000032, 60011866, 30002659},
	{"amarr", "Amarr", "阿玛", 10000043, 60008494, 30002187},
	{"rens", "Rens", "伦斯", 10000030, 60004588, 30002510},
	{"hek", "Hek", "赫克", 10000042, 60005686, 30002053},
}

func HubByCode(code string) (Hub, bool) {
	for _, h := range Hubs {
		if h.Code == code {
			return h, true
		}
	}
	return Hub{}, false
}

type Params struct {
	Quantity                                  int64 `json:"quantity"`
	SalesTaxRate, BrokerRate                  float64
	TransportCostPerM3, CargoM3, ItemVolumeM3 float64
	RouteJumps, LowSecJumps                   int
	MinSecurity, RouteMinSecurity             float64
	AverageDailyVolume                        float64
	HistoryDays                               int
	MaxDataAge                                time.Duration
	Now, FetchedAt                            time.Time
}
type Opportunity struct {
	TypeID                                                                                                                              int64 `json:"typeId"`
	Source, Destination                                                                                                                 Hub
	Quantity                                                                                                                            int64 `json:"quantity"`
	AskVWAP, BidVWAP, GrossProfit, Fees, TransportCost, NetProfit, Margin, Capital                                                      float64
	VolumeM3, DailyVolume, DaysToSell, BuySlippage, SellSlippage                                                                        float64
	RouteJumps, LowSecJumps                                                                                                             int
	ProfitQuality, Demand, Liquidity, Execution, RouteSafety, CapitalEfficiency, ProfitPerM3, ROI, SnapshotFreshness, Confidence, Score float64
	Eligible                                                                                                                            bool     `json:"eligible"`
	Reasons                                                                                                                             []string `json:"reasons,omitempty"`
}

type level struct {
	price     float64
	qty       int64
	minVolume int64
}

func stationLevels(orders []esi.MarketOrder, station int64, buy bool) []level {
	out := []level{}
	for _, o := range orders {
		if o.LocationID == station && o.IsBuyOrder == buy && o.Price > 0 && o.VolumeRemain > 0 {
			out = append(out, level{price: o.Price, qty: o.VolumeRemain, minVolume: o.MinVolume})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if buy {
			return out[i].price > out[j].price
		}
		return out[i].price < out[j].price
	})
	return out
}
func vwap(levels []level, q int64) (float64, int64, float64) {
	var n int64
	var total float64
	best := 0.0
	if len(levels) > 0 {
		best = levels[0].price
	}
	for _, l := range levels {
		remaining := q - n
		if l.minVolume > 0 && remaining < l.minVolume {
			continue
		}
		take := l.qty
		if take > q-n {
			take = q - n
		}
		total += float64(take) * l.price
		n += take
		if n == q {
			break
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	v := total / float64(n)
	slip := 0.0
	if best > 0 {
		slip = math.Abs(v-best) / best
	}
	return v, n, slip
}
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
func Evaluate(typeID int64, src, dst Hub, srcOrders, dstOrders []esi.MarketOrder, p Params) Opportunity {
	o := Opportunity{TypeID: typeID, Source: src, Destination: dst, Quantity: p.Quantity, RouteJumps: p.RouteJumps, LowSecJumps: p.LowSecJumps, DailyVolume: p.AverageDailyVolume}
	if p.Quantity <= 0 {
		o.Reasons = append(o.Reasons, "quantity_invalid")
		return o
	}
	ask, aq, as := vwap(stationLevels(srcOrders, src.StationID, false), p.Quantity)
	bid, bq, bs := vwap(stationLevels(dstOrders, dst.StationID, true), p.Quantity)
	q := aq
	if bq < q {
		q = bq
	}
	o.Quantity = q
	if q == 0 {
		o.Reasons = append(o.Reasons, "no_executable_depth")
		return o
	}
	// Both sides must be repriced at the final common executable quantity.
	ask, _, as = vwap(stationLevels(srcOrders, src.StationID, false), q)
	bid, _, bs = vwap(stationLevels(dstOrders, dst.StationID, true), q)
	o.AskVWAP, o.BidVWAP, o.BuySlippage, o.SellSlippage = ask, bid, as, bs
	coverage := float64(q) / float64(p.Quantity)
	o.Capital = ask * float64(q)
	revenue := bid * float64(q)
	o.GrossProfit = revenue - o.Capital
	o.Fees = revenue*p.SalesTaxRate + o.Capital*p.BrokerRate
	o.VolumeM3 = float64(q) * p.ItemVolumeM3
	o.TransportCost = o.VolumeM3 * p.TransportCostPerM3
	o.NetProfit = o.GrossProfit - o.Fees - o.TransportCost
	if o.Capital > 0 {
		o.Margin = o.NetProfit / o.Capital
	}
	if p.AverageDailyVolume > 0 {
		o.DaysToSell = float64(q) / p.AverageDailyVolume
	}
	o.ROI = o.Margin
	if o.VolumeM3 > 0 {
		o.ProfitPerM3 = o.NetProfit / o.VolumeM3
	}
	o.ProfitQuality = clamp(o.Margin/0.25)*.35 + clamp(o.NetProfit/500000000)*.40 + clamp(o.ProfitPerM3/1_000_000)*.25
	o.Demand = clamp(p.AverageDailyVolume / (float64(q)/3 + 1))
	o.Liquidity = clamp(math.Log10(p.AverageDailyVolume+1) / 4)
	o.Execution = clamp(1-(as+bs)*5) * coverage
	o.RouteSafety = clamp(1 - float64(p.LowSecJumps)*.2 - float64(p.RouteJumps)*.015)
	if p.RouteMinSecurity != 0 {
		o.RouteSafety *= clamp((p.RouteMinSecurity + 1) / 2)
	}
	o.CapitalEfficiency = clamp(o.NetProfit / math.Max(o.Capital, 1) / .20)
	confidence := coverage
	if p.HistoryDays < 14 {
		confidence *= float64(p.HistoryDays) / 14
	}
	maxAge := p.MaxDataAge
	if maxAge == 0 {
		maxAge = 15 * time.Minute
	}
	o.SnapshotFreshness = 1
	if !p.Now.IsZero() && !p.FetchedAt.IsZero() {
		o.SnapshotFreshness = clamp(1 - math.Max(0, p.Now.Sub(p.FetchedAt).Seconds())/maxAge.Seconds())
		confidence *= o.SnapshotFreshness
	}
	o.Confidence = clamp(confidence)
	base := .30*o.ProfitQuality + .15*clamp(o.ROI/.20) + .10*clamp(o.ProfitPerM3/1_000_000) + .10*o.Demand + .08*o.Liquidity + .07*o.Execution + .08*o.RouteSafety + .07*o.CapitalEfficiency + .05*o.SnapshotFreshness
	o.Score = 100 * base * o.Confidence
	if coverage < .95 {
		o.Reasons = append(o.Reasons, "insufficient_depth")
	}
	if o.NetProfit < 50000000 {
		o.Reasons = append(o.Reasons, "net_profit_below_threshold")
	}
	if o.Margin < .05 {
		o.Reasons = append(o.Reasons, "margin_below_threshold")
	}
	if p.AverageDailyVolume < float64(q)/3 {
		o.Reasons = append(o.Reasons, "demand_too_low")
	}
	if p.HistoryDays < 14 {
		o.Reasons = append(o.Reasons, "history_insufficient")
	}
	if p.CargoM3 > 0 && o.VolumeM3 > p.CargoM3 {
		o.Reasons = append(o.Reasons, "cargo_exceeded")
	}
	if p.MinSecurity != 0 && p.RouteMinSecurity < p.MinSecurity {
		o.Reasons = append(o.Reasons, "route_security_below_minimum")
	}
	o.Eligible = len(o.Reasons) == 0
	return o
}
