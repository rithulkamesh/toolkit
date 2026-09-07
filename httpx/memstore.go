package httpx

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	count  int64
	expiry time.Time
}

// MemoryStore is an in-process fixed-window [LimitStore]. It is not shared
// between processes — use a Redis-backed store for multi-instance
// deployments. A background sweeper evicts expired keys.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]*memEntry
	now     func() time.Time
}

// NewMemoryStore returns a ready MemoryStore and starts its sweeper, which runs
// until ctx is cancelled.
func NewMemoryStore(ctx context.Context) *MemoryStore {
	s := &MemoryStore{entries: make(map[string]*memEntry), now: time.Now}
	go s.sweep(ctx)
	return s
}

// Incr implements [LimitStore].
func (s *MemoryStore) Incr(_ context.Context, key string, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	e := s.entries[key]
	if e == nil || now.After(e.expiry) {
		e = &memEntry{expiry: now.Add(window)}
		s.entries[key] = e
	}
	e.count++
	return e.count, nil
}

func (s *MemoryStore) sweep(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.mu.Lock()
			now := s.now()
			for k, e := range s.entries {
				if now.After(e.expiry) {
					delete(s.entries, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
