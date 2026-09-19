package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"relay-server/internal/httpapi"
	"relay-server/internal/marketplan"
	"strconv"
	"strings"
)

func (s *Server) tradePlanJobs(w http.ResponseWriter, r *http.Request) {
	account, ok := s.authenticatedAccount(r)
	if !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if s.marketPlanJobs == nil {
		httpapi.WriteError(w, r, 503, "planner_unavailable", "persistent planning unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/trade/plan-jobs")
	if path == "" || path == "/" {
		if r.Method != http.MethodPost {
			httpapi.WriteError(w, r, 405, "method_not_allowed", "POST required")
			return
		}
		defer r.Body.Close()
		var q marketplan.CreateRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&q) != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid planning job")
			return
		}
		hash, e := marketplan.Normalize(&q)
		if e != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid planning constraints")
			return
		}
		job, e := s.marketPlanJobs.Create(r.Context(), account, q, hash)
		if e != nil {
			// Log the internal PostgreSQL/serialization failure with request ID in the
			// server journal; the public response remains deliberately non-sensitive.
			fmt.Printf("market plan job create failed account=%s mode=%s: %v\n", account, q.Mode, e)
			httpapi.WriteError(w, r, 500, "job_create_failed", "could not create planning job")
			return
		}
		w.Header().Set("Location", "/api/v1/trade/plan-jobs/"+job.ID)
		w.WriteHeader(http.StatusAccepted)
		writePlanJSON(w, job)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		job, e := s.marketPlanJobs.Get(r.Context(), account, id)
		if errors.Is(e, marketplan.ErrNotFound) {
			httpapi.WriteError(w, r, 404, "not_found", "planning job not found")
			return
		}
		if e != nil {
			httpapi.WriteError(w, r, 500, "job_read_failed", "could not read planning job")
			return
		}
		writePlanJSON(w, job)
		return
	}
	if len(parts) == 2 && parts[1] == "results" && r.Method == http.MethodGet {
		after, _ := strconv.ParseInt(r.URL.Query().Get("afterRevision"), 10, 64)
		out, e := s.marketPlanJobs.ResultsAfter(r.Context(), account, id, after)
		if e != nil {
			httpapi.WriteError(w, r, 500, "result_read_failed", "could not read planning results")
			return
		}
		writePlanJSON(w, map[string]any{"items": out})
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		job, e := s.marketPlanJobs.Cancel(r.Context(), account, id)
		if errors.Is(e, marketplan.ErrNotFound) {
			httpapi.WriteError(w, r, 404, "not_found", "planning job not found")
			return
		}
		if e != nil {
			httpapi.WriteError(w, r, 500, "job_cancel_failed", "could not cancel planning job")
			return
		}
		writePlanJSON(w, job)
		return
	}
	httpapi.WriteError(w, r, 404, "not_found", "route not found")
}
func writePlanJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
