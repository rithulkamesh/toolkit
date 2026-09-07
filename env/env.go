// Package env reads environment variables with typed fallbacks.
//
// Every getter takes a fallback and never fails: an unset, empty, or
// unparseable value yields the fallback. Reach for [Required] or [MustString]
// at startup when a value has no safe default and the process should not run
// without it.
package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// String returns the value of key, or fallback when unset/empty.
func String(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Int returns key parsed as a base-10 int, or fallback when unset/empty or
// not a valid integer.
func Int(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

// Bool returns key parsed as a boolean, or fallback when unset/empty or
// unrecognised. Accepted (case-insensitive): 1/t/true/yes/on and
// 0/f/false/no/off.
func Bool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "t", "true", "yes", "on":
		return true
	case "0", "f", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// Duration returns key parsed by time.ParseDuration (e.g. "500ms", "30s",
// "2h45m"), or fallback when unset/empty or invalid.
func Duration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			return d
		}
	}
	return fallback
}

// List splits key on commas, trims each element, and drops empties. Returns
// fallback when the result would be empty.
func List(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	out := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// Required returns key's value, or an error when it is unset/empty.
func Required(key string) (string, error) {
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("env: required variable %q is not set", key)
}

// MustString returns key's value or panics. Call it only during process
// startup, where a panic is the intended outcome for missing config.
func MustString(key string) string {
	v, err := Required(key)
	if err != nil {
		panic(err)
	}
	return v
}
