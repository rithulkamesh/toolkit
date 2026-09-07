package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestAuthenticate(t *testing.T) {
	resolver := ResolverFunc(func(r *http.Request) (*Principal, error) {
		if r.Header.Get("Authorization") == "Bearer good" {
			return &Principal{Subject: "u1", Scopes: []string{"read"}}, nil
		}
		return nil, nil
	})
	h := Authenticate(resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || p.Subject != "u1" {
			t.Error("principal not in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("good token: got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: got %d", rec.Code)
	}
}

func TestRequireScope(t *testing.T) {
	base := okHandler()
	withPrincipal := func(p *Principal) *http.Request {
		return httptest.NewRequest("GET", "/", nil).WithContext(WithPrincipal(context.Background(), p))
	}

	h := RequireScope("workflows:write")(base)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withPrincipal(&Principal{Scopes: []string{"workflows:write"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("has scope: got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withPrincipal(&Principal{Scopes: []string{"workflows:read"}}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing scope: got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no principal: got %d", rec.Code)
	}
}

func TestRequireLiveKey(t *testing.T) {
	h := RequireLiveKey()(okHandler())
	req := httptest.NewRequest("POST", "/billing", nil).
		WithContext(WithPrincipal(context.Background(), &Principal{TokenKind: TokenTestKey}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("test key: got %d", rec.Code)
	}

	req = httptest.NewRequest("POST", "/billing", nil).
		WithContext(WithPrincipal(context.Background(), &Principal{TokenKind: TokenLiveKey}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("live key: got %d", rec.Code)
	}
}

func TestRequireCSRF(t *testing.T) {
	h := RequireCSRF(CSRFConfig{})(okHandler())

	// GET is exempt.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET exempt: got %d", rec.Code)
	}

	// POST with matching cookie + header passes.
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc123"})
	req.Header.Set("X-CSRF-Token", "abc123")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("matching token: got %d", rec.Code)
	}

	// POST with mismatch is rejected.
	req = httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc123"})
	req.Header.Set("X-CSRF-Token", "nope")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mismatch: got %d", rec.Code)
	}

	// POST with an Authorization header is exempt (not cookie-auth).
	req = httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer exempt: got %d", rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	store := NewMemoryStore(context.Background())
	h := RateLimit(store, RateLimitConfig{Limit: 2, Window: time.Minute, Prefix: "t"})(okHandler())

	do := func() int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do() != 200 || do() != 200 {
		t.Fatal("first two requests should pass")
	}
	if do() != http.StatusTooManyRequests {
		t.Fatal("third request should be limited")
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.9:5555"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2, 3.3.3.3")
	if got := ClientIP(req); got != "3.3.3.3" {
		t.Fatalf("XFF last hop: got %q", got)
	}
	if got := RemoteIP(req); got != "10.0.0.9" {
		t.Fatalf("RemoteIP: got %q", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := CORS(CORSConfig{AllowedOrigins: []string{"https://app.example"}})(okHandler())
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status: got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("allow-origin: got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestChainOrder(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(mk("a"), mk("b"), mk("c"))(okHandler()).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("order: %v", order)
	}
}

func TestHealth(t *testing.T) {
	h := Health(HealthCheck{Name: "db", Check: func(context.Context) error { return nil }})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz: %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz ok: got %d", rec.Code)
	}
}

func TestGate(t *testing.T) {
	h := Gate(http.StatusPreconditionRequired, "email_not_verified", "verify email",
		func(p *Principal) bool { return p.Extra["verified"] == "true" })(okHandler())

	req := httptest.NewRequest("GET", "/", nil).
		WithContext(WithPrincipal(context.Background(), &Principal{Extra: map[string]string{"verified": "false"}}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("unverified: got %d", rec.Code)
	}
}
