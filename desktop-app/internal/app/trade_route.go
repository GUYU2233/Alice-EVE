package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type TradeRouteSystem struct {
	SystemID       int64   `json:"systemId"`
	Name           string  `json:"name"`
	SecurityStatus float64 `json:"securityStatus"`
}
type TradeRouteResult struct {
	Status       string             `json:"status"`
	FromSystemID int64              `json:"fromSystemId"`
	ToSystemID   int64              `json:"toSystemId"`
	Jumps        int                `json:"jumps"`
	MinSecurity  float64            `json:"minSecurity"`
	Systems      []TradeRouteSystem `json:"systems"`
}
type TradeRouteQuery struct {
	From        int64   `json:"From"`
	To          int64   `json:"To"`
	MinSecurity float64 `json:"MinSecurity"`
	MaxJumps    int     `json:"MaxJumps"`
}
type routeCacheEntry struct {
	result  TradeRouteResult
	expires time.Time
}
type routeClientCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[string]routeCacheEntry
}

func newRouteClientCache(ttl time.Duration, max int) *routeClientCache {
	return &routeClientCache{ttl: ttl, max: max, items: map[string]routeCacheEntry{}}
}
func routeKey(q TradeRouteQuery) string {
	return fmt.Sprintf("%d/%d/%g/%d", q.From, q.To, q.MinSecurity, q.MaxJumps)
}
func (c *routeClientCache) get(q TradeRouteQuery) (TradeRouteResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[routeKey(q)]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(c.items, routeKey(q))
		}
		return TradeRouteResult{}, false
	}
	return e.result, true
}
func (c *routeClientCache) put(q TradeRouteQuery, r TradeRouteResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		now := time.Now()
		for k, v := range c.items {
			if now.After(v.expires) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= c.max {
			for k := range c.items {
				delete(c.items, k)
				break
			}
		}
	}
	c.items[routeKey(q)] = routeCacheEntry{r, time.Now().Add(c.ttl)}
}
func (r *RelayClient) SecureTradeRoute(ctx context.Context, from, to int64, minSecurity float64, maxJumps int) (TradeRouteResult, error) {
	out, err := r.SecureTradeRoutes(ctx, []TradeRouteQuery{{from, to, minSecurity, maxJumps}})
	if err != nil {
		return TradeRouteResult{}, err
	}
	return out[0], nil
}
func (r *RelayClient) SecureTradeRoutes(ctx context.Context, queries []TradeRouteQuery) ([]TradeRouteResult, error) {
	if len(queries) == 0 || len(queries) > 500 {
		return nil, fmt.Errorf("invalid route batch")
	}
	out := make([]TradeRouteResult, len(queries))
	missing := make([]TradeRouteQuery, 0)
	positions := map[string][]int{}
	for i, q := range queries {
		if v, ok := r.routeCache.get(q); ok {
			out[i] = v
		} else {
			k := routeKey(q)
			if len(positions[k]) == 0 {
				missing = append(missing, q)
			}
			positions[k] = append(positions[k], i)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	body, _ := json.Marshal(map[string]any{"queries": missing})
	var response struct {
		Routes []TradeRouteResult `json:"routes"`
	}
	if err := r.doRequest(ctx, http.MethodPost, "/api/v1/trade/routes", body, &response, true); err != nil {
		return nil, err
	}
	if len(response.Routes) != len(missing) {
		return nil, fmt.Errorf("route batch response mismatch")
	}
	for i, v := range response.Routes {
		q := missing[i]
		if v.Status == "ready" {
			r.routeCache.put(q, v)
		}
		for _, p := range positions[routeKey(q)] {
			out[p] = v
		}
	}
	return out, nil
}
