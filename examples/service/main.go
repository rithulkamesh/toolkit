// Command service is a runnable HTTP API wired together entirely from the
// toolkit: typed env config, a middleware stack (request IDs, access logging,
// CORS, timeouts, auth, scope checks, rate limiting), health endpoints, and a
// /pipeline route that executes a small dependency graph concurrently.
//
//	go run ./examples/service
//	curl localhost:8080/healthz
//	curl -H 'Authorization: Bearer demo' -X POST localhost:8080/pipeline
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/rithulkamesh/toolkit/dag"
	"github.com/rithulkamesh/toolkit/env"
	"github.com/rithulkamesh/toolkit/httpx"
)

// task is a trivial dag.Node.
type task struct {
	name string
	deps []string
}

func (t task) ID() string          { return t.name }
func (t task) DependsOn() []string { return t.deps }

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
			Scopes:    []string{"pipeline:run"},
			TokenKind: httpx.TokenSession,
		}, nil
	})

	protected := httpx.Chain(
		httpx.Authenticate(resolver),
		httpx.RequireScope("pipeline:run"),
		httpx.RateLimit(limiter, httpx.RateLimitConfig{
			Limit: 10, Window: time.Minute, Prefix: "pipeline", Key: httpx.KeyByPrincipal,
		}),
	)

	mux := http.NewServeMux()
	mux.Handle("/", httpx.Health(httpx.HealthCheck{
		Name:  "self",
		Check: func(context.Context) error { return nil },
	}))
	mux.Handle("/pipeline", protected(http.HandlerFunc(runPipeline)))

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

func runPipeline(w http.ResponseWriter, r *http.Request) {
	tasks := []task{
		{name: "fetch"},
		{name: "parse", deps: []string{"fetch"}},
		{name: "enrich", deps: []string{"fetch"}},
		{name: "write", deps: []string{"parse", "enrich"}},
	}

	start := time.Now()
	err := dag.Run(r.Context(), tasks, func(_ context.Context, t task) error {
		time.Sleep(50 * time.Millisecond) // stand-in for real work
		if t.name == "" {
			return errors.New("empty task")
		}
		return nil
	}, dag.RunOptions{MaxConcurrency: 4})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error(), "pipeline_failed")
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"tasks":      len(tasks),
		"duration":   time.Since(start).String(),
		"request_id": httpx.RequestIDFrom(r.Context()),
	})
}
