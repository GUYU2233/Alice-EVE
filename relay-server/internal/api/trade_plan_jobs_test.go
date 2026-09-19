package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"relay-server/internal/marketplan"
	"testing"
)

type planJobStub struct {
	created marketplan.CreateRequest
	job     marketplan.Job
}

func (s *planJobStub) Create(_ context.Context, a string, q marketplan.CreateRequest, h string) (marketplan.Job, error) {
	s.created = q
	s.job = marketplan.Job{ID: "job-1", AccountID: a, Mode: q.Mode, State: "queued", ConstraintHash: h}
	return s.job, nil
}
func (s *planJobStub) Get(context.Context, string, string) (marketplan.Job, error) { return s.job, nil }
func (s *planJobStub) ResultsAfter(context.Context, string, string, int64) ([]marketplan.Result, error) {
	return []marketplan.Result{}, nil
}
func (s *planJobStub) Cancel(context.Context, string, string) (marketplan.Job, error) {
	s.job.State = "cancelled"
	return s.job, nil
}
func TestTradePlanJobCreatePersistsNormalizedGlobalJob(t *testing.T) {
	s := NewServer()
	repo := &planJobStub{}
	s.marketPlanJobs = repo
	acct, e := s.accountService.EnsureAccount(context.Background(), "test", "jobs", "Trader")
	if e != nil {
		t.Fatal(e)
	}
	session, e := s.accountService.IssueSession(context.Background(), acct.ID, "device")
	if e != nil {
		t.Fatal(e)
	}
	body := `{"mode":"basket","sourceRegionIds":[10000002],"constraints":{"budget":1000,"budgetReserve":100,"cargoM3":50,"minSecurity":0.5,"maxJumps":15}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trade/plan-jobs", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/api/v1/trade/plan-jobs/job-1" {
		t.Fatalf("location=%s", rr.Header().Get("Location"))
	}
	if repo.created.DestinationScope != "all_collected_regions" || repo.created.Constraints.TargetLoadFactor != .9 {
		t.Fatalf("created=%+v", repo.created)
	}
}
