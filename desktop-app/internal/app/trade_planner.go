package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

type TradeConstraint struct {
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
}
type TradePriceLevel struct {
	Price         float64 `json:"price"`
	Volume        int64   `json:"volume"`
	MinimumVolume int64   `json:"minimumVolume,omitempty"`
}
type TradeCandidate struct {
	TypeID                                                          int64 `json:"TypeID"`
	From, To                                                        EVETradeHub
	MaxQuantity                                                     int64
	UnitVolume, UnitCost, UnitReturn, NetPerUnit, Score, Confidence float64
	AskLevels, BidLevels                                            []TradePriceLevel
	SalesTaxRate, BrokerRate                                        float64
	Jumps                                                           int
	MinSecurity                                                     float64
}
type TradePackRequest struct {
	Candidates []TradeCandidate `json:"candidates"`
	Constraint TradeConstraint  `json:"constraint"`
	Limit      int              `json:"limit,omitempty"`
}
type TradeChainRequest struct {
	Start      EVETradeHub      `json:"start"`
	Routes     []map[string]any `json:"routes"`
	Candidates []TradeCandidate `json:"candidates"`
	Snapshot   map[string]any   `json:"snapshot"`
	Constraint TradeConstraint  `json:"constraint"`
}

func (r *RelayClient) postTradePlanner(ctx context.Context, path string, input any) ([]map[string]any, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	err = r.doRequest(ctx, http.MethodPost, "/api/v1/trade/plans/"+path, body, &out, true)
	return out, err
}
func (r *RelayClient) TradeRegionCollection(ctx context.Context, regionID int64) (TradeRegionStatus, error) {
	var out TradeRegionStatus
	err := r.doRequest(ctx, http.MethodGet, "/api/v1/trade/market/regions/"+strconv.FormatInt(regionID, 10), nil, &out, true)
	return out, err
}
func (r *RelayClient) CollectTradeRegion(ctx context.Context, regionID int64) (TradeRegionStatus, error) {
	var out TradeRegionStatus
	err := r.doRequest(ctx, http.MethodPost, "/api/v1/trade/market/regions/"+strconv.FormatInt(regionID, 10), nil, &out, true)
	return out, err
}
func (r *RelayClient) ComposeTradePlan(ctx context.Context, q TradePackRequest) ([]map[string]any, error) {
	return r.postTradePlanner(ctx, "compose", q)
}
func (r *RelayClient) ChainTradePlans(ctx context.Context, q TradeChainRequest) ([]map[string]any, error) {
	return r.postTradePlanner(ctx, "chain", q)
}
