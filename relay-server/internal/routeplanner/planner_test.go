package routeplanner

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

type fakeLoader struct {
	versions, loads int
	g               Graph
}

func (f *fakeLoader) ActiveVersion(context.Context) (int64, error) {
	f.versions++
	return f.g.Version, nil
}
func (f *fakeLoader) LoadGraph(context.Context, int64) (Graph, error) { f.loads++; return f.g, nil }
func TestPlannerSecurityBoundsAndCache(t *testing.T) {
	l := &fakeLoader{g: Graph{Version: 1, Systems: map[int64]System{1: {1, "A", 1}, 2: {2, "B", .4}, 3: {3, "C", .8}, 4: {4, "D", .7}}, Neighbors: map[int64][]int64{1: {2, 3}, 2: {4}, 3: {4}}}}
	p := New(l, time.Hour)
	r, e := p.Route(context.Background(), Query{1, 4, .5, 4})
	if e != nil || r.Jumps != 2 || r.Systems[1].ID != 3 || r.MinSecurity != .7 {
		t.Fatalf("route=%+v err=%v", r, e)
	}
	r.Systems[0].Name = "mutated"
	again, _ := p.Route(context.Background(), Query{1, 4, .5, 4})
	if again.Systems[0].Name == "mutated" {
		t.Fatal("cached result mutated")
	}
	if l.loads != 1 {
		t.Fatalf("graph loads=%d", l.loads)
	}
	if _, e = p.Route(context.Background(), Query{1, 2, .5, 4}); !errors.Is(e, ErrNoRoute) {
		t.Fatalf("unsafe route: %v", e)
	}
}
func TestBatchReturnsUnavailable(t *testing.T) {
	l := &fakeLoader{g: Graph{Version: 1, Systems: map[int64]System{1: {1, "A", 1}, 2: {2, "B", 1}}, Neighbors: map[int64][]int64{}}}
	r, e := New(l, time.Minute).Routes(context.Background(), []Query{{1, 2, .5, 2}})
	if e != nil || len(r) != 1 || r[0].Status != "unavailable" {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestRejectsNaNAndInvalidatesOnVersion(t *testing.T) {
	l := &fakeLoader{g: Graph{Version: 1, Systems: map[int64]System{1: {1, "A", 1}}, Neighbors: map[int64][]int64{}}}
	p := New(l, time.Hour)
	if _, e := p.Route(context.Background(), Query{1, 1, math.NaN(), 1}); e == nil {
		t.Fatal("NaN accepted")
	}
	_, _ = p.Route(context.Background(), Query{1, 1, 0, 1})
	l.g.Version = 2
	_, _ = p.Route(context.Background(), Query{1, 1, 0, 1})
	if l.loads != 2 {
		t.Fatalf("version did not invalidate: %d", l.loads)
	}
}
