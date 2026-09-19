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
	TargetLoadFactor     float64 `json:"targetLoadFactor"`
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
type TradeLoadItem struct {
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
type TradePackPlan struct {
	From           EVETradeHub     `json:"from"`
	To             EVETradeHub     `json:"to"`
	Items          []TradeLoadItem `json:"items"`
	Capital        float64         `json:"capital"`
	VolumeM3       float64         `json:"volumeM3"`
	CargoCapacity  float64         `json:"cargoCapacity"`
	LoadFactor     float64         `json:"loadFactor"`
	NetProfit      float64         `json:"netProfit"`
	Jumps          int             `json:"jumps"`
	MinSecurity    float64         `json:"minSecurity"`
	UnfilledReason string          `json:"unfilledReason,omitempty"`
}
type TradeCargoLot struct {
	Item TradeLoadItem `json:"item"`
	From EVETradeHub   `json:"from"`
	To   EVETradeHub   `json:"to"`
}
type TradeChainStop struct {
	Hub           EVETradeHub     `json:"hub"`
	Loads         []TradeCargoLot `json:"loads,omitempty"`
	Unloads       []TradeCargoLot `json:"unloads,omitempty"`
	CashBefore    float64         `json:"cashBefore"`
	SaleRevenue   float64         `json:"saleRevenue"`
	PurchaseCost  float64         `json:"purchaseCost"`
	CashAfter     float64         `json:"cashAfter"`
	CargoBeforeM3 float64         `json:"cargoBeforeM3"`
	UnloadedM3    float64         `json:"unloadedM3"`
	LoadedM3      float64         `json:"loadedM3"`
	CargoAfterM3  float64         `json:"cargoAfterM3"`
}
type TradePickupDeliveryPlan struct {
	Stops          []TradeChainStop `json:"stops"`
	Items          []TradeLoadItem  `json:"items,omitempty"`
	Inventory      []TradeCargoLot  `json:"inventory,omitempty"`
	RealizedProfit float64          `json:"realizedProfit"`
	Capital        float64          `json:"capital"`
	VolumeM3       float64          `json:"volumeM3"`
	TotalJumps     int              `json:"totalJumps"`
	MinSecurity    float64          `json:"minSecurity"`
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
func (r *RelayClient) ComposeTradePlan(ctx context.Context, q TradePackRequest) ([]TradePackPlan, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	var out []TradePackPlan
	err = r.doRequest(ctx, http.MethodPost, "/api/v1/trade/plans/compose", body, &out, true)
	return out, err
}
func (r *RelayClient) ChainTradePlans(ctx context.Context, q TradeChainRequest) ([]TradePickupDeliveryPlan, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	var out []TradePickupDeliveryPlan
	err = r.doRequest(ctx, http.MethodPost, "/api/v1/trade/plans/chain", body, &out, true)
	return out, err
}
