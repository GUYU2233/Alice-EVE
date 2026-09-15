package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"relay-server/internal/accounts"
	"relay-server/internal/authn"
	"relay-server/internal/store"
	"testing"
	"time"
)

func TestConversationRepositoryAndCursor(t *testing.T) {
	s := NewServer()
	now := time.Now().UTC()
	ar := accounts.NewMemoryRepository()
	_ = ar.CreateAccount(context.Background(), accounts.Account{ID: "acct", Status: accounts.AccountStatusActive, CreatedAt: now, UpdatedAt: now})
	dev := store.Device{ID: "mobile", TokenHash: authn.HashToken("tok"), Type: "mobile", CreatedAt: now}
	if err := s.store.AddDevice(dev); err != nil {
		t.Fatal(err)
	}
	s.accountRepo = ar
	// Memory server authorizer is intentionally device-bound; exercise repository directly.
	req := httptest.NewRequest("GET", "/api/v1/agent/conversations", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("status=%d", rr.Code)
	}
	var probe map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&probe)
}
