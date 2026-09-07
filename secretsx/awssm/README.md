# secretsx/awssm

`import "github.com/rithulkamesh/toolkit/secretsx/awssm"`

AWS Secrets Manager and SSM Parameter Store as [`secretsx.Provider`](..)s.
Uses `aws-sdk-go-v2` (own `go.mod`).

```go
sm, err := awssm.NewSecretsManager(ctx)          // default AWS config chain
store := secretsx.Chain{sm, secretsx.Env{Prefix: "APP_"}}

key, err := store.Get(ctx, "prod/api-key")       // raw secret string
pw, err := store.Get(ctx, "prod/db#password")    // secret is JSON, take one field
ssl, err := store.Get(ctx, "prod/db#opts/ssl")   // nested field
```

`NewParameterStore` does the same over SSM, always `WithDecryption`. Both take
a `Prefix` and accept a fake client (`SecretsManagerAPI` / `ParameterStoreAPI`)
for tests.

DSN: `secretsx.Open("aws-sm:us-east-1")` / `secretsx.Open("aws-ssm:")` after a
blank import.
