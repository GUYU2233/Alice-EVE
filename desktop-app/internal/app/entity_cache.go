package app

import (
	"sync"
	"time"
)

type entityCacheEntry struct {
	value   any
	expires time.Time
}
type entityClientCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	items map[string]entityCacheEntry
}

func newEntityClientCache(ttl time.Duration, max int) *entityClientCache {
	return &entityClientCache{ttl: ttl, max: max, items: map[string]entityCacheEntry{}}
}
func (c *entityClientCache) get(k string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(c.items, k)
		}
		return nil, false
	}
	return e.value, true
}
func (c *entityClientCache) put(k string, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.max {
		now := time.Now()
		for k, e := range c.items {
			if now.After(e.expires) {
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
	c.items[k] = entityCacheEntry{v, time.Now().Add(c.ttl)}
}
