package httpx

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// LimitStore is a fixed-window counter backend. Incr adds one to the counter
// for key and returns the new total, applying the window as a TTL on the first
// increment. [NewMemoryStore] is an in-process implementation; a Redis-backed
// store is a few lines (see the package README).
type LimitStore interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// RateLimitConfig configures [RateLimit].
type RateLimitConfig struct {
	Limit  int                        // max requests per window (required, > 0)
	Window time.Duration              // window length (required, > 0)
	Prefix string                     // key namespace, e.g. "login" or "api"
	Key    func(*http.Request) string // bucket key; defaults to [ClientIP]
}

// RateLimit rejects requests over the configured limit with 429 and a
// Retry-After header. If the store returns an error the request is allowed
// through (fail-open): a broken limiter must not take down the API.
func RateLimit(store LimitStore, cfg RateLimitConfig) Middleware {
	if cfg.Key == nil {
		cfg.Key = ClientIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.Prefix + ":" + cfg.Key(r)
			n, err := store.Incr(r.Context(), key, cfg.Window)
			if err == nil && n > int64(cfg.Limit) {
				w.Header().Set("Retry-After", strconv.Itoa(int(cfg.Window.Seconds())))
				Error(w, http.StatusTooManyRequests, "rate limit exceeded", "rate_limited")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// KeyByPrincipal buckets the limiter by authenticated subject, falling back to
// [ClientIP] for anonymous requests. Use it as RateLimitConfig.Key on routes
// behind [Authenticate].
func KeyByPrincipal(r *http.Request) string {
	if p, ok := PrincipalFrom(r.Context()); ok && p.Subject != "" {
		return "sub:" + p.Subject
	}
	return ClientIP(r)
}
