package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader is the header carrying the request ID, both inbound
// (propagated from an upstream proxy or client) and outbound.
const RequestIDHeader = "X-Request-Id"

// RequestID ensures every request has an ID: it reuses an inbound
// X-Request-Id when present and otherwise generates a random 128-bit one. The
// ID is echoed in the response headers and stored in the request context
// (read it with [RequestIDFrom]).
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if id == "" {
				var b [16]byte
				_, _ = rand.Read(b[:])
				id = hex.EncodeToString(b[:])
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
		})
	}
}
