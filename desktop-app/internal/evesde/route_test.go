package evesde

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func routeRepo(t *testing.T) *Repository {
	t.Helper()
	data := `{"kind":"region","id":1,"name":"R"}
{"kind":"constellation","id":2,"region_id":1,"name":"C"}
{"kind":"system","id":10,"constellation_id":2,"name":"A","security":0.9}
{"kind":"system","id":20,"constellation_id":2,"name":"B","security":0.2}
{"kind":"system","id":30,"constellation_id":2,"name":"C","security":0.8}
{"kind":"system","id":40,"constellation_id":2,"name":"D","security":0.7}
{"kind":"system","id":50,"constellation_id":2,"name":"E","security":-0.1}
{"kind":"stargate","id":1,"system_id":10,"destination_system_id":20}
{"kind":"stargate","id":2,"system_id":20,"destination_system_id":40}
{"kind":"stargate","id":3,"system_id":10,"destination_system_id":30}
{"kind":"stargate","id":4,"system_id":30,"destination_system_id":50}
{"kind":"stargate","id":5,"system_id":50,"destination_system_id":40}
{"kind":"stargate","id":6,"system_id":30,"destination_system_id":40}
`
	p := filepath.Join(t.TempDir(), "sde.sqlite")
	if err := Import(context.Background(), p, strings.NewReader(data), Manifest{Version: "route", Source: "test"}, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func ids(r RouteResult) []int64 {
	out := make([]int64, len(r.Systems))
	for i, s := range r.Systems {
		out[i] = s.ID
	}
	return out
}
func TestRouteModesConstraintsAndMetrics(t *testing.T) {
	r := routeRepo(t)
	ctx := context.Background()
	shortest, err := r.Route(ctx, 10, 40, RouteOptions{Mode: RouteShortest})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(shortest); len(got) != 3 || got[1] != 20 || shortest.Jumps != 2 || shortest.LowSecCount != 1 || shortest.NullSecCount != 0 || shortest.MinSecurity != 0.2 {
		t.Fatalf("shortest=%+v ids=%v", shortest, got)
	}
	safest, err := r.Route(ctx, 10, 40, RouteOptions{Mode: RouteSafest})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(safest); len(got) != 3 || got[1] != 30 || safest.LowSecCount != 0 {
		t.Fatalf("safest=%+v ids=%v", safest, got)
	}
	floor := 0.75
	if _, err = r.Route(ctx, 10, 40, RouteOptions{MinSecurity: &floor}); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("floor error=%v", err)
	}
}
func TestRouteSameSystem(t *testing.T) {
	r := routeRepo(t)
	got, err := r.Route(context.Background(), 10, 10, RouteOptions{})
	if err != nil || got.Jumps != 0 || got.MinSecurity != 0.9 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
