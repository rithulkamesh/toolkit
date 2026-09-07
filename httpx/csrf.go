package httpx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRFConfig configures [RequireCSRF] and [IssueCSRFCookie].
type CSRFConfig struct {
	CookieName        string // default "csrf_token"
	HeaderName        string // default "X-CSRF-Token"
	CookiePath        string // default "/"
	Secure            bool   // set true when served over HTTPS
	SessionCookieName string // when set, CSRF is enforced only if this cookie is present
}

func (c CSRFConfig) withDefaults() CSRFConfig {
	if c.CookieName == "" {
		c.CookieName = "csrf_token"
	}
	if c.HeaderName == "" {
		c.HeaderName = "X-CSRF-Token"
	}
	if c.CookiePath == "" {
		c.CookiePath = "/"
	}
	return c
}

// CSRFToken returns a fresh 256-bit URL-safe random token.
func CSRFToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// IssueCSRFCookie sets the CSRF cookie (deliberately readable by JavaScript so
// the client can echo it back as a header) and returns the token. Call it right
// after establishing a session.
func IssueCSRFCookie(w http.ResponseWriter, cfg CSRFConfig) string {
	cfg = cfg.withDefaults()
	token := CSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     cfg.CookiePath,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: false, // must be readable by same-origin JS
	})
	return token
}

// RequireCSRF enforces the double-submit-cookie pattern on unsafe methods: the
// request must carry the CSRF cookie AND a header whose value matches it.
//
// It is defence-in-depth behind SameSite=Lax cookies. Exempt from the check:
// safe methods (GET/HEAD/OPTIONS/TRACE), any request carrying an Authorization
// header (not a browser relying on ambient cookies, so not a CSRF target), and
// — when SessionCookieName is configured — any request without that session
// cookie.
func RequireCSRF(cfg CSRFConfig) Middleware {
	cfg = cfg.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if safeMethod(r.Method) || r.Header.Get("Authorization") != "" {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.SessionCookieName != "" {
				if c, err := r.Cookie(cfg.SessionCookieName); err != nil || c.Value == "" {
					next.ServeHTTP(w, r)
					return
				}
			}
			cookie, err := r.Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				Error(w, http.StatusForbidden, "missing CSRF cookie", "csrf")
				return
			}
			header := r.Header.Get(cfg.HeaderName)
			if header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				Error(w, http.StatusForbidden, "CSRF token mismatch", "csrf")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}
