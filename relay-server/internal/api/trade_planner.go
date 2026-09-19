package api

import (
	"encoding/json"
	"net/http"

	"relay-server/internal/httpapi"
	"relay-server/internal/trade"
)

type plannerRequest struct {
	Start      trade.Hub         `json:"start"`
	Routes     []trade.Route     `json:"routes"`
	Candidates []trade.Candidate `json:"candidates"`
	Snapshot   trade.Snapshot    `json:"snapshot"`
	Constraint trade.Constraint  `json:"constraint"`
}
type packRequest struct {
	Candidates []trade.Candidate `json:"candidates"`
	Constraint trade.Constraint  `json:"constraint"`
	Limit      int               `json:"limit,omitempty"`
}

func (s *Server) tradePlanner(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAccount(r); !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "POST required")
		return
	}
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	var out any
	switch r.URL.Path {
	case "/api/v1/trade/plans/compose":
		var q packRequest
		if dec.Decode(&q) != nil || len(q.Candidates) > 500 {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid planner input")
			return
		}
		v, err := trade.PackByRoute(q.Candidates, q.Constraint, q.Limit)
		if err != nil {
			httpapi.WriteError(w, r, 400, "invalid_constraints", "invalid planner constraints")
			return
		}
		out = v
	case "/api/v1/trade/plans/chain":
		var q plannerRequest
		if dec.Decode(&q) != nil || len(q.Candidates) > 2000 || len(q.Routes) > 10000 {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid planner input")
			return
		}
		v, err := trade.SearchPickupDeliveryPlans(q.Start, q.Routes, q.Candidates, q.Constraint)
		if err != nil {
			httpapi.WriteError(w, r, 400, "invalid_constraints", "invalid planner constraints")
			return
		}
		if len(v) > 50 {
			v = v[:50]
		}
		out = v
	default:
		httpapi.WriteError(w, r, 404, "not_found", "route not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
