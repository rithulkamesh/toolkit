package httpx

import "context"

type ctxKey int

const (
	principalKey ctxKey = iota
	requestIDKey
)

// WithPrincipal returns a copy of ctx carrying p. Handler tests can use it to
// inject a principal without running the auth middleware.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom returns the principal attached by [Authenticate] or
// [OptionalAuth], if any.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey).(*Principal)
	return p, ok
}

// RequestIDFrom returns the ID attached by [RequestID], or "".
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}
