package video

import (
	"context"
	"errors"
	"sync"
	"time"
)

type cacheEntry struct {
	value   response
	err     error
	expires time.Time
}
type flight struct {
	done  chan struct{}
	value response
	err   error
}
type cache struct {
	mu                   sync.Mutex
	entries              map[string]cacheEntry
	flights              map[string]*flight
	maxEntries, maxBytes int
	cacheErrors          bool
}

func newCache(entries, bytes int) *cache {
	return &cache{entries: map[string]cacheEntry{}, flights: map[string]*flight{}, maxEntries: entries, maxBytes: bytes, cacheErrors: true}
}

// Waiters share one fetch. Cancellation is never cached; a disconnected caller
// cannot leave background platform/ASR work running indefinitely.
func (c *cache) get(ctx context.Context, key string, fetch func() (response, error)) (response, error) {
	if err := ctx.Err(); err != nil {
		return response{}, err
	}
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.value, entry.err
	}
	if pending, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return response{}, ctx.Err()
		case <-pending.done:
			return pending.value, pending.err
		}
	}
	if len(c.flights) >= c.maxEntries {
		c.mu.Unlock()
		return response{}, problem("platform_busy", "视频读取正忙，请稍后重试")
	}
	pending := &flight{done: make(chan struct{})}
	c.flights[key] = pending
	c.mu.Unlock()
	value, err := fetch()
	c.mu.Lock()
	pending.value, pending.err = value, err
	delete(c.flights, key)
	if (err == nil || c.cacheErrors) && ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && len(value.Body) <= c.maxBytes {
		ttl := 10 * time.Minute
		if err != nil {
			ttl = 30 * time.Second
		}
		now := time.Now()
		bytes := len(value.Body)
		for k, entry := range c.entries {
			if now.After(entry.expires) || k == key {
				delete(c.entries, k)
			} else {
				bytes += len(entry.value.Body)
			}
		}
		for len(c.entries) >= c.maxEntries || bytes > c.maxBytes {
			var oldest string
			var expiry time.Time
			for k, e := range c.entries {
				if expiry.IsZero() || e.expires.Before(expiry) {
					oldest, expiry = k, e.expires
				}
			}
			if oldest == "" {
				break
			}
			bytes -= len(c.entries[oldest].value.Body)
			delete(c.entries, oldest)
		}
		c.entries[key] = cacheEntry{value: value, err: err, expires: now.Add(ttl)}
	}
	close(pending.done)
	c.mu.Unlock()
	return value, err
}
