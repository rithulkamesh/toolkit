// Package secretsx reads configuration secrets from one or more backends
// behind a single interface.
//
// A [Provider] resolves a logical key ("db/password") to a secret value. The
// standard library core ships three: [Env], [Dir] (one file per secret, as
// Kubernetes, Docker, and systemd mount them), and [JSONFile]. Vault, AWS
// Secrets Manager, and other backends live in nested modules that register
// themselves for [Open].
//
// [Chain] tries providers in order and returns the first hit, which is the
// idiomatic "real backend, then environment fallback":
//
//	store := secretsx.Chain{
//		mustVault, // from github.com/rithulkamesh/toolkit/secretsx/vault
//		secretsx.Env{Prefix: "APP_"},
//	}
//	pw, err := store.Get(ctx, "db/password")
//
// Wrap any provider in [NewCached] to avoid a network round-trip per read.
package secretsx

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ErrNotFound is returned (possibly wrapped) by a [Provider] that has no value
// for the requested key. [Chain] uses it to decide whether to try the next
// provider; any other error stops the chain.
var ErrNotFound = errors.New("secret not found")

// Provider resolves a logical key to a secret value.
//
// Get must return an error that satisfies errors.Is(err, [ErrNotFound]) when
// the key is simply absent, and a different error for transport or auth
// failures so callers can tell "no such secret" from "backend is down".
type Provider interface {
	Get(ctx context.Context, key string) (string, error)
}

// ProviderFunc adapts a function to [Provider].
type ProviderFunc func(ctx context.Context, key string) (string, error)

// Get calls f.
func (f ProviderFunc) Get(ctx context.Context, key string) (string, error) { return f(ctx, key) }

// Map is an in-memory [Provider], useful for tests and static configuration.
type Map map[string]string

// Get returns m[key], or [ErrNotFound].
func (m Map) Get(_ context.Context, key string) (string, error) {
	if v, ok := m[key]; ok {
		return v, nil
	}
	return "", notFound(key)
}

// Chain is a [Provider] that consults its members in order. It returns the
// first value found; it stops and returns early on any error that is not
// [ErrNotFound]. An exhausted chain returns [ErrNotFound].
type Chain []Provider

// Get implements [Provider].
func (c Chain) Get(ctx context.Context, key string) (string, error) {
	for _, p := range c {
		v, err := p.Get(ctx, key)
		switch {
		case err == nil:
			return v, nil
		case errors.Is(err, ErrNotFound):
			continue
		default:
			return "", err
		}
	}
	return "", notFound(key)
}

// Prefixed wraps a [Provider], prepending prefix to every key before the
// lookup. Compose namespaces without touching call sites.
func Prefixed(prefix string, p Provider) Provider {
	return ProviderFunc(func(ctx context.Context, key string) (string, error) {
		return p.Get(ctx, prefix+key)
	})
}

// MustGet returns the secret at key or panics. Call it only at startup, where
// a missing secret should stop the process.
func MustGet(ctx context.Context, p Provider, key string) string {
	v, err := p.Get(ctx, key)
	if err != nil {
		panic(fmt.Errorf("secretsx: %w", err))
	}
	return v
}

// --- DSN registry -----------------------------------------------------------

var (
	registryMu sync.RWMutex
	registry   = map[string]OpenFunc{}
)

// OpenFunc builds a [Provider] from the part of a DSN after the "scheme:"
// prefix (which may be empty).
type OpenFunc func(rest string) (Provider, error)

// Register makes scheme usable with [Open]. It panics on a duplicate or empty
// scheme, so call it from an init function. The core registers "env", "dir",
// and "json"; nested modules register "vault", "aws-sm", and so on.
func Register(scheme string, fn OpenFunc) {
	if scheme == "" || fn == nil {
		panic("secretsx: Register needs a non-empty scheme and function")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[scheme]; dup {
		panic("secretsx: scheme already registered: " + scheme)
	}
	registry[scheme] = fn
}

// Schemes lists the registered DSN schemes, sorted.
func Schemes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for s := range registry {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Open builds a [Provider] from a DSN of the form "scheme:rest", e.g.
// "env:APP_", "dir:/run/secrets", "vault:https://vault.internal:8200".
func Open(dsn string) (Provider, error) {
	scheme, rest, ok := strings.Cut(dsn, ":")
	if !ok {
		return nil, fmt.Errorf("secretsx: DSN %q has no scheme", dsn)
	}
	registryMu.RLock()
	fn, known := registry[scheme]
	registryMu.RUnlock()
	if !known {
		return nil, fmt.Errorf("secretsx: unknown DSN scheme %q (have %s)", scheme, strings.Join(Schemes(), ", "))
	}
	return fn(rest)
}

// OpenChain builds a [Chain] from several DSNs, in order.
func OpenChain(dsns ...string) (Chain, error) {
	out := make(Chain, 0, len(dsns))
	for _, d := range dsns {
		p, err := Open(d)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func notFound(key string) error {
	return fmt.Errorf("secretsx: %q: %w", key, ErrNotFound)
}
