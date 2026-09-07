package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the caller's IP address, trusting a reverse proxy in front
// of the service.
//
// When X-Forwarded-For is present, the real client is its LAST entry: a proxy
// or load balancer appends the connecting peer, while any earlier entries were
// supplied by the client and are forgeable. Falls back to the request's
// RemoteAddr (port stripped).
//
// Use [RemoteIP] instead when the service terminates client connections
// directly with no trusted proxy — there, X-Forwarded-For is fully
// attacker-controlled.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	return RemoteIP(r)
}

// RemoteIP returns the connection peer address from RemoteAddr, with any port
// removed. It never consults request headers.
func RemoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
