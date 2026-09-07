// Command service is a runnable HTTP API wired together entirely from the
// toolkit: typed env config, a middleware stack (request IDs, access logging,
// CORS, timeouts, auth, scope checks, rate limiting), and health endpoints.
//
//	go run ./examples/service
//	curl localhost:8080/healthz
//	curl -H 'Authorization: Bearer demo' localhost:8080/v1/things
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rithulkamesh/toolkit/env"
	"github.com/rithulkamesh/toolkit/httpx"
)

func main() {
	addr := env.String("HTTP_ADDR", ":8080")
	reqTimeout := env.Duration("HTTP_TIMEOUT", 15*time.Second)
	origins := env.List("CORS_ORIGINS", []string{"*"})

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()
	limiter := httpx.NewMemoryStore(ctx)

	// Toy resolver: "Authorization: Bearer demo" is the only valid credential.
	resolver := httpx.ResolverFunc(func(r *http.Request) (*httpx.Principal, error) {
		if r.Header.Get("Authorization") != "Bearer demo" {
			return nil, nil
		}
		return &httpx.Principal{
			Subject:   "user_demo",
			TenantID:  "tenant_demo",
			Scopes:    []string{"things:read"},
			TokenKind: httpx.TokenSession,
		}, nil
	})

	protected := httpx.Chain(
		httpx.Authenticate(resolver),
		httpx.RequireScope("things:read"),
		httpx.RateLimit(limiter, httpx.RateLimitConfig{
			Limit: 10, Window: time.Minute, Prefix: "things", Key: httpx.KeyByPrincipal,
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/", httpx.Health(httpx.HealthCheck{
		Name:  "self",
		Check: func(context.Context) error { return nil },
	}))
	mux.Handle("/v1/things", protected(http.HandlerFunc(listThings)))

	root := httpx.Chain(
		httpx.RequestID(),
		httpx.AccessLog(logger),
		httpx.CORS(httpx.CORSConfig{AllowedOrigins: origins}),
		httpx.Timeout(reqTimeout, "request timed out"),
	)(mux)

	logger.Info("listening", "addr", addr)
	if err := http.ListenAndServe(addr, root); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func listThings(w http.ResponseWriter, r *http.Request) {
	var subject string
	if p, ok := httpx.PrincipalFrom(r.Context()); ok {
		subject = p.Subject
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"things":     []string{"alpha", "beta", "gamma"},
		"principal":  subject,
		"request_id": httpx.RequestIDFrom(r.Context()),
	})
}
