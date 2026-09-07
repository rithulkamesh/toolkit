package httpx

import "net/http"

// RequireLiveKey blocks any request authenticated with a [TokenTestKey],
// regardless of the scopes that key was granted. Wrap routes whose effects are
// irreversible or financial — billing changes, data export, tenant deletion —
// so a sandbox key can exercise the API safely but can never trigger them.
func RequireLiveKey() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p, ok := PrincipalFrom(r.Context()); ok && p.TokenKind == TokenTestKey {
				Error(w, http.StatusForbidden, "test API keys cannot access this endpoint", "live_key_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
