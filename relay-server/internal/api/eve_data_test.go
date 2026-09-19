package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountCharactersCollectionRouteHasNoCharacterID(t *testing.T) {
	s := NewServer()
	acct, e := s.accountService.EnsureAccount(context.Background(), "test", "characters-collection", "Trader")
	if e != nil {
		t.Fatal(e)
	}
	session, e := s.accountService.IssueSession(context.Background(), acct.ID, "device")
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/v1/eve/characters", "/api/v1/eve/characters/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+session.AccessToken)
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}
}
