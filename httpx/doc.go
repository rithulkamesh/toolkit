// Package httpx is a set of composable net/http middleware for building a JSON
// API gateway: authentication, scope checks, CSRF, rate limiting, CORS,
// timeouts, request IDs, access logging, and health endpoints.
//
// Every middleware has the shape func(http.Handler) http.Handler (aliased as
// [Middleware]) and is combined with [Chain]:
//
//	stack := httpx.Chain(
//		httpx.RequestID(),
//		httpx.AccessLog(logger),
//		httpx.CORS(corsCfg),
//		httpx.Authenticate(resolver),
//	)
//	http.ListenAndServe(addr, stack(mux))
//
// The package is deliberately unopinionated about *how* you authenticate. You
// implement [Resolver] (look up a session cookie, verify a JWT, check an API
// key — whatever you do) and return a [Principal]; the rest of the middleware
// reads that principal from the request context.
//
// There are no external dependencies.
package httpx
