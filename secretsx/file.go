package secretsx

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Dir is a [Provider] that reads one file per secret from a directory, the
// shape Kubernetes (projected secret volumes), Docker (/run/secrets), and
// systemd ($CREDENTIALS_DIRECTORY) all use.
//
// The key is the file name relative to [Dir.Root]; "" for Root defaults to
// $CREDENTIALS_DIRECTORY when set, otherwise "/run/secrets". Keys are cleaned
// and confined to Root, so "../etc/passwd" cannot escape. A single trailing
// newline is trimmed unless [Dir.Raw] is set.
type Dir struct {
	Root string
	Raw  bool
}

// Get implements [Provider].
func (d Dir) Get(_ context.Context, key string) (string, error) {
	root := d.Root
	if root == "" {
		if root = os.Getenv("CREDENTIALS_DIRECTORY"); root == "" {
			root = "/run/secrets"
		}
	}
	// Confine key to root: clean it as an absolute path (drops "..") then
	// re-root it.
	rel := filepath.Clean(string(filepath.Separator) + filepath.FromSlash(key))
	full := filepath.Join(root, rel)

	b, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return "", notFound(key)
	}
	if err != nil {
		return "", err
	}
	s := string(b)
	if !d.Raw {
		s = strings.TrimSuffix(strings.TrimSuffix(s, "\n"), "\r")
	}
	return s, nil
}

// JSONFile is a [Provider] backed by a single JSON object of string values,
// e.g. a mounted secret bundle. Nested keys are addressed with "/"
// ({"db":{"password":"x"}} -> "db/password"). Non-string leaves are returned
// as their compact JSON encoding. The file is read once and cached; call
// [JSONFile.Reload] after it changes.
type JSONFile struct {
	Path string

	once sync.Once
	mu   sync.RWMutex
	data map[string]any
	err  error
}

// Get implements [Provider].
func (j *JSONFile) Get(_ context.Context, key string) (string, error) {
	j.once.Do(j.load)
	j.mu.RLock()
	data, loadErr := j.data, j.err
	j.mu.RUnlock()
	if loadErr != nil {
		return "", loadErr
	}

	var cur any = data
	for _, seg := range strings.Split(key, "/") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", notFound(key)
		}
		cur, ok = obj[seg]
		if !ok {
			return "", notFound(key)
		}
	}
	if s, ok := cur.(string); ok {
		return s, nil
	}
	enc, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(enc), nil
}

// Reload re-reads the file on the next [JSONFile.Get].
func (j *JSONFile) Reload() {
	j.mu.Lock()
	j.data, j.err = nil, nil
	j.once = sync.Once{}
	j.mu.Unlock()
}

func (j *JSONFile) load() {
	b, err := os.ReadFile(j.Path)
	var data map[string]any
	if err == nil {
		err = json.Unmarshal(b, &data)
	}
	j.mu.Lock()
	j.data, j.err = data, err
	j.mu.Unlock()
}

func init() {
	Register("dir", func(rest string) (Provider, error) {
		return Dir{Root: rest}, nil
	})
	Register("json", func(rest string) (Provider, error) {
		return &JSONFile{Path: rest}, nil
	})
}
