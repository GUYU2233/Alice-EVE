package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"relay-server/internal/esidata"
	"relay-server/internal/httpapi"
)

func (s *Server) authenticatedAccount(r *http.Request) (string, bool) {
	token := bearerToken(r)
	if token == "" || s.accountService == nil {
		return "", false
	}
	session, err := s.accountService.AuthenticateAccess(r.Context(), token)
	if err != nil || session.AccountID == "" {
		return "", false
	}
	return session.AccountID, true
}

// eveCharacterData exposes only the authenticated account's normalized snapshots.
func (s *Server) eveSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET required")
		return
	}
	accountID, ok := s.authenticatedAccount(r)
	if !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	if s.esiJobs == nil {
		httpapi.WriteError(w, r, 503, "data_unavailable", "sync status unavailable")
		return
	}
	jobs, err := s.esiJobs.ListAccount(r.Context(), accountID)
	if err != nil {
		httpapi.WriteError(w, r, 500, "data_unavailable", "sync status unavailable")
		return
	}
	writeEVEJSON(w, jobs)
}

func (s *Server) eveCharacterData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET required")
		return
	}
	accountID, ok := s.authenticatedAccount(r)
	if !ok {
		httpapi.WriteError(w, r, 401, "unauthorized", "authentication required")
		return
	}
	remainder := ""
	if r.URL.Path != "/api/v1/eve/characters" && r.URL.Path != "/api/v1/eve/characters/" {
		remainder = strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/eve/characters/"), "/")
	}
	if remainder == "" {
		data, err := s.esiData.ListAccountCharacters(r.Context(), accountID)
		if err != nil {
			httpapi.WriteError(w, r, 500, "data_unavailable", "snapshot unavailable")
			return
		}
		writeEVEJSON(w, data)
		return
	}
	parts := strings.Split(remainder, "/")
	characterID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || characterID <= 0 {
		httpapi.WriteError(w, r, 400, "invalid_request", "invalid character id")
		return
	}
	if len(parts) == 1 {
		data, err := s.esiData.ListCharacterDomains(r.Context(), accountID, characterID)
		if err != nil {
			httpapi.WriteError(w, r, 500, "data_unavailable", "snapshot unavailable")
			return
		}
		writeEVEJSON(w, data)
		return
	}
	domain := parts[1]
	data, err := s.esiData.GetCharacterSnapshot(r.Context(), accountID, characterID, domain)
	if err != nil {
		status := 500
		code := "data_unavailable"
		if err == esidata.ErrNotFound {
			status = 404
			code = "not_found"
		}
		httpapi.WriteError(w, r, status, code, "snapshot unavailable")
		return
	}
	writeEVEJSON(w, data)
}
func (s *Server) evePublicData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		httpapi.WriteError(w, r, 405, "method_not_allowed", "GET or POST required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/eve/public/"), "/"), "/")
	if s.servePublicESI(w, r, parts) {
		return
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		httpapi.WriteError(w, r, 400, "invalid_request", "kind and cache key required")
		return
	}
	entry, err := s.esiData.GetPublicData(r.Context(), parts[0], parts[1])
	if err != nil {
		status := 500
		code := "data_unavailable"
		if err == esidata.ErrNotFound {
			status = 404
			code = "not_found"
		}
		httpapi.WriteError(w, r, status, code, "public data unavailable")
		return
	}
	writeEVEJSON(w, entry)
}
func writeEVEJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
