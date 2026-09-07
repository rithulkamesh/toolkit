package httpx

import (
	"net/http"
	"time"
)

// Timeout wraps handlers with http.TimeoutHandler: if a handler runs longer
// than d, the client receives 503 with msg as the body. Do not apply it to
// streaming routes (Server-Sent Events, long polls, downloads) — use
// [WriteDeadline] with a skip predicate there.
func Timeout(d time.Duration, msg string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, msg)
	}
}

// WriteDeadline sets a per-request write deadline on the underlying
// connection, bounding how long a slow or stalled client can hold it open.
// skip reports requests that must NOT get a deadline (typically SSE streams
// that legitimately stay open for minutes); pass nil to deadline everything.
func WriteDeadline(d time.Duration, skip func(*http.Request) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip == nil || !skip(r) {
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
			}
			next.ServeHTTP(w, r)
		})
	}
}
