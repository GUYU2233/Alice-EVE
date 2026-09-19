package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"relay-server/internal/routeplanner"
	"testing"
)

type routeStub struct{ queries []routeplanner.Query }

func (s *routeStub) Route(_ context.Context, q routeplanner.Query) (routeplanner.Result, error) {
	s.queries = append(s.queries, q)
	return routeplanner.Result{Status: "ready", From: q.From, To: q.To, Jumps: 2}, nil
}
func (s *routeStub) Routes(_ context.Context, q []routeplanner.Query) ([]routeplanner.Result, error) {
	s.queries = append(s.queries, q...)
	out := make([]routeplanner.Result, len(q))
	for i, x := range q {
		out[i] = routeplanner.Result{Status: "ready", From: x.From, To: x.To}
	}
	return out, nil
}
func authorizedRouteServer(t *testing.T) (*Server, string) {
	s := NewServer()
	a, e := s.accountService.EnsureAccount(context.Background(), "test", "route", "Route")
	if e != nil {
		t.Fatal(e)
	}
	session, e := s.accountService.IssueSession(context.Background(), a.ID, "route-device")
	if e != nil {
		t.Fatal(e)
	}
	return s, session.AccessToken
}
func TestRouteBatchPassesQueries(t *testing.T) {
	s, token := authorizedRouteServer(t)
	stub := &routeStub{}
	s.routePlanner = stub
	body := bytes.NewBufferString(`{"queries":[{"From":1,"To":2,"MinSecurity":0.5,"MaxJumps":10},{"From":2,"To":3,"MinSecurity":0.7,"MaxJumps":5}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trade/routes", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if len(stub.queries) != 2 || stub.queries[1].MaxJumps != 5 {
		t.Fatalf("%+v", stub.queries)
	}
	var out struct {
		Routes []routeplanner.Result `json:"routes"`
	}
	if json.Unmarshal(rr.Body.Bytes(), &out) != nil || len(out.Routes) != 2 {
		t.Fatalf("%s", rr.Body.String())
	}
}
func TestRouteBatchBounded(t *testing.T) {
	s, token := authorizedRouteServer(t)
	s.routePlanner = &routeStub{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trade/routes", bytes.NewBufferString(`{"queries":[]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("%d", rr.Code)
	}
}
