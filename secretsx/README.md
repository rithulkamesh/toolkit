# secretsx

`import "github.com/rithulkamesh/toolkit/secretsx"`

One interface over many secret backends.

```go
type Provider interface {
	Get(ctx context.Context, key string) (string, error)
}
```

A `Provider` maps a logical key (`"db/password"`) to a value, returning
`secretsx.ErrNotFound` when the key is simply absent (vs. a transport/auth
error). `Chain` tries providers in order — the idiomatic "real backend, then
environment fallback":

```go
store := secretsx.Chain{
	mustVault,                          // secretsx/vault
	secretsx.Env{Prefix: "APP_"},
}
pw, err := store.Get(ctx, "db/password")
```

| Provider | Backend | Module |
|---|---|---|
| `Env` | environment variables (`db/password` → `$APP_DB_PASSWORD`) | this one (stdlib) |
| `Dir` | one file per secret (Kubernetes / Docker / systemd mounts) | this one (stdlib) |
| `JSONFile` | a single JSON object of values | this one (stdlib) |
| `Map` | in-memory, for tests and static config | this one (stdlib) |
| `vault.Client` | HashiCorp Vault KV v2 | [`secretsx/vault`](./vault) |
| `awssm.SecretsManager` / `awssm.ParameterStore` | AWS | [`secretsx/awssm`](./awssm) |

Wrap any provider in `secretsx.NewCached(p, ttl)` to keep a hot path off the
network.

## DSN registry

Providers register a scheme in `init`, so a config string builds a chain:

```go
store, err := secretsx.OpenChain(
	"vault:https://vault.internal:8200", // needs a blank import of secretsx/vault
	"dir:/run/secrets",
	"env:APP_",
)
```
