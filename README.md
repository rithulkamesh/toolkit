# toolkit

Small, dependency-free Go building blocks for standing up a JSON API service:
a concurrent DAG executor, HTTP middleware for an API gateway, typed env
config, an outbound-host guard for LLM/third-party calls, and an SMTP mailer.

Extracted and generalised from a production multi-service platform. Every
package is stdlib-only, independently useful, and small enough to read in one
sitting.

```
go get github.com/rithulkamesh/toolkit@latest
```

> The module path above is a placeholder — **rename it to your own first**:
> `scripts/rename-module.sh github.com/you/toolkit`

## Packages

| Package | What it does | Size |
|---|---|---|
| [`dag`](./dag) | Run a dependency graph of work concurrently, in order, with cascading failure handling. Generic over any `{ ID(); DependsOn() }` type. | ~250 LoC |
| [`httpx`](./httpx) | Composable `net/http` middleware: auth, scopes, CSRF, rate limiting, CORS, timeouts, request IDs, access logging, health/readiness. You bring the auth logic; it wires everything else. | ~700 LoC |
| [`env`](./env) | `env.String/Int/Bool/Duration/List` with fallbacks; `env.Required/MustString` for startup config. | ~90 LoC |
| [`llmguard`](./llmguard) | Fail-closed allowlist for outbound API hosts, so a bad prompt or config can't exfiltrate to an arbitrary endpoint. Exact + `*.wildcard` matching. | ~110 LoC |
| [`mail`](./mail) | Transactional email over plain SMTP (SES / Postmark / Mailgun / Resend / self-hosted). `LogSender` for dev and tests. | ~90 LoC |

Nothing here imports anything outside the Go standard library. `go.sum` is
empty on purpose.

## Quick start

```go
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	limiter := httpx.NewMemoryStore(context.Background())

	resolver := httpx.ResolverFunc(func(r *http.Request) (*httpx.Principal, error) {
		// look up a session cookie / verify a JWT / check an API key here
		if r.Header.Get("Authorization") == "" {
			return nil, nil
		}
		return &httpx.Principal{Subject: "u1", Scopes: []string{"things:read"}}, nil
	})

	mux := http.NewServeMux()
	mux.Handle("/", httpx.Health())
	mux.Handle("/v1/things", httpx.Chain(
		httpx.Authenticate(resolver),
		httpx.RequireScope("things:read"),
		httpx.RateLimit(limiter, httpx.RateLimitConfig{Limit: 100, Window: time.Minute}),
	)(http.HandlerFunc(listThings)))

	root := httpx.Chain(
		httpx.RequestID(),
		httpx.AccessLog(logger),
		httpx.CORS(httpx.CORSConfig{AllowedOrigins: env.List("CORS_ORIGINS", []string{"*"})}),
		httpx.Timeout(15*time.Second, "request timed out"),
	)(mux)

	http.ListenAndServe(env.String("HTTP_ADDR", ":8080"), root)
}
```

A complete runnable version, including a concurrent `dag` pipeline endpoint, is
in [`examples/service`](./examples/service):

```
go run ./examples/service
curl localhost:8080/healthz
curl -H 'Authorization: Bearer demo' -X POST localhost:8080/pipeline
```

## Using it with an AI coding agent

This repo is meant to be handed to an assistant as a foundation. Point it at
[`AGENTS.md`](./AGENTS.md) — it explains the layout, the zero-dependency
constraint, and how to extend each package without breaking the design.

## Develop

```
make ci     # gofmt + go vet + go test + go build
make test
```

Go 1.23+ (generics with methods, `slices`, `log/slog`).

## Roadmap

Deliberately left out of v1 to keep the module dependency-free. Each can be
added as a separate module so consumers opt in:

- `otelx` — one-call OpenTelemetry traces + metrics with a Prometheus endpoint.
- `kafkax` — an event `Envelope`, DLQ writer, and W3C trace-context over Kafka headers.
- `secretsx` — Vault KV v2 with env fallback.
- `connectorkit` — a `Provider` interface + registry for third-party integrations (OAuth, poll/webhook, tool calls).
- A Rust "durable step worker" template that consumes events and executes `dag` steps.
- A React + TanStack frontend starter (typed fetch client, SSE run timeline, DAG visualiser).

## License

MIT — see [LICENSE](./LICENSE).
