package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

type MarketPlanConstraints struct {
	Budget           float64 `json:"budget"`
	BudgetReserve    float64 `json:"budgetReserve"`
	CargoM3          float64 `json:"cargoM3"`
	MinSecurity      float64 `json:"minSecurity"`
	MaxJumps         int     `json:"maxJumps"`
	TargetLoadFactor float64 `json:"targetLoadFactor"`
}
type MarketPlanJobRequest struct {
	CharacterID          int64                 `json:"characterId"`
	Mode                 string                `json:"mode"`
	SourceRegionIDs      []int64               `json:"sourceRegionIds"`
	DestinationScope     string                `json:"destinationScope"`
	DestinationRegionIDs []int64               `json:"destinationRegionIds,omitempty"`
	Constraints          MarketPlanConstraints `json:"constraints"`
}
type MarketPlanJob struct {
	ID                   string                `json:"id"`
	Mode                 string                `json:"mode"`
	State                string                `json:"state"`
	SourceRegionIDs      []int64               `json:"sourceRegionIds"`
	DestinationScope     string                `json:"destinationScope"`
	DestinationRegionIDs []int64               `json:"destinationRegionIds"`
	Constraints          MarketPlanConstraints `json:"constraints"`
	Progress             json.RawMessage       `json:"progress"`
	Iteration            int64                 `json:"iteration"`
	ResultRevision       int64                 `json:"resultRevision"`
	LastError            string                `json:"lastError"`
	CreatedAt            string                `json:"createdAt"`
	UpdatedAt            string                `json:"updatedAt"`
}
type MarketPlanResult struct {
	Revision  int64          `json:"revision"`
	Rank      int            `json:"rank"`
	StableKey string         `json:"stableKey"`
	Score     float64        `json:"score"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"createdAt"`
}
type MarketPlanResultPage struct {
	Items []MarketPlanResult `json:"items"`
}

func (r *RelayClient) CreateMarketPlanJob(ctx context.Context, q MarketPlanJobRequest) (MarketPlanJob, error) {
	var out MarketPlanJob
	b, e := json.Marshal(q)
	if e != nil {
		return out, e
	}
	e = r.doRequest(ctx, http.MethodPost, "/api/v1/trade/plan-jobs", b, &out, true)
	return out, e
}
func (r *RelayClient) MarketPlanJob(ctx context.Context, id string) (MarketPlanJob, error) {
	var out MarketPlanJob
	e := r.doRequest(ctx, http.MethodGet, "/api/v1/trade/plan-jobs/"+url.PathEscape(id), nil, &out, true)
	return out, e
}
func (r *RelayClient) MarketPlanResults(ctx context.Context, id string, after int64) (MarketPlanResultPage, error) {
	var out MarketPlanResultPage
	e := r.doRequest(ctx, http.MethodGet, "/api/v1/trade/plan-jobs/"+url.PathEscape(id)+"/results?afterRevision="+strconv.FormatInt(after, 10), nil, &out, true)
	return out, e
}
func (r *RelayClient) CancelMarketPlanJob(ctx context.Context, id string) (MarketPlanJob, error) {
	var out MarketPlanJob
	e := r.doRequest(ctx, http.MethodPost, "/api/v1/trade/plan-jobs/"+url.PathEscape(id)+"/cancel", nil, &out, true)
	return out, e
}
