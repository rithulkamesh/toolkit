// Package vault is a HashiCorp Vault KV v2 [secretsx.Provider], talking to
// Vault's HTTP API directly with the standard library — no Vault SDK.
//
//	cli, err := vault.New(
//		vault.WithAddr("https://vault.internal:8200"),
//		vault.WithKubernetesAuth("my-app"),
//	)
//	store := secretsx.Chain{cli, secretsx.Env{Prefix: "APP_"}}
//
// A key is "path#field": the KV v2 secret at path, then one field out of it.
// Omit "#field" (or end the key with "#") to get the whole secret as a JSON
// object. "path" may contain slashes for nested KV v2 paths.
package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rithulkamesh/toolkit/secretsx"
)

// DefaultKubernetesJWTPath is where a pod's ServiceAccount token is mounted.
const DefaultKubernetesJWTPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// Client reads secrets from one Vault server. It implements [secretsx.Provider]
// and is safe for concurrent use.
type Client struct {
	addr      string
	mount     string
	namespace string
	tokens    TokenSource
	http      *http.Client
}

// Option configures [New].
type Option func(*Client)

// WithAddr sets the Vault address ("https://host:8200"). Defaults to $VAULT_ADDR.
func WithAddr(addr string) Option { return func(c *Client) { c.addr = strings.TrimRight(addr, "/") } }

// WithMount sets the KV v2 mount path. Defaults to "secret".
func WithMount(m string) Option { return func(c *Client) { c.mount = strings.Trim(m, "/") } }

// WithNamespace sets the Vault Enterprise namespace (X-Vault-Namespace).
// Defaults to $VAULT_NAMESPACE.
func WithNamespace(ns string) Option { return func(c *Client) { c.namespace = ns } }

// WithToken authenticates with a fixed token.
func WithToken(token string) Option {
	return func(c *Client) { c.tokens = staticToken(token) }
}

// WithTokenFile reads the token from path on every request, so an external
// agent can rotate it in place.
func WithTokenFile(path string) Option {
	return func(c *Client) { c.tokens = &fileToken{path: path} }
}

// WithKubernetesAuth logs in via the Kubernetes auth method using the pod's
// ServiceAccount JWT, and refreshes the token before its lease expires.
func WithKubernetesAuth(role string, opts ...K8sOption) Option {
	return func(c *Client) {
		k := &k8sAuth{role: role, mount: "kubernetes", jwtPath: DefaultKubernetesJWTPath, client: c}
		for _, o := range opts {
			o(k)
		}
		c.tokens = k
	}
}

// K8sOption tunes [WithKubernetesAuth].
type K8sOption func(*k8sAuth)

// WithK8sMount overrides the auth mount (default "kubernetes").
func WithK8sMount(m string) K8sOption { return func(k *k8sAuth) { k.mount = strings.Trim(m, "/") } }

// WithK8sJWTPath overrides the ServiceAccount token path.
func WithK8sJWTPath(p string) K8sOption { return func(k *k8sAuth) { k.jwtPath = p } }

