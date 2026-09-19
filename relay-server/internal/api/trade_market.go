package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"relay-server/internal/httpapi"
	"relay-server/internal/marketdata"
)

type marketRegionResponse struct {
	RegionID  int64                      `json:"regionId"`
	State     string                     `json:"state,omitempty"`
	StartedAt time.Time                  `json:"startedAt,omitempty"`
	EndedAt   *time.Time                 `json:"endedAt,omitempty"`
	Snapshot  any                        `json:"snapshot,omitempty"`
	Error     string                     `json:"error,omitempty"`
	Schedule  *marketdata.RegionSchedule `json:"schedule,omitempty"`
	Job       *marketdata.CollectionJob  `json:"job,omitempty"`
}

// tradeMarket exposes durable priority refresh and region status. Collection is
// performed only by the scheduler worker, so request cancellation cannot own it.
func (s *Server) tradeMarket(w http.ResponseWriter, r *http.Request) {
	accountID, ok := s.authenticatedAccount(r)
	if !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if s.marketRepo == nil {
		httpapi.WriteError(w, r, 503, "market_unavailable", "market scheduler unavailable")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/trade/market/"), "/"), "/")
	if len(parts) != 2 || parts[0] != "regions" {
		httpapi.WriteError(w, r, 404, "not_found", "route not found")
		return
	}
	regionID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || regionID <= 0 {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid region id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := s.marketRepo.RegionStatus(r.Context(), regionID)
		if errors.Is(err, marketdata.ErrNotFound) {
			httpapi.WriteError(w, r, 404, "collection_not_found", "no persistent market state for region")
			return
		}
		if err != nil {
			httpapi.WriteError(w, r, 500, "market_status_failed", "failed to read market status")
			return
		}
		writeMarketStatus(w, status, http.StatusOK)
	case http.MethodPost:
		job, err := s.marketRepo.Enqueue(r.Context(), regionID, 1000000, marketdata.TriggerUserPriorityRefresh, accountID, time.Now().UTC())
		if err != nil {
			httpapi.WriteError(w, r, 500, "market_enqueue_failed", "failed to enqueue market refresh")
			return
		}
		status, statusErr := s.marketRepo.RegionStatus(r.Context(), regionID)
		if statusErr != nil && !errors.Is(statusErr, marketdata.ErrNotFound) {
			httpapi.WriteError(w, r, 500, "market_status_failed", "failed to read market status")
			return
		}
		status.Job = &job
		writeMarketStatus(w, status, http.StatusAccepted)
	default:
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET or POST required")
	}
}

func writeMarketStatus(w http.ResponseWriter, status marketdata.RegionStatus, code int) {
	response := marketRegionResponse{RegionID: status.Schedule.RegionID, Schedule: &status.Schedule, Job: status.Job}
	if response.RegionID == 0 && status.Job != nil {
		response.RegionID = status.Job.RegionID
	}
	if status.Job != nil {
		response.State = string(status.Job.State)
		response.StartedAt = status.Job.StartedAt
		if !status.Job.CompletedAt.IsZero() {
			ended := status.Job.CompletedAt
			response.EndedAt = &ended
		}
		response.Error = status.Job.FailureDetail
	}
	if status.Snapshot != nil {
		response.Snapshot = status.Snapshot
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(response)
}
