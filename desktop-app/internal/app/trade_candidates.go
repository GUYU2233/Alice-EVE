package app

import (
	"context"
	"encoding/json"
	"net/http"
)

type TradeCandidateSearch struct {
	RegionIDs            []int64 `json:"regionIds,omitempty"`
	SourceRegionIDs      []int64 `json:"sourceRegionIds,omitempty"`
	DestinationRegionIDs []int64 `json:"destinationRegionIds,omitempty"`
	DestinationScope     string  `json:"destinationScope,omitempty"`
	PerTypeLocations     int     `json:"perTypeLocations,omitempty"`
	Budget               float64 `json:"budget"`
	CargoM3              float64 `json:"cargoM3"`
	SalesTaxRate         float64 `json:"salesTaxRate"`
	BrokerRate           float64 `json:"brokerRate"`
	MinProfit            float64 `json:"minProfit"`
	MinProfitRate        float64 `json:"minProfitRate"`
	MinSecurity          float64 `json:"minSecurity"`
	MaxJumps             int     `json:"maxJumps"`
	IncludeDepth         bool    `json:"includeDepth"`
	Limit                int     `json:"limit"`
	Offset               int     `json:"offset"`
}
type TradeDepthLevel struct {
	Price            float64 `json:"price"`
	Volume           int64   `json:"volume"`
	CumulativeVolume int64   `json:"cumulativeVolume"`
	MinimumVolume    int64   `json:"minimumVolume"`
}
type AllTradeCandidate struct {
	TypeID, SourceRegionID, SourceLocationID, SourceSystemID, DestinationRegionID, DestinationLocationID, DestinationSystemID int64
	BuyPrice, SellPrice, ItemVolumeM3                                                                                         float64
	Quantity                                                                                                                  int64
	Capital, GrossProfit, Fees, NetProfit, ProfitRate, CargoUsedM3                                                            float64
	SourceSnapshotAt, DestinationSnapshotAt                                                                                   string
	RouteSafetyStatus                                                                                                         string
	AskLevels, BidLevels                                                                                                      []TradeDepthLevel
}
type TradeCandidatePage struct {
	Items         []AllTradeCandidate `json:"items"`
	Limit, Offset int
	HasMore       bool `json:"hasMore"`
}

func (r *RelayClient) SearchTradeCandidates(ctx context.Context, q TradeCandidateSearch) (TradeCandidatePage, error) {
	var out TradeCandidatePage
	body, err := json.Marshal(q)
	if err != nil {
		return out, err
	}
	err = r.doRequest(ctx, http.MethodPost, "/api/v1/trade/candidates/search", body, &out, true)
	return out, err
}
