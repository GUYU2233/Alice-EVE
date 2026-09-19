package app

import (
	"testing"
	"time"
)

func TestRouteClientCacheTTLAndBounds(t *testing.T) {
	c := newRouteClientCache(20*time.Millisecond, 2)
	q := TradeRouteQuery{1, 2, .5, 10}
	c.put(q, TradeRouteResult{Status: "ready", Jumps: 3})
	if r, ok := c.get(q); !ok || r.Jumps != 3 {
		t.Fatal("cache miss")
	}
	time.Sleep(25 * time.Millisecond)
	if _, ok := c.get(q); ok {
		t.Fatal("expired cache hit")
	}
	c.put(q, TradeRouteResult{})
	c.put(TradeRouteQuery{2, 3, .5, 10}, TradeRouteResult{})
	c.put(TradeRouteQuery{3, 4, .5, 10}, TradeRouteResult{})
	if len(c.items) > 2 {
		t.Fatal("cache exceeded bound")
	}
}
func TestUnavailableRouteIsNotStoredAsLongLivedClientCacheEntry(t *testing.T) {
	c := newRouteClientCache(time.Hour, 2)
	q := TradeRouteQuery{1, 2, .5, 10}
	if r := (TradeRouteResult{Status: "unavailable"}); r.Status == "ready" {
		c.put(q, r)
	}
	if _, ok := c.get(q); ok {
		t.Fatal("unavailable route should be retried rather than cached")
	}
}

func TestRouteKeyIncludesConstraints(t *testing.T) {
	a := routeKey(TradeRouteQuery{1, 2, .5, 10})
	if a == routeKey(TradeRouteQuery{1, 2, 0, 10}) || a == routeKey(TradeRouteQuery{1, 2, .5, 20}) {
		t.Fatal("constraint collision")
	}
}
