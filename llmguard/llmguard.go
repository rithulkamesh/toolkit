// Package llmguard enforces an outbound host allowlist for calls to LLM
// providers and other third-party APIs.
//
// Route every outbound request through [Guard.Check] first. A prompt-injection
// payload or a tampered config that swaps in an attacker-controlled endpoint
// then fails closed instead of exfiltrating data or keys.
//
//	guard := llmguard.FromEnv("LLM_ALLOWED_HOSTS",
//		"api.anthropic.com,api.openai.com,*.openai.azure.com")
//
//	if err := guard.Check(endpoint); err != nil {
//		return err
//	}
//	resp, err := http.Post(endpoint, "application/json", body)
package llmguard

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Guard holds a host allowlist.
type Guard struct {
	hosts    []string
	allowAll bool
}

// New builds a Guard from the given hosts. Each host is either exact
// ("api.openai.com") or a wildcard suffix ("*.openai.azure.com", which matches
// that domain and any subdomain). With no hosts, every [Guard.Check] fails;
// call [AllowAll] to opt out explicitly.
func New(hosts ...string) *Guard {
	g := &Guard{}
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			g.hosts = append(g.hosts, h)
		}
	}
	return g
}

// AllowAll returns a Guard that permits every endpoint. Use it only in local
// development.
func AllowAll() *Guard { return &Guard{allowAll: true} }

// FromEnv reads a comma-separated allowlist from the named environment
// variable, falling back to fallback (also comma-separated) when the variable
// is unset or blank.
func FromEnv(key, fallback string) *Guard {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	return New(strings.Split(raw, ",")...)
}

// Hosts returns the configured allowlist (nil for an AllowAll guard).
func (g *Guard) Hosts() []string { return g.hosts }

// Check returns nil when endpoint's host is on the allowlist, and a
// descriptive error otherwise. endpoint may be a full URL
// ("https://host/path") or a bare "host" / "host:port".
func (g *Guard) Check(endpoint string) error {
	if g.allowAll {
		return nil
	}
	host := extractHost(endpoint)
	if host == "" {
		return fmt.Errorf("llmguard: cannot determine host from %q", endpoint)
	}
	for _, h := range g.hosts {
		if matchHost(h, host) {
			return nil
		}
	}
	return fmt.Errorf("llmguard: host %q is not on the allowlist", host)
}

// CheckURL is [Guard.Check] for an already-parsed URL.
func (g *Guard) CheckURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("llmguard: nil URL")
	}
	return g.Check(u.Host)
}

func extractHost(endpoint string) string {
	s := strings.ToLower(strings.TrimSpace(endpoint))
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			s = u.Host
		}
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return s
}

func matchHost(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if suffix, ok := strings.CutPrefix(pattern, "*."); ok {
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return false
}
