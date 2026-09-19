// Package routeplanner provides reusable bounded path planning over immutable SDE graphs.
package routeplanner

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

var ErrNoRoute = errors.New("route not found")

const MaxJumps = 256

type System struct {
	ID       int64   `json:"systemId"`
	Name     string  `json:"name"`
	Security float64 `json:"securityStatus"`
}
type Graph struct {
	Version   int64
	Systems   map[int64]System
	Neighbors map[int64][]int64
}
type Loader interface {
	ActiveVersion(context.Context) (int64, error)
	LoadGraph(context.Context, int64) (Graph, error)
}
type Query struct {
	From        int64   `json:"From"`
	To          int64   `json:"To"`
	MinSecurity float64 `json:"MinSecurity"`
	MaxJumps    int     `json:"MaxJumps"`
}
type Result struct {
	Status      string   `json:"status"`
	From        int64    `json:"fromSystemId"`
	To          int64    `json:"toSystemId"`
	Version     int64    `json:"version"`
	Jumps       int      `json:"jumps"`
	MinSecurity float64  `json:"minSecurity"`
	Systems     []System `json:"systems"`
}
type cacheEntry struct {
	result  Result
	err     error
	expires time.Time
}
type Planner struct {
	loader     Loader
	ttl        time.Duration
	maxEntries int
	mu         sync.RWMutex
	loadMu     sync.Mutex
	graph      Graph
	cache      map[string]cacheEntry
}

func New(loader Loader, ttl time.Duration) *Planner {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Planner{loader: loader, ttl: ttl, maxEntries: 4096, cache: map[string]cacheEntry{}}
}
func (q Query) normalize() (Query, error) {
	if q.From <= 0 || q.To <= 0 || math.IsNaN(q.MinSecurity) || math.IsInf(q.MinSecurity, 0) || q.MinSecurity < -1 || q.MinSecurity > 1 || q.MaxJumps < 0 || q.MaxJumps > MaxJumps {
		return q, errors.New("invalid route query")
	}
	if q.MinSecurity == 0 {
		q.MinSecurity = 0
	}
	if q.MaxJumps == 0 {
		q.MaxJumps = MaxJumps
	}
	return q, nil
}
func (p *Planner) Route(ctx context.Context, q Query) (Result, error) {
	q, err := q.normalize()
	if err != nil {
		return Result{}, err
	}
	g, err := p.activeGraph(ctx)
	if err != nil {
		return Result{}, err
	}
	key := fmt.Sprintf("%d/%d/%d/%016x/%d", g.Version, q.From, q.To, math.Float64bits(q.MinSecurity), q.MaxJumps)
	now := time.Now()
	p.mu.RLock()
	e, ok := p.cache[key]
	p.mu.RUnlock()
	if ok && now.Before(e.expires) {
		return cloneResult(e.result), e.err
	}
	r, err := search(ctx, g, q)
	p.mu.Lock()
	if len(p.cache) >= p.maxEntries {
		p.evictLocked(now)
	}
	p.cache[key] = cacheEntry{cloneResult(r), err, now.Add(p.ttl)}
	p.mu.Unlock()
	return r, err
}
func (p *Planner) Routes(ctx context.Context, qs []Query) ([]Result, error) {
	if len(qs) == 0 || len(qs) > 500 {
		return nil, errors.New("invalid route batch")
	}
	out := make([]Result, len(qs))
	for i, q := range qs {
		r, e := p.Route(ctx, q)
		if e != nil && !errors.Is(e, ErrNoRoute) {
			return nil, e
		}
		if errors.Is(e, ErrNoRoute) {
			r = Result{Status: "unavailable", From: q.From, To: q.To}
		}
		out[i] = r
	}
	return out, nil
}
func (p *Planner) activeGraph(ctx context.Context) (Graph, error) {
	version, err := p.loader.ActiveVersion(ctx)
	if err != nil {
		return Graph{}, err
	}
	p.mu.RLock()
	g := p.graph
	p.mu.RUnlock()
	if g.Version == version && version != 0 {
		return g, nil
	}
	p.loadMu.Lock()
	defer p.loadMu.Unlock()
	p.mu.RLock()
	g = p.graph
	p.mu.RUnlock()
	if g.Version == version && version != 0 {
		return g, nil
	}
	loaded, err := p.loader.LoadGraph(ctx, version)
	if err != nil {
		return Graph{}, err
	}
	if loaded.Version != version {
		return Graph{}, errors.New("route graph version mismatch")
	}
	p.mu.Lock()
	p.graph = loaded
	p.cache = map[string]cacheEntry{}
	p.mu.Unlock()
	return loaded, nil
}
func (p *Planner) evictLocked(now time.Time) {
	for k, v := range p.cache {
		if now.After(v.expires) {
			delete(p.cache, k)
		}
	}
	if len(p.cache) < p.maxEntries {
		return
	}
	for k := range p.cache {
		delete(p.cache, k)
		break
	}
}
func cloneResult(r Result) Result { r.Systems = append([]System(nil), r.Systems...); return r }
func search(ctx context.Context, g Graph, q Query) (Result, error) {
	a, aok := g.Systems[q.From]
	_, bok := g.Systems[q.To]
	if !aok || !bok || a.Security < q.MinSecurity {
		return Result{}, ErrNoRoute
	}
	if q.From == q.To {
		return Result{Status: "ready", From: q.From, To: q.To, Version: g.Version, Jumps: 0, MinSecurity: a.Security, Systems: []System{a}}, nil
	}
	prev := map[int64]int64{}
	depth := map[int64]int{q.From: 0}
	seen := map[int64]bool{q.From: true}
	queue := list.New()
	queue.PushBack(q.From)
	found := false
	for queue.Len() > 0 && !found {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		v := queue.Remove(queue.Front()).(int64)
		if depth[v] >= q.MaxJumps {
			continue
		}
		for _, n := range g.Neighbors[v] {
			s, ok := g.Systems[n]
			if !ok || seen[n] || s.Security < q.MinSecurity {
				continue
			}
			seen[n] = true
			prev[n] = v
			depth[n] = depth[v] + 1
			if n == q.To {
				found = true
				break
			}
			queue.PushBack(n)
		}
	}
	if !found {
		return Result{}, ErrNoRoute
	}
	ids := []int64{q.To}
	for ids[len(ids)-1] != q.From {
		ids = append(ids, prev[ids[len(ids)-1]])
	}
	systems := make([]System, len(ids))
	min := 2.0
	for i := range ids {
		s := g.Systems[ids[len(ids)-1-i]]
		systems[i] = s
		if s.Security < min {
			min = s.Security
		}
	}
	return Result{Status: "ready", From: q.From, To: q.To, Version: g.Version, Jumps: len(systems) - 1, MinSecurity: min, Systems: systems}, nil
}
