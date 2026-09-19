package trade

import (
	"context"
	"errors"
	"relay-server/internal/esi"
	"sort"
	"time"
)

type MarketSource interface {
	RegionOrders(context.Context, int64, string, int64) ([]esi.MarketOrder, esi.Response, error)
	RegionHistory(context.Context, int64, int64) ([]esi.MarketHistory, esi.Response, error)
}
type Service struct {
	ESI MarketSource
	Now func() time.Time
}
type Quote struct {
	Hub           Hub       `json:"hub"`
	TypeID        int64     `json:"typeId"`
	BestBuy       float64   `json:"bestBuy"`
	BestSell      float64   `json:"bestSell"`
	Spread        float64   `json:"spread"`
	SpreadPercent float64   `json:"spreadPercent"`
	BuyVolume     int64     `json:"buyVolume"`
	SellVolume    int64     `json:"sellVolume"`
	BuyOrders     int       `json:"buyOrders"`
	SellOrders    int       `json:"sellOrders"`
	FetchedAt     time.Time `json:"fetchedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}
type PlanRequest struct {
	TypeID, Quantity                                                    int64
	Source, Destination                                                 string
	SalesTaxRate, BrokerRate, TransportCostPerM3, CargoM3, ItemVolumeM3 float64
	RouteJumps, LowSecJumps                                             int
}
type Plan struct {
	// Legacy point-to-point fields are retained for API compatibility.
	Opportunity        Opportunity `json:"opportunity"`
	SourceQuote        Quote       `json:"sourceQuote"`
	DestinationQuote   Quote       `json:"destinationQuote"`
	HistoryDays        int         `json:"historyDays"`
	AverageDailyVolume float64     `json:"averageDailyVolume"`

	// Planner fields describe a bounded multi-leg plan.
	Stops          []Stop     `json:"stops,omitempty"`
	Legs           []Leg      `json:"legs,omitempty"`
	Snapshot       Snapshot   `json:"snapshot"`
	Constraint     Constraint `json:"constraint"`
	NetProfit      float64    `json:"netProfit"`
	PlanScore      float64    `json:"planScore"`
	PlanConfidence float64    `json:"planConfidence"`
}

func (s *Service) Quote(ctx context.Context, code string, typeID int64) (Quote, []esi.MarketOrder, error) {
	h, ok := HubByCode(code)
	if !ok || typeID <= 0 {
		return Quote{}, nil, errors.New("invalid hub or type")
	}
	orders, meta, err := s.ESI.RegionOrders(ctx, h.RegionID, "all", typeID)
	if err != nil {
		return Quote{}, nil, err
	}
	q := Quote{Hub: h, TypeID: typeID, FetchedAt: s.now(), ExpiresAt: meta.ExpiresAt}
	for _, o := range orders {
		if o.LocationID != h.StationID {
			continue
		}
		if o.IsBuyOrder {
			q.BuyOrders++
			q.BuyVolume += o.VolumeRemain
			if o.Price > q.BestBuy {
				q.BestBuy = o.Price
			}
		} else {
			q.SellOrders++
			q.SellVolume += o.VolumeRemain
			if q.BestSell == 0 || o.Price < q.BestSell {
				q.BestSell = o.Price
			}
		}
	}
	if q.BestBuy > 0 && q.BestSell > 0 {
		q.Spread = q.BestSell - q.BestBuy
		q.SpreadPercent = q.Spread / q.BestBuy
	}
	return q, orders, nil
}
func (s *Service) Compare(ctx context.Context, typeID int64) ([]Quote, error) {
	if typeID <= 0 {
		return nil, errors.New("invalid type")
	}
	out := make([]Quote, 0, len(Hubs))
	for _, h := range Hubs {
		q, _, err := s.Quote(ctx, h.Code, typeID)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}
func (s *Service) BestPlans(ctx context.Context, base PlanRequest) ([]Plan, error) {
	if base.TypeID <= 0 || base.Quantity <= 0 {
		return nil, errors.New("invalid plan request")
	}
	out := make([]Plan, 0, len(Hubs)*(len(Hubs)-1))
	for _, src := range Hubs {
		for _, dst := range Hubs {
			if src.Code == dst.Code {
				continue
			}
			r := base
			r.Source, r.Destination = src.Code, dst.Code
			p, err := s.Plan(ctx, r)
			if err != nil {
				continue
			}
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Opportunity.Eligible != out[j].Opportunity.Eligible {
			return out[i].Opportunity.Eligible
		}
		return out[i].Opportunity.Score > out[j].Opportunity.Score
	})
	return out, nil
}
func (s *Service) Plan(ctx context.Context, r PlanRequest) (Plan, error) {
	src, ok := HubByCode(r.Source)
	if !ok {
		return Plan{}, errors.New("invalid source hub")
	}
	dst, ok := HubByCode(r.Destination)
	if !ok || src.Code == dst.Code {
		return Plan{}, errors.New("invalid destination hub")
	}
	sq, so, err := s.Quote(ctx, src.Code, r.TypeID)
	if err != nil {
		return Plan{}, err
	}
	dq, do, err := s.Quote(ctx, dst.Code, r.TypeID)
	if err != nil {
		return Plan{}, err
	}
	hist, _, err := s.ESI.RegionHistory(ctx, dst.RegionID, r.TypeID)
	if err != nil {
		return Plan{}, err
	}
	adv, days := robustVolume(hist, 30)
	now := s.now()
	fetched := sq.FetchedAt
	if dq.FetchedAt.Before(fetched) {
		fetched = dq.FetchedAt
	}
	o := Evaluate(r.TypeID, src, dst, so, do, Params{Quantity: r.Quantity, SalesTaxRate: r.SalesTaxRate, BrokerRate: r.BrokerRate, TransportCostPerM3: r.TransportCostPerM3, CargoM3: r.CargoM3, ItemVolumeM3: r.ItemVolumeM3, RouteJumps: r.RouteJumps, LowSecJumps: r.LowSecJumps, AverageDailyVolume: adv, HistoryDays: days, Now: now, FetchedAt: fetched})
	return Plan{Opportunity: o, SourceQuote: sq, DestinationQuote: dq, HistoryDays: days, AverageDailyVolume: adv}, nil
}
func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
func robustVolume(h []esi.MarketHistory, max int) (float64, int) {
	if len(h) > max {
		h = h[len(h)-max:]
	}
	v := make([]float64, 0, len(h))
	for _, d := range h {
		if d.Volume >= 0 {
			v = append(v, float64(d.Volume))
		}
	}
	if len(v) == 0 {
		return 0, 0
	}
	sort.Float64s(v)
	lo, hi := v[int(float64(len(v)-1)*.05)], v[int(float64(len(v)-1)*.95)]
	var sum float64
	for _, x := range v {
		if x < lo {
			x = lo
		}
		if x > hi {
			x = hi
		}
		sum += x
	}
	return sum / float64(len(v)), len(v)
}