// WithHTTPClient sets the HTTP client (for timeouts, custom TLS, proxies).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// New builds a [Client]. It reads $VAULT_ADDR, $VAULT_NAMESPACE, and — when no
// auth option is given — $VAULT_TOKEN then ~/.vault-token.
func New(opts ...Option) (*Client, error) {
	c := &Client{
		addr:      strings.TrimRight(os.Getenv("VAULT_ADDR"), "/"),
		mount:     "secret",
		namespace: os.Getenv("VAULT_NAMESPACE"),
		http:      &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	if c.addr == "" {
		return nil, fmt.Errorf("vault: no address (set VAULT_ADDR or WithAddr)")
	}
	if c.tokens == nil {
		if t := os.Getenv("VAULT_TOKEN"); t != "" {
			c.tokens = staticToken(t)
		} else if home, err := os.UserHomeDir(); err == nil {
			if p := filepath.Join(home, ".vault-token"); fileExists(p) {
				c.tokens = &fileToken{path: p}
			}
		}
	}
	if c.tokens == nil {
		return nil, fmt.Errorf("vault: no token (set VAULT_TOKEN, WithToken, WithTokenFile, or WithKubernetesAuth)")
	}
	return c, nil
}

// Get implements [secretsx.Provider].
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	secretPath, field, _ := strings.Cut(key, "#")
	secretPath = strings.Trim(secretPath, "/")
	if secretPath == "" {
		return "", fmt.Errorf("vault: empty secret path in key %q", key)
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("vault: auth: %w", err)
	}

	url := fmt.Sprintf("%s/v1/%s/data/%s", c.addr, c.mount, secretPath)
	var body struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	status, err := c.do(ctx, http.MethodGet, url, token, nil, &body)
	if err != nil {
		return "", err
	}
	if status == http.StatusNotFound {
		return "", fmt.Errorf("vault: %s/%s: %w", c.mount, secretPath, secretsx.ErrNotFound)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("vault: read %s/%s: HTTP %d", c.mount, secretPath, status)
	}

	if field == "" {
		enc, _ := json.Marshal(body.Data.Data)
		return string(enc), nil
	}
	v, ok := body.Data.Data[field]
	if !ok {
		return "", fmt.Errorf("vault: %s/%s has no field %q: %w", c.mount, secretPath, field, secretsx.ErrNotFound)
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	enc, _ := json.Marshal(v)
	return string(enc), nil
}

// do issues one request, decoding a JSON response into out (when non-nil and
// the status is 2xx). It returns the status code; a 404 is not an error here.
func (c *Client) do(ctx context.Context, method, url, token string, reqBody, out any) (int, error) {
	var rdr io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Vault-Namespace", c.namespace)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("vault: %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
		return resp.StatusCode, fmt.Errorf("vault: %s %s: HTTP %d: %s", method, url, resp.StatusCode, bytes.TrimSpace(msg))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("vault: decode %s response: %w", url, err)
		}
	} else {
		io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode, nil
}

// --- token sources --------------------------------------------------------

// TokenSource yields a Vault token, refreshing it as needed.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type staticToken string

func (s staticToken) Token(context.Context) (string, error) { return string(s), nil }

type fileToken struct {
	path string
}

func (f *fileToken) Token(context.Context) (string, error) {
	b, err := os.ReadFile(f.path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

type k8sAuth struct {
	role    string
	mount   string
	jwtPath string
	client  *Client

	mu      sync.Mutex
	token   string
	renewAt time.Time
}

func (k *k8sAuth) Token(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token != "" && time.Now().Before(k.renewAt) {
		return k.token, nil
	}

	jwt, err := os.ReadFile(k.jwtPath)
	if err != nil {
		return "", fmt.Errorf("read ServiceAccount token: %w", err)
	}

	url := fmt.Sprintf("%s/v1/auth/%s/login", k.client.addr, k.mount)
	req := map[string]string{"role": k.role, "jwt": strings.TrimSpace(string(jwt))}
	var out struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	status, err := k.client.do(ctx, http.MethodPost, url, "", req, &out)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK || out.Auth.ClientToken == "" {
		return "", fmt.Errorf("vault: kubernetes login failed (HTTP %d)", status)
	}

	k.token = out.Auth.ClientToken
	ttl := time.Duration(out.Auth.LeaseDuration) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	// Renew at two-thirds of the lease.
	k.renewAt = time.Now().Add(ttl - ttl/3)
	return k.token, nil
}

func init() {
	// "vault:https://host:8200" — token/namespace from the environment.
	secretsx.Register("vault", func(rest string) (secretsx.Provider, error) {
		var opts []Option
		if rest != "" {
			opts = append(opts, WithAddr(rest))
		}
		return New(opts...)
	})
}
