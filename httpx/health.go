package httpx

import (
	"context"
	"net/http"
)

// HealthCheck is a named readiness probe. Check returns a non-nil error when
// the dependency is unavailable.
type HealthCheck struct {
	Name  string
	Check func(context.Context) error
}

// Health returns a handler exposing two endpoints:
//
//	GET /healthz  — liveness: always 200 "ok" (is the process up?)
//	GET /readyz   — readiness: 200 with {"<name>":"ok",...} when every check
//	               passes, else 503 with the failing checks' error strings
//
// Mount it at the root of a dedicated mux, or on your main mux — the paths are
// specific enough not to collide.
func Health(checks ...HealthCheck) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		results := make(map[string]string, len(checks))
		healthy := true
		for _, c := range checks {
			if err := c.Check(r.Context()); err != nil {
				healthy = false
				results[c.Name] = err.Error()
			} else {
				results[c.Name] = "ok"
			}
		}
		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}
		JSON(w, status, results)
	})

	return mux
}
