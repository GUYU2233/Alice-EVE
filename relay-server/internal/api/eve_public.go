package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"relay-server/internal/esipublic"
	"relay-server/internal/httpapi"
	"relay-server/internal/trade"
)

func (s *Server) servePublicESI(w http.ResponseWriter, r *http.Request, parts []string) bool {
	if s.esiPublic == nil {
		return false
	}
	var result esipublic.Result
	var err error
	parseID := func(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) }
	switch {
	case len(parts) == 2 && parts[0] == "market" && parts[1] == "plans":
		if _, ok := s.authenticatedAccount(r); !ok {
			httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
			return true
		}
		if r.Method != http.MethodPost {
			httpapi.WriteError(w, r, 405, "method_not_allowed", "POST required")
			return true
		}
		defer r.Body.Close()
		var req trade.PlanRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&req) != nil || req.TypeID <= 0 || req.Quantity <= 0 || req.Quantity > 1000000000 || req.SalesTaxRate < 0 || req.SalesTaxRate > .2 || req.BrokerRate < 0 || req.BrokerRate > .2 || req.CargoM3 < 0 || req.ItemVolumeM3 <= 0 || req.RouteJumps < 0 || req.LowSecJumps < 0 {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid trade parameters")
			return true
		}
		svc := &trade.Service{ESI: s.esiPublic.Gateway()}
		plans, pErr := svc.BestPlans(r.Context(), req)
		if pErr != nil {
			httpapi.WriteError(w, r, 502, "esi_unavailable", "trade plans unavailable")
			return true
		}
		if len(plans) > 20 {
			plans = plans[:20]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(plans)
		return true
	case len(parts) == 4 && parts[0] == "market" && parts[1] == "types" && parts[3] == "comparison":
		typeID, parseErr := parseID(parts[2])
		if parseErr != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid type id")
			return true
		}
		svc := &trade.Service{ESI: s.esiPublic.Gateway()}
		quotes, qErr := svc.Compare(r.Context(), typeID)
		if qErr != nil {
			httpapi.WriteError(w, r, 502, "esi_unavailable", "comparison unavailable")
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(quotes)
		return true
	case len(parts) == 5 && parts[0] == "market" && parts[1] == "hubs" && parts[3] == "types":
		if r.Method != http.MethodGet {
			httpapi.WriteError(w, r, 405, "method_not_allowed", "GET required")
			return true
		}
		typeID, parseErr := parseID(parts[4])
		if parseErr != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid type id")
			return true
		}
		svc := &trade.Service{ESI: s.esiPublic.Gateway()}
		quote, _, quoteErr := svc.Quote(r.Context(), parts[2], typeID)
		if quoteErr != nil {
			httpapi.WriteError(w, r, 502, "esi_unavailable", "hub quote unavailable")
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(quote)
		return true
	case len(parts) == 2 && parts[0] == "market" && parts[1] == "hubs":
		if r.Method != http.MethodGet {
			httpapi.WriteError(w, r, 405, "method_not_allowed", "GET required")
			return true
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(trade.Hubs)
		return true
	case len(parts) == 2 && parts[0] == "universe" && parts[1] == "names":
		if r.Method != http.MethodPost {
			httpapi.WriteError(w, r, 405, "method_not_allowed", "POST required")
			return true
		}
		defer r.Body.Close()
		var ids []int64
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&ids); e != nil {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid ID list")
			return true
		}
		result, err = s.esiPublic.UniverseNames(r.Context(), ids)
	case len(parts) == 1 && parts[0] == "status":
		result, err = s.esiPublic.Status(r.Context())
	case len(parts) == 2 && parts[0] == "market" && parts[1] == "prices":
		result, err = s.esiPublic.MarketPrices(r.Context())
	case len(parts) == 3 && parts[0] == "universe" && parts[1] == "types":
		var id int64
		id, err = parseID(parts[2])
		if err == nil {
			result, err = s.esiPublic.UniverseType(r.Context(), id)
		}
	case len(parts) == 2 && parts[0] == "corporations":
		var id int64
		id, err = parseID(parts[1])
		if err == nil {
			result, err = s.esiPublic.Corporation(r.Context(), id)
		}
	case len(parts) == 2 && parts[0] == "alliances":
		var id int64
		id, err = parseID(parts[1])
		if err == nil {
			result, err = s.esiPublic.Alliance(r.Context(), id)
		}
	case len(parts) == 3 && parts[0] == "universe" && parts[1] == "systems":
		var id int64
		id, err = parseID(parts[2])
		if err == nil {
			result, err = s.esiPublic.UniverseSystem(r.Context(), id)
		}
	case len(parts) == 3 && parts[0] == "universe" && parts[1] == "stations":
		var id int64
		id, err = parseID(parts[2])
		if err == nil {
			result, err = s.esiPublic.UniverseStation(r.Context(), id)
		}
	case len(parts) == 3 && parts[0] == "markets" && parts[2] == "orders":
		var regionID, typeID int64
		regionID, err = parseID(parts[1])
		if err == nil && r.URL.Query().Get("type_id") != "" {
			typeID, err = parseID(r.URL.Query().Get("type_id"))
		}
		if err == nil {
			result, err = s.esiPublic.RegionOrders(r.Context(), regionID, r.URL.Query().Get("order_type"), typeID)
		}
	case len(parts) == 3 && parts[0] == "markets" && parts[2] == "history":
		var regionID, typeID int64
		regionID, err = parseID(parts[1])
		if err == nil {
			typeID, err = parseID(r.URL.Query().Get("type_id"))
		}
		if err == nil {
			result, err = s.esiPublic.RegionHistory(r.Context(), regionID, typeID)
		}
	default:
		return false
	}
	if err != nil {
		status, code := http.StatusBadGateway, "esi_unavailable"
		if errors.Is(err, strconv.ErrSyntax) || strings.Contains(err.Error(), "out of range") || strings.Contains(err.Error(), "invalid order") {
			status, code = http.StatusBadRequest, "invalid_request"
		}
		httpapi.WriteError(w, r, status, code, "public ESI data unavailable")
		return true
	}
	if result.Stale {
		w.Header().Set("Warning", `110 - "Response is stale"`)
		w.Header().Set("X-ESI-Cache-Stale", "true")
	}
	w.Header().Set("ETag", result.Entry.ETag)
	w.Header().Set("Expires", result.Entry.ExpiresAt.UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(result.Entry.Payload)
	return true
}
