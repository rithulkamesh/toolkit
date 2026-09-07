package httpx

import "net/http"

// RequireScope rejects a request whose principal lacks the exact scope:
// 401 when there is no principal, 403 when there is one but it is missing the
// scope.
func RequireScope(scope string) Middleware {
	return gateScopes(func(p *Principal) bool { return p.HasScope(scope) })
}

// RequireAnyScope passes when the principal holds at least one of scopes.
func RequireAnyScope(scopes ...string) Middleware {
	return gateScopes(func(p *Principal) bool {
		for _, s := range scopes {
			if p.HasScope(s) {
				return true
			}
		}
		return false
	})
}

// RequireAllScopes passes only when the principal holds every one of scopes.
func RequireAllScopes(scopes ...string) Middleware {
	return gateScopes(func(p *Principal) bool {
		for _, s := range scopes {
			if !p.HasScope(s) {
				return false
			}
		}
		return true
	})
}

func gateScopes(ok func(*Principal) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, found := PrincipalFrom(r.Context())
			if !found {
				Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			if !ok(p) {
				Error(w, http.StatusForbidden, "forbidden", "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
