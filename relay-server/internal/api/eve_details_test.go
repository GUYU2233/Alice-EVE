package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"relay-server/internal/esidata"
	"testing"
	"time"
)

func putDetailSnapshot(t *testing.T, repo esidata.Repository, account string, char int64, domain string, payload any) {
	t.Helper()
	b, _ := json.Marshal(payload)
	now := time.Now().UTC()
	if e := repo.UpsertCharacterSnapshot(context.Background(), esidata.CharacterSnapshot{AccountID: account, CharacterID: char, Domain: domain, Payload: b, FetchedAt: now, ExpiresAt: now.Add(time.Hour), Source: "test"}); e != nil {
		t.Fatal(e)
	}
}
func TestSnapshotOwnershipPredicates(t *testing.T) {
	repo := esidata.NewMemory()
	putDetailSnapshot(t, repo, "a", 42, "contracts", []map[string]any{{"contract_id": float64(9)}})
	putDetailSnapshot(t, repo, "a", 42, "mail_headers", []map[string]any{{"mail_id": float64(7)}})
	putDetailSnapshot(t, repo, "a", 42, "killmails", []map[string]any{{"killmail_id": float64(8), "killmail_hash": "hash"}})
	r := httptest.NewRequest("GET", "/", nil)
	if !snapshotContainsID(r, repo, "a", 42, "contracts", "contract_id", 9) || !snapshotContainsID(r, repo, "a", 42, "mail_headers", "mail_id", 7) || !snapshotContainsPair(r, repo, "a", 42, "killmails", 8, "hash") {
		t.Fatal("owned summaries not found")
	}
	if snapshotContainsID(r, repo, "b", 42, "contracts", "contract_id", 9) || snapshotContainsID(r, repo, "a", 99, "contracts", "contract_id", 9) || snapshotContainsPair(r, repo, "a", 42, "killmails", 8, "other") {
		t.Fatal("cross-account or mismatched summary accepted")
	}
}
func TestMailDetailHandlerRequiresAuthentication(t *testing.T) {
	s := NewServer()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/eve/details/mail/42/7", nil))
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
}
