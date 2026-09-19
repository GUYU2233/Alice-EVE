package marketdata

import (
	"context"
	"errors"
	"math"
	"time"
)

const (
	DefaultCandidateLimit = 100
	MaxCandidateLimit     = 500
	MaxCandidateOffset    = 10000
	MaxCandidateRegions   = 64
)

type CandidateSearch struct {
	RegionIDs     []int64
	Budget        float64
	CargoM3       float64
	SalesTaxRate  float64
	BrokerRate    float64
	MinProfit     float64
	MinProfitRate float64
	MinSecurity   float64
	MaxJumps      int
	IncludeDepth  bool
	Limit         int
	Offset        int
}

type TradeCandidate struct {
	TypeID                int64        `json:"typeId"`
	SourceRegionID        int64        `json:"sourceRegionId"`
	SourceLocationID      int64        `json:"sourceLocationId"`
	SourceSystemID        int64        `json:"sourceSystemId"`
	DestinationRegionID   int64        `json:"destinationRegionId"`
	DestinationLocationID int64        `json:"destinationLocationId"`
	DestinationSystemID   int64        `json:"destinationSystemId"`
	BuyPrice              float64      `json:"buyPrice"`
	SellPrice             float64      `json:"sellPrice"`
	ItemVolumeM3          float64      `json:"itemVolumeM3"`
	Quantity              int64        `json:"quantity"`
	Capital               float64      `json:"capital"`
	GrossProfit           float64      `json:"grossProfit"`
	Fees                  float64      `json:"fees"`
	NetProfit             float64      `json:"netProfit"`
	ProfitRate            float64      `json:"profitRate"`
	CargoUsedM3           float64      `json:"cargoUsedM3"`
	SourceSnapshotAt      time.Time    `json:"sourceSnapshotAt"`
	DestinationSnapshotAt time.Time    `json:"destinationSnapshotAt"`
	RouteSafetyStatus     string       `json:"routeSafetyStatus"`
	AskLevels             []DepthLevel `json:"askLevels,omitempty"`
	BidLevels             []DepthLevel `json:"bidLevels,omitempty"`
}

type DepthLevel struct {
	Price            float64 `json:"price"`
	Volume           int64   `json:"volume"`
	CumulativeVolume int64   `json:"cumulativeVolume"`
	MinimumVolume    int64   `json:"minimumVolume"`
}

type CandidatePage struct {
	Items   []TradeCandidate `json:"items"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	HasMore bool             `json:"hasMore"`
}

type CandidateRepository interface {
	SearchCandidates(context.Context, CandidateSearch) (CandidatePage, error)
}

func (q *CandidateSearch) normalize() error {
	if len(q.RegionIDs) == 0 || len(q.RegionIDs) > MaxCandidateRegions || q.Budget <= 0 || q.CargoM3 <= 0 ||
		math.IsNaN(q.Budget) || math.IsInf(q.Budget, 0) || math.IsNaN(q.CargoM3) || math.IsInf(q.CargoM3, 0) ||
		q.SalesTaxRate < 0 || q.SalesTaxRate >= 1 || q.BrokerRate < 0 || q.BrokerRate >= 1 ||
		q.MinProfit < 0 || q.MinProfitRate < 0 || q.MinSecurity < -1 || q.MinSecurity > 1 || q.MaxJumps < 0 || q.MaxJumps > 256 || q.Offset < 0 || q.Offset > MaxCandidateOffset {
		return ErrInvalidInput
	}
	seen := make(map[int64]struct{}, len(q.RegionIDs))
	for _, id := range q.RegionIDs {
		if !validRegion(id) {
			return ErrInvalidInput
		}
		if _, ok := seen[id]; ok {
			return ErrInvalidInput
		}
		seen[id] = struct{}{}
	}
	if q.Limit == 0 {
		q.Limit = DefaultCandidateLimit
	}
	if q.Limit < 1 || q.Limit > MaxCandidateLimit {
		return ErrInvalidInput
	}
	return nil
}

var errCandidateQuery = errors.New("candidate query failed")
