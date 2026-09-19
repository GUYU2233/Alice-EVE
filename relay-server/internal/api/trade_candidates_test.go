package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"relay-server/internal/marketdata"
)

type candidateRepoStub struct {
	query marketdata.CandidateSearch
}

func (f *candidateRepoStub) SearchCandidates(_ context.Context, q marketdata.CandidateSearch) (marketdata.CandidatePage, error) {
	f.query = q
	return marketdata.CandidatePage{Items: []marketdata.TradeCandidate{{TypeID: 34, Quantity: 10, RouteSafetyStatus: "pending"}}, Limit: q.Limit}, nil
}

func TestCandidateSearchRequiresAuthentication(t *testing.T) {
	s := NewServer()
	s.candidateRepo = &candidateRepoStub{}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/trade/candidates/search", bytes.NewBufferString(`{}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCandidateSearchDoesNotRequireTypeOrQuantity(t *testing.T) {
	s := NewServer()
	repo := &candidateRepoStub{}
	s.candidateRepo = repo
	account, err := s.accountService.EnsureAccount(context.Background(), "test", "trade-candidates", "Trader")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.accountService.IssueSession(context.Background(), account.ID, "trade-device")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trade/candidates/search", bytes.NewBufferString(`{"regionIds":[10000002,10000043],"budget":1000000,"cargoM3":100}`))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if repo.query.Budget != 1000000 || repo.query.CargoM3 != 100 || len(repo.query.RegionIDs) != 2 {
		t.Fatalf("query=%+v", repo.query)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"quantity":10`)) || !bytes.Contains(rr.Body.Bytes(), []byte(`"routeSafetyStatus":"pending"`)) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}
