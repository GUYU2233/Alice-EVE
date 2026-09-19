package evesde

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
)

var ErrNoRoute = errors.New("evesde: no route satisfying constraints")

type RouteMode string

const (
	RouteShortest RouteMode = "shortest"
	RouteSafest   RouteMode = "safest"
)

// RouteOptions controls traversal. MinSecurity is compared to the raw SDE
// security value; nil imposes no floor. Origin is always allowed so a pilot
// can route out of a system below the requested floor.
type RouteOptions struct {
	Mode        RouteMode
	MinSecurity *float64
}

// RouteResult includes both endpoints in Systems. Security counts cover
// destination systems entered (the origin is excluded); low-sec is [0, 0.5)
// and null-sec is < 0 using raw SDE values.
type RouteResult struct {
	Systems      []System `json:"systems"`
	Jumps        int      `json:"jumps"`
	MinSecurity  float64  `json:"minSecurity"`
	LowSecCount  int      `json:"lowSecCount"`
	NullSecCount int      `json:"nullSecCount"`
}

type routeCost struct{ null, low, jumps int }

func (c routeCost) less(other routeCost, mode RouteMode) bool {
	if mode == RouteSafest {
		if c.null != other.null {
			return c.null < other.null
		}
		if c.low != other.low {
			return c.low < other.low
		}
	}
	return c.jumps < other.jumps
}

func (c routeCost) equalPriority(other routeCost, mode RouteMode) bool {
	if mode == RouteSafest {
		return c == other
	}
	return c.jumps == other.jumps
}

// Route finds a deterministic route over SDE stargates. Safest minimizes
// null-sec entries, then low-sec entries, then jumps; shortest minimizes jumps.
// Equal-cost choices are resolved by ascending system ID.
func (r *Repository) Route(ctx context.Context, originID, destinationID int64, opts RouteOptions) (RouteResult, error) {
	if originID <= 0 || destinationID <= 0 {
		return RouteResult{}, fmt.Errorf("system ids must be positive")
	}
	if opts.Mode == "" {
		opts.Mode = RouteShortest
	}
	if opts.Mode != RouteShortest && opts.Mode != RouteSafest {
		return RouteResult{}, fmt.Errorf("unsupported route mode %q", opts.Mode)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,constellation_id,name,security FROM systems ORDER BY id`)
	if err != nil {
		return RouteResult{}, err
	}
	defer rows.Close()
	systems := map[int64]System{}
	for rows.Next() {
		var s System
		if err = rows.Scan(&s.ID, &s.ConstellationID, &s.Name, &s.Security); err != nil {
			return RouteResult{}, err
		}
		systems[s.ID] = s
	}
	if err = rows.Err(); err != nil {
		return RouteResult{}, err
	}
	if _, ok := systems[originID]; !ok {
		return RouteResult{}, ErrNotFound
	}
	if _, ok := systems[destinationID]; !ok {
		return RouteResult{}, ErrNotFound
	}
	if originID == destinationID {
		return summarizeRoute([]System{systems[originID]}), nil
	}
	grows, err := r.db.QueryContext(ctx, `SELECT system_id,destination_system_id FROM stargates ORDER BY system_id,destination_system_id,id`)
	if err != nil {
		return RouteResult{}, err
	}
	defer grows.Close()
	adj := map[int64][]int64{}
	for grows.Next() {
		var a, b int64
		if err = grows.Scan(&a, &b); err != nil {
			return RouteResult{}, err
		}
		adj[a] = append(adj[a], b)
	}
	if err = grows.Err(); err != nil {
		return RouteResult{}, err
	}
	for id := range adj {
		sort.Slice(adj[id], func(i, j int) bool { return adj[id][i] < adj[id][j] })
	}
	inf := routeCost{math.MaxInt, math.MaxInt, math.MaxInt}
	costs := map[int64]routeCost{originID: {}}
	prev := map[int64]int64{}
	visited := map[int64]bool{}
	for {
		cur := int64(0)
		best := inf
		for id, c := range costs {
			if visited[id] {
				continue
			}
			if cur == 0 || c.less(best, opts.Mode) || (c.equalPriority(best, opts.Mode) && id < cur) {
				cur, best = id, c
			}
		}
		if cur == 0 {
			break
		}
		if cur == destinationID {
			break
		}
		visited[cur] = true
		for _, next := range adj[cur] {
			s, ok := systems[next]
			if !ok {
				continue
			}
			if opts.MinSecurity != nil && s.Security < *opts.MinSecurity {
				continue
			}
			nc := best
			nc.jumps++
			if s.Security < 0 {
				nc.null++
			} else if s.Security < 0.5 {
				nc.low++
			}
			old, seen := costs[next]
			if !seen || nc.less(old, opts.Mode) || (nc.equalPriority(old, opts.Mode) && cur < prev[next]) {
				costs[next] = nc
				prev[next] = cur
			}
		}
	}
	if _, ok := costs[destinationID]; !ok {
		return RouteResult{}, ErrNoRoute
	}
	ids := []int64{destinationID}
	for ids[len(ids)-1] != originID {
		p, ok := prev[ids[len(ids)-1]]
		if !ok {
			return RouteResult{}, ErrNoRoute
		}
		ids = append(ids, p)
	}
	path := make([]System, len(ids))
	for i := range ids {
		path[len(ids)-1-i] = systems[ids[i]]
	}
	return summarizeRoute(path), nil
}

func summarizeRoute(path []System) RouteResult {
	out := RouteResult{Systems: path, Jumps: len(path) - 1, MinSecurity: path[0].Security}
	for i, s := range path {
		if s.Security < out.MinSecurity {
			out.MinSecurity = s.Security
		}
		if i == 0 {
			continue
		}
		if s.Security < 0 {
			out.NullSecCount++
		} else if s.Security < 0.5 {
			out.LowSecCount++
		}
	}
	return out
}
