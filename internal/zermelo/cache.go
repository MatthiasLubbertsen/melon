package zermelo

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache keeps upstream answers just long enough to absorb bursts (several
// tabs, a refetch on focus, an MCP client and the app asking at once). It is
// deliberately short: a schedule that is a minute old may already be wrong.
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	group   singleflight.Group
}

type entry struct {
	value any
	at    time.Time
}

func NewCache() *Cache {
	c := &Cache{entries: map[string]entry{}}
	go func() {
		for range time.Tick(time.Minute) {
			c.mu.Lock()
			for k, e := range c.entries {
				if time.Since(e.at) > StaleFor {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		}
	}()
	return c
}

const (
	// MinAge is the floor even an explicit refresh cannot go below.
	MinAge = 5 * time.Second
	// StaleFor is how long an old answer may stand in when Zermelo is down.
	StaleFor = 30 * time.Minute
)

// Result describes where a value came from.
type Result struct {
	FetchedAt time.Time
	// Stale is set when Zermelo failed and an older answer is served instead.
	Stale bool
}

// Get returns the cached value when younger than ttl, otherwise fetches.
// Concurrent callers for the same key share one upstream request.
func Get[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, fetch func(context.Context) (T, error)) (T, Result, error) {
	if ttl < MinAge {
		ttl = MinAge
	}
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && time.Since(e.at) < ttl {
		return e.value.(T), Result{FetchedAt: e.at}, nil
	}
	type fetched struct {
		v  T
		at time.Time
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		// Detached from the first caller's context so one cancelled request
		// does not fail everyone waiting on the same fetch.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		val, err := fetch(fctx)
		if err != nil {
			return nil, err
		}
		at := time.Now()
		c.mu.Lock()
		c.entries[key] = entry{value: val, at: at}
		c.mu.Unlock()
		return fetched{val, at}, nil
	})
	if err != nil {
		// Only outages justify stale data; auth errors must surface.
		if ok && (isOutage(err)) && time.Since(e.at) < StaleFor {
			return e.value.(T), Result{FetchedAt: e.at, Stale: true}, nil
		}
		var zero T
		return zero, Result{}, err
	}
	f := v.(fetched)
	return f.v, Result{FetchedAt: f.at}, nil
}

// Drop forgets every entry whose key starts with prefix.
func (c *Cache) Drop(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.entries, k)
		}
	}
}
