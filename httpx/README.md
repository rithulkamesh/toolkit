# httpx

Composable `net/http` middleware for a JSON API gateway. Zero dependencies.

## The stack

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
limiter := httpx.NewMemoryStore(ctx) // or a Redis-backed LimitStore

root := httpx.Chain(
	httpx.RequestID(),
	httpx.AccessLog(logger),
	httpx.CORS(httpx.CORSConfig{
		AllowedOrigins:   []string{"https://app.example"},
		AllowCredentials: true,
	}),
	httpx.Timeout(15*time.Second, "request timed out"),
)

api := httpx.Chain(
	httpx.Authenticate(myResolver),
	httpx.RequireCSRF(httpx.CSRFConfig{SessionCookieName: "session", Secure: true}),
	httpx.RateLimit(limiter, httpx.RateLimitConfig{Limit: 600, Window: time.Minute, Key: httpx.KeyByPrincipal}),
)

mux := http.NewServeMux()
mux.Handle("/", httpx.Health(dbCheck))
mux.Handle("/v1/workflows", api(httpx.Chain(httpx.RequireScope("workflows:read"))(workflowsHandler)))
mux.Handle("/v1/billing/plan", api(httpx.Chain(httpx.RequireLiveKey())(billingHandler)))

http.ListenAndServe(":8080", root(mux))
```

## You provide auth

The package never assumes how you authenticate. Implement `Resolver`:

```go
myResolver := httpx.ResolverFunc(func(r *http.Request) (*httpx.Principal, error) {
	// 1. session cookie?  2. Authorization: Bearer <session token>?  3. API key?
	if key := apiKey(r); key != "" {
		rec, err := db.LookupKey(r.Context(), key)
		if err != nil {
			return nil, err // presented but rejected -> 401
		}
		kind := httpx.TokenLiveKey
		if strings.HasPrefix(key, "sk_test_") {
			kind = httpx.TokenTestKey
		}
		return &httpx.Principal{Subject: rec.UserID, TenantID: rec.OrgID, Scopes: rec.Scopes, TokenKind: kind}, nil
	}
	return nil, nil // nothing presented -> 401 (Authenticate) or anonymous (OptionalAuth)
})
```

## Middleware reference

| Function | Effect |
|---|---|
| `RequestID()` | reuse/generate `X-Request-Id`, put it in context + response |
| `AccessLog(logger)` | one `slog` line per request; 5xx→Error, 4xx→Warn |
| `CORS(cfg)` | preflight + response headers; `*`+credentials handled safely |
| `Timeout(d, msg)` | 503 if a handler exceeds `d` (not for streams) |
| `WriteDeadline(d, skip)` | per-connection write deadline; `skip` exempts SSE routes |
| `Authenticate(resolver)` | resolve principal → context, else 401 |
| `OptionalAuth(resolver)` | attach principal if present, never blocks |
| `RequireScope(s)` / `RequireAnyScope` / `RequireAllScopes` | 401 no principal, 403 missing scope; exact match, no wildcards |
| `RequireLiveKey()` | 403 for `TokenTestKey` regardless of scopes |
| `Gate(status, code, msg, check)` | generic precondition (email verified, org named, plan active) |
| `RequireCSRF(cfg)` | double-submit cookie on unsafe methods; bearer + safe methods exempt |
| `RateLimit(store, cfg)` | fixed-window 429 + `Retry-After`; fail-open |
| `Health(checks...)` | `GET /healthz` (liveness) + `GET /readyz` (readiness) |

## Redis rate-limit store

`RateLimit` takes any `LimitStore`. The Redis version:

```go
type redisStore struct{ rdb *redis.Client }

func (s redisStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	n, err := s.rdb.Incr(ctx, key).Result()
	if err == nil && n == 1 {
		s.rdb.Expire(ctx, key, window)
	}
	return n, err
}
```

## Responses

Errors are JSON: `{"error":"forbidden","code":"forbidden"}`. Use `httpx.Error(w, status, msg, code)` and `httpx.JSON(w, status, v)` in your own handlers for consistency.
