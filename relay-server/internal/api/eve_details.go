package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"relay-server/internal/esidata"
	"relay-server/internal/httpapi"
)

// eveOnDemandDetail proxies sensitive detail only after proving the requested
// summary belongs to the authenticated account. Mail bodies are never persisted.
func (s *Server) eveOnDemandDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET required")
		return
	}
	accountID, ok := s.authenticatedAccount(r)
	if !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if s.esiWorker == nil {
		httpapi.WriteError(w, r, 503, "data_unavailable", "detail unavailable")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/eve/details/"), "/"), "/")
	if len(parts) < 3 {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid detail reference")
		return
	}
	characterID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || characterID <= 0 {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid character id")
		return
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id <= 0 {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid detail id")
		return
	}
	var result any
	switch parts[0] {
	case "contracts":
		if !snapshotContainsID(r, s.esiData, accountID, characterID, "contracts", "contract_id", id) {
			httpapi.WriteError(w, r, 404, "not_found", "detail unavailable")
			return
		}
		if len(parts) != 4 {
			httpapi.WriteError(w, r, 400, "invalid_request", "contract detail kind required")
			return
		}
		if parts[3] == "items" {
			result, err = s.esiWorker.ContractItems(r.Context(), accountID, id)
		} else if parts[3] == "bids" {
			result, err = s.esiWorker.ContractBids(r.Context(), accountID, id)
		} else {
			httpapi.WriteError(w, r, 400, "invalid_request", "invalid contract detail kind")
			return
		}
	case "mail":
		if len(parts) != 3 || !snapshotContainsID(r, s.esiData, accountID, characterID, "mail_headers", "mail_id", id) {
			httpapi.WriteError(w, r, 404, "not_found", "detail unavailable")
			return
		}
		result, err = s.esiWorker.MailBody(r.Context(), accountID, id)
		w.Header().Set("Cache-Control", "no-store")
	case "killmails":
		if len(parts) != 4 {
			httpapi.WriteError(w, r, 400, "invalid_request", "killmail hash required")
			return
		}
		hash := parts[3]
		if !snapshotContainsPair(r, s.esiData, accountID, characterID, "killmails", id, hash) {
			httpapi.WriteError(w, r, 404, "not_found", "detail unavailable")
			return
		}
		result, err = s.esiWorker.KillmailDetail(r.Context(), id, hash)
	default:
		httpapi.WriteError(w, r, 404, "not_found", "detail unavailable")
		return
	}
	if err != nil {
		httpapi.WriteError(w, r, 502, "esi_unavailable", "detail unavailable")
		return
	}
	writeEVEJSON(w, result)
}

func snapshotArray(r *http.Request, repo esidata.Repository, account string, character int64, domain string) []map[string]any {
	s, err := repo.GetCharacterSnapshot(r.Context(), account, character, domain)
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if json.Unmarshal(s.Payload, &rows) != nil {
		return nil
	}
	return rows
}
func snapshotContainsID(r *http.Request, repo esidata.Repository, account string, character int64, domain, key string, id int64) bool {
	for _, row := range snapshotArray(r, repo, account, character, domain) {
		if n, ok := row[key].(float64); ok && int64(n) == id {
			return true
		}
	}
	return false
}
func snapshotContainsPair(r *http.Request, repo esidata.Repository, account string, character int64, domain string, id int64, hash string) bool {
	for _, row := range snapshotArray(r, repo, account, character, domain) {
		n, nok := row["killmail_id"].(float64)
		h, hok := row["killmail_hash"].(string)
		if nok && hok && int64(n) == id && h == hash {
			return true
		}
	}
	return false
}
