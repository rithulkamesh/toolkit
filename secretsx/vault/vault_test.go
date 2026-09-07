package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rithulkamesh/toolkit/secretsx"
)

// fakeVault serves the two endpoints the client uses: KV v2 read and the
// Kubernetes login.
func fakeVault(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/secret/data/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		p := r.URL.Path[len("/v1/secret/data/"):]
		if p != "app/db" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data":     map[string]any{"password": "pw", "port": 5432},
				"metadata": map[string]any{"version": 3},
			},
		})
	})

	mux.HandleFunc("/v1/auth/kubernetes/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Role, JWT string }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Role != "my-app" || body.JWT != "sa-jwt-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"auth": map[string]any{"client_token": "s.k8s-issued", "lease_duration": 3600},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClientGet(t *testing.T) {
	srv := fakeVault(t)
	c, err := New(WithAddr(srv.URL), WithToken("s.root"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if v, err := c.Get(ctx, "app/db#password"); err != nil || v != "pw" {
		t.Fatalf("Get(app/db#password) = %q, %v", v, err)
	}
	if v, err := c.Get(ctx, "app/db#port"); err != nil || v != "5432" {
		t.Fatalf("Get(app/db#port) = %q, %v (non-string field as JSON)", v, err)
	}
	if v, err := c.Get(ctx, "app/db"); err != nil || v == "" {
		t.Fatalf("Get(app/db) whole-secret = %q, %v", v, err)
	}
	if _, err := c.Get(ctx, "app/db#nope"); !errors.Is(err, secretsx.ErrNotFound) {
		t.Fatalf("missing field: want ErrNotFound, got %v", err)
	}
	if _, err := c.Get(ctx, "app/missing#x"); !errors.Is(err, secretsx.ErrNotFound) {
		t.Fatalf("missing secret: want ErrNotFound, got %v", err)
	}
}

func TestKubernetesAuth(t *testing.T) {
	srv := fakeVault(t)

	jwt := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(jwt, []byte("sa-jwt-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := New(
		WithAddr(srv.URL),
		WithKubernetesAuth("my-app", WithK8sJWTPath(jwt)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := c.Get(context.Background(), "app/db#password"); err != nil || v != "pw" {
		t.Fatalf("Get after k8s login = %q, %v", v, err)
	}
}

func TestNewRequiresAddrAndToken(t *testing.T) {
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("HOME", t.TempDir()) // no ~/.vault-token

	if _, err := New(); err == nil {
		t.Fatal("New with no address should error")
	}
	if _, err := New(WithAddr("https://v:8200")); err == nil {
		t.Fatal("New with no token should error")
	}
}

func TestDSNRegistration(t *testing.T) {
	srv := fakeVault(t)
	t.Setenv("VAULT_TOKEN", "s.root")

	p, err := secretsx.Open("vault:" + srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := p.Get(context.Background(), "app/db#password"); err != nil || v != "pw" {
		t.Fatalf("via DSN: %q, %v", v, err)
	}
}
