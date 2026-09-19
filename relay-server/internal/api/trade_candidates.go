package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"relay-server/internal/httpapi"
	"relay-server/internal/marketdata"
)

type candidateSearchRequest struct {
	RegionIDs     []int64 `json:"regionIds"`
	Budget        float64 `json:"budget"`
	CargoM3       float64 `json:"cargoM3"`
	SalesTaxRate  float64 `json:"salesTaxRate"`
	BrokerRate    float64 `json:"brokerRate"`
	MinProfit     float64 `json:"minProfit"`
	MinProfitRate float64 `json:"minProfitRate"`
	MinSecurity   float64 `json:"minSecurity"`
	MaxJumps      int     `json:"maxJumps"`
	IncludeDepth  bool    `json:"includeDepth"`
	Limit         int     `json:"limit"`
	Offset        int     `json:"offset"`
}

func (s *Server) tradeCandidateSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAccount(r); !ok {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if s.candidateRepo == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "market_unavailable", "market candidate search unavailable")
		return
	}
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	var in candidateSearchRequest
	if err := dec.Decode(&in); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid candidate search input")
		return
	}
	page, err := s.candidateRepo.SearchCandidates(r.Context(), marketdata.CandidateSearch{
		RegionIDs: in.RegionIDs, Budget: in.Budget, CargoM3: in.CargoM3,
		SalesTaxRate: in.SalesTaxRate, BrokerRate: in.BrokerRate,
		MinProfit: in.MinProfit, MinProfitRate: in.MinProfitRate,
		MinSecurity: in.MinSecurity, MaxJumps: in.MaxJumps, IncludeDepth: in.IncludeDepth, Limit: in.Limit, Offset: in.Offset,
	})
	if errors.Is(err, marketdata.ErrInvalidInput) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "invalid_request", "invalid candidate search constraints")
		return
	}
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "candidate_search_failed", "candidate search failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
