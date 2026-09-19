package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"relay-server/internal/httpapi"
	"relay-server/internal/routeplanner"
	"strconv"
)

type routeRouter interface {
	Route(context.Context, routeplanner.Query) (routeplanner.Result, error)
	Routes(context.Context, []routeplanner.Query) ([]routeplanner.Result, error)
}

type routeBatchRequest struct {
	Queries []routeplanner.Query `json:"queries"`
}

func (s *Server) tradeRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAccount(r); !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if s.routePlanner == nil {
		httpapi.WriteError(w, r, 503, "route_unavailable", "route service unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		from, e1 := strconv.ParseInt(q.Get("fromSystemId"), 10, 64)
		to, e2 := strconv.ParseInt(q.Get("toSystemId"), 10, 64)
		sec, e3 := strconv.ParseFloat(q.Get("minSecurity"), 64)
		jumps, e4 := strconv.Atoi(q.Get("maxJumps"))
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid route constraints")
			return
		}
		out, err := s.routePlanner.Route(r.Context(), routeplanner.Query{From: from, To: to, MinSecurity: sec, MaxJumps: jumps})
		if errors.Is(err, routeplanner.ErrNoRoute) {
			httpapi.WriteError(w, r, 404, "route_not_found", "no route satisfies security and jump constraints")
			return
		}
		if err != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "route calculation failed")
			return
		}
		writeRouteJSON(w, out)
	case http.MethodPost:
		defer r.Body.Close()
		var in routeBatchRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || len(in.Queries) == 0 || len(in.Queries) > 500 {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid route batch")
			return
		}
		out, err := s.routePlanner.Routes(r.Context(), in.Queries)
		if err != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "route calculation failed")
			return
		}
		writeRouteJSON(w, map[string]any{"routes": out})
	default:
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET or POST required")
	}
}
func writeRouteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
