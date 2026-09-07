# secretsx/vault

`import "github.com/rithulkamesh/toolkit/secretsx/vault"`

A HashiCorp Vault **KV v2** [`secretsx.Provider`](..), talking to Vault's HTTP
API directly with the standard library — no Vault SDK.

```go
cli, err := vault.New(
	vault.WithAddr("https://vault.internal:8200"), // or $VAULT_ADDR
	vault.WithKubernetesAuth("my-app"),            // pod ServiceAccount JWT
)
store := secretsx.Chain{cli, secretsx.Env{Prefix: "APP_"}}

pw, err := store.Get(ctx, "app/db#password")  // KV v2 secret "app/db", field "password"
all, err := store.Get(ctx, "app/db#")         // the whole secret as a JSON object
```

Auth options: `WithToken`, `WithTokenFile` (re-read per request for rotation),
`WithKubernetesAuth` (logs in, refreshes before the lease expires). With none
given, `New` uses `$VAULT_TOKEN` then `~/.vault-token`.

DSN: `secretsx.Open("vault:https://vault.internal:8200")` after a blank import.
