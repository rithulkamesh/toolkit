package secretsx

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Cached wraps a [Provider] with a per-key TTL cache, so a hot path does not
// hit the backend on every read. It is safe for concurrent use.
//
// Successful reads are cached for TTL. Set [Cached.NegativeTTL] to also cache
// [ErrNotFound] for a shorter window and blunt lookup storms for missing keys;
// other errors are never cached.
type Cached struct {
	Provider    Provider
	TTL         time.Duration
	NegativeTTL time.Duration

	now     func() time.Time // swappable in tests
	mu      sync.Mutex
	entries map[string]entry
}

type entry struct {
	val     string
	err     error
	expires time.Time
}

// NewCached returns a [Cached] over p with the given positive TTL.
func NewCached(p Provider, ttl time.Duration) *Cached {
	return &Cached{Provider: p, TTL: ttl}
}

// Get implements [Provider].
func (c *Cached) Get(ctx context.Context, key string) (string, error) {
	now := c.clock()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Before(e.expires) {
		c.mu.Unlock()
		return e.val, e.err
	}
	c.mu.Unlock()

	val, err := c.Provider.Get(ctx, key)

	switch {
	case err == nil:
		c.store(key, entry{val: val, expires: now.Add(c.TTL)})
	case errors.Is(err, ErrNotFound) && c.NegativeTTL > 0:
		c.store(key, entry{err: err, expires: now.Add(c.NegativeTTL)})
	}
	return val, err
}

// Invalidate drops key from the cache. With no keys it clears everything.
func (c *Cached) Invalidate(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(keys) == 0 {
		c.entries = nil
		return
	}
	for _, k := range keys {
		delete(c.entries, k)
	}
}

func (c *Cached) store(key string, e entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]entry)
	}
	c.entries[key] = e
}

func (c *Cached) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
