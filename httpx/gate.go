package httpx

import "net/http"

// Gate blocks a request until check(principal) returns true, responding with
// the given status and a JSON body carrying code and msg. It is the general
// form of the onboarding-precondition pattern: "email must be verified",
// "organization must be named", "plan must be active".
//
//	emailVerified := httpx.Gate(
//		http.StatusPreconditionRequired, "email_not_verified",
//		"verify your email address to continue",
//		func(p *httpx.Principal) bool { return p.Extra["email_verified"] == "true" },
//	)
//
// A request with no principal always gets 401 (wrap [Authenticate] first).
func Gate(status int, code, msg string, check func(*Principal) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
				return
			}
			if !check(p) {
				Error(w, status, msg, code)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
