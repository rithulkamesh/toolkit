package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig configures [CORS].
type CORSConfig struct {
	AllowedOrigins   []string // exact origins, or a single "*" for any
	AllowedMethods   []string // default: GET, POST, PUT, PATCH, DELETE, OPTIONS
	AllowedHeaders   []string // default: Content-Type, Authorization, X-CSRF-Token
	ExposedHeaders   []string // response headers the browser may read
	AllowCredentials bool     // send Access-Control-Allow-Credentials: true
	MaxAge           int      // preflight cache seconds (default 600)
}

// CORS answers preflight (OPTIONS) requests and adds the matching CORS
// response headers to actual requests. An origin not on the allowlist simply
// receives no CORS headers (the browser then blocks the response).
//
// Note: "*" origins and AllowCredentials are mutually exclusive per the Fetch
// spec; when both are set this echoes the request origin instead of "*".
func CORS(cfg CORSConfig) Middleware {
	if len(cfg.AllowedMethods) == 0 {
		cfg.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(cfg.AllowedHeaders) == 0 {
		cfg.AllowedHeaders = []string{"Content-Type", "Authorization", "X-CSRF-Token"}
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 600
	}

	wildcard := false
	allowed := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (wildcard || allowed[origin]) {
				if wildcard && !cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if len(cfg.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
				}
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
