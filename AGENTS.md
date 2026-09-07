# AGENTS.md

Instructions for an AI coding agent working in this repository. (Also valid as
`CLAUDE.md` — symlink or copy if your tool expects that name.)

## What this repo is

A small library of **dependency-free Go building blocks** for standing up a
JSON API service. It is a foundation to build an application *on top of*, not an
application itself. Four packages:

- `httpx/` — composable `net/http` middleware for an API gateway.
- `env/` — typed environment-variable reads with fallbacks.
- `llmguard/` — fail-closed outbound-host allowlist.
- `mail/` — SMTP transactional email.
- `examples/service/` — a runnable service wiring the above together.

## Hard constraints — do not break these

1. **Standard library only — the root module.** No third-party imports under
   `httpx/`, `env/`, `llmguard/`, `mail/`, `examples/`. The root `go.mod` has no
   `require` block and `go.sum` does not exist. A block that needs a dependency
   goes in its own nested module (see "Companion modules" below), never here.
2. **Go 1.23+** for the root module, and keep it that way unless the user asks
   to bump it. Nested modules track their heaviest dependency's minimum.
3. **Every exported symbol has a doc comment** that starts with its name.
4. **Every package keeps its `README.md` and/or `doc.go` in sync** with the
   code. If you change behaviour, update the prose in the same change.
5. **Tests are table-driven where it makes sense, use only `testing` +
   `net/http/httptest`, and must pass with `-race`.**
6. **No breaking changes to exported APIs** without calling it out explicitly
   in your summary to the user.

## Conventions

- Middleware has the type `httpx.Middleware = func(http.Handler) http.Handler`
  and is composed with `httpx.Chain`. New middleware follows the same shape and
  the "factory returns a `Middleware`" pattern (`RequireScope(scope)` →
  `Middleware`).
- Error responses are JSON via `httpx.Error(w, status, msg, code)`. Success
  bodies via `httpx.JSON(w, status, v)`.
- Config structs take a `withDefaults()` method rather than requiring every
  field (see `httpx.CSRFConfig`).
- Prefer clarity over cleverness. These files are meant to be read start to
  finish by a human evaluating whether to adopt the library.

## How to extend

**Add a middleware:** new file in `httpx/`, factory returning `Middleware`,
doc comment, a row in `httpx/README.md`'s reference table, and a test in
`httpx/httpx_test.go`.

**Add an `env` getter:** new function in `env/env.go` following the
`(key, fallback) -> value` shape (never returns an error; add a `Required`
variant if a missing value should fail), plus a `env_test.go` case.

**Add a companion module** (a block that needs dependencies): create it as its
**own module** in a subdirectory with its own `go.mod`, so the root module
stays dependency-free. Add a row to the root `README.md` "Companion modules"
table and a matrix entry in `.github/workflows/ci.yml`'s `modules` job.

## Companion modules

Multi-provider blocks that carry dependencies. Each is a separate Go module:

- `secretsx/` — `Provider` interface, `Chain`, `Cached`, stdlib `Env`/`Dir`/
  `JSONFile`. Stdlib only.
- `secretsx/vault/` — Vault KV v2 over `net/http`. Stdlib only.
- `secretsx/awssm/` — AWS Secrets Manager + SSM Parameter Store. Uses aws-sdk-go-v2.
- `kafkax/` — `Envelope`, `Producer`/`Consumer`, W3C trace-context, DLQ wrapper.
  Stdlib only.
- `kafkax/franz/` — franz-go binding. Uses github.com/twmb/franz-go.
- `otelx/` — OpenTelemetry traces + metrics, exporter switch. Uses the OTel SDK.

Rules:

- A nested module that talks to the shared core (`secretsx/vault` → `secretsx`)
  uses a `replace github.com/rithulkamesh/toolkit/<core> => ../` directive so
  the repo builds against local code. Publishing standalone needs a real
  per-module tag (`secretsx/v0.1.0`) and the `require` updated to match.
- Each provider registers itself with the core's DSN registry in an `init`
  (`secretsx.Register("vault", ...)`), so `secretsx.Open("vault:...")` works
  after a blank import.
- Backends are testable without the real service: mock the HTTP API with
  `httptest` (Vault), or take a narrow client interface and pass a fake
  (`SecretsManagerAPI`, franz record mapping).

## Checks to run before finishing

```
make ci      # gofmt -w . && go vet ./... && go test ./... && go build ./...
```

If `go` is not on PATH (this machine builds Go in Docker), use:

```
docker run --rm -v "$PWD":/src -w /src golang:1.23-alpine sh -c \
  'go vet ./... && go test ./... && go build ./...'
```

## Out of scope

Don't add a web framework, a router, an ORM, a config-file loader, a DI
container, or a logging library. The point of this repo is that it has none of
those. If the user wants them, they compose them *around* these packages in
their own application.
