package secretsx

import (
	"context"
	"os"
	"strings"
)

// Env is a [Provider] backed by environment variables.
//
// A logical key is upper-cased and every run of non-alphanumeric characters
// becomes a single underscore, so "db/password" reads $DB_PASSWORD. Set
// [Env.Prefix] to namespace ("APP_" -> $APP_DB_PASSWORD), or [Env.KeyFunc] to
// override the mapping entirely. A variable that is unset — or set but empty,
// unless [Env.AllowEmpty] is true — reports [ErrNotFound].
type Env struct {
	Prefix     string
	KeyFunc    func(key string) string
	AllowEmpty bool
}

// Get implements [Provider].
func (e Env) Get(_ context.Context, key string) (string, error) {
	name := e.Prefix + e.keyFunc()(key)
	v, ok := os.LookupEnv(name)
	if !ok || (v == "" && !e.AllowEmpty) {
		return "", notFoundEnv(name)
	}
	return v, nil
}

func (e Env) keyFunc() func(string) string {
	if e.KeyFunc != nil {
		return e.KeyFunc
	}
	return DefaultEnvKey
}

// DefaultEnvKey maps a logical key to an environment-variable name: upper-case,
// with each maximal run of characters outside [A-Za-z0-9] collapsed to one
// underscore and trimmed from the ends. "db.pool/max-size" -> "DB_POOL_MAX_SIZE".
func DefaultEnvKey(key string) string {
	var b strings.Builder
	b.Grow(len(key))
	prevUnderscore := false
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32)
			prevUnderscore = false
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func notFoundEnv(name string) error {
	return notFound("$" + name)
}

func init() {
	// "env:" and "env:PREFIX_"
	Register("env", func(rest string) (Provider, error) {
		return Env{Prefix: rest}, nil
	})
}
