package httpx

import "net/http"

// Resolver turns an inbound request into a [Principal].
//
//   - Return (principal, nil) on success.
//   - Return (nil, nil) when no credentials were presented — [Authenticate]
//     treats this as 401, [OptionalAuth] lets the request through anonymously.
//   - Return (nil, err) for credentials that were presented but rejected
//     (expired token, bad signature, unknown key).
type Resolver interface {
	Resolve(r *http.Request) (*Principal, error)
}

// ResolverFunc adapts a plain function to [Resolver].
type ResolverFunc func(r *http.Request) (*Principal, error)

// Resolve implements [Resolver].
func (f ResolverFunc) Resolve(r *http.Request) (*Principal, error) { return f(r) }

// Authenticate resolves a principal and attaches it to the request context.
// A resolver error, or a nil principal, results in 401. Use [OptionalAuth] for
// endpoints that also serve anonymous callers.
func Authenticate(resolver Resolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := resolver.Resolve(r)
			if err != nil || p == nil {
				Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// OptionalAuth attaches a principal when the resolver returns one and otherwise
// passes the request through unauthenticated. Resolver errors are swallowed
// (the request proceeds as anonymous).
func OptionalAuth(resolver Resolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p, err := resolver.Resolve(r); err == nil && p != nil {
				r = r.WithContext(WithPrincipal(r.Context(), p))
			}
			next.ServeHTTP(w, r)
		})
	}
}
