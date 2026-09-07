package secretsx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultEnvKey(t *testing.T) {
	cases := map[string]string{
		"db/password":        "DB_PASSWORD",
		"db.pool/max-size":   "DB_POOL_MAX_SIZE",
		"already_ok":         "ALREADY_OK",
		"/leading/trailing/": "LEADING_TRAILING",
		"a--b__c":            "A_B_C",
		"lowerUPPER":         "LOWERUPPER",
	}
	for in, want := range cases {
		if got := DefaultEnvKey(in); got != want {
			t.Errorf("DefaultEnvKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnvProvider(t *testing.T) {
	t.Setenv("APP_DB_PASSWORD", "hunter2")
	t.Setenv("APP_EMPTY", "")

	e := Env{Prefix: "APP_"}
	if v, err := e.Get(context.Background(), "db/password"); err != nil || v != "hunter2" {
		t.Fatalf("Get(db/password) = %q, %v", v, err)
	}
	if _, err := e.Get(context.Background(), "empty"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty var: want ErrNotFound, got %v", err)
	}
	if _, err := (Env{Prefix: "APP_", AllowEmpty: true}).Get(context.Background(), "empty"); err != nil {
		t.Fatalf("AllowEmpty: unexpected error %v", err)
	}
	if _, err := e.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing var: want ErrNotFound, got %v", err)
	}
}

func TestChain(t *testing.T) {
	boom := errors.New("backend down")
	ctx := context.Background()

	got, err := Chain{Map{}, Map{"k": "v"}}.Get(ctx, "k")
	if err != nil || got != "v" {
		t.Fatalf("chain hit second: %q, %v", got, err)
	}

	_, err = Chain{Map{}, Map{}}.Get(ctx, "k")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("exhausted chain: want ErrNotFound, got %v", err)
	}

	failing := ProviderFunc(func(context.Context, string) (string, error) { return "", boom })
	_, err = Chain{failing, Map{"k": "v"}}.Get(ctx, "k")
	if !errors.Is(err, boom) {
		t.Fatalf("hard error should stop the chain, got %v", err)
	}
}

func TestPrefixed(t *testing.T) {
	p := Prefixed("svc/", Map{"svc/token": "abc"})
	if v, err := p.Get(context.Background(), "token"); err != nil || v != "abc" {
		t.Fatalf("Prefixed Get = %q, %v", v, err)
	}
}

func TestDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "token"), []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "db")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "password"), []byte("raw-no-newline"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := Dir{Root: root}
	if v, err := d.Get(context.Background(), "token"); err != nil || v != "s3cr3t" {
		t.Fatalf("Get(token) = %q, %v (want trailing newline trimmed)", v, err)
	}
	if v, err := d.Get(context.Background(), "db/password"); err != nil || v != "raw-no-newline" {
		t.Fatalf("Get(db/password) = %q, %v", v, err)
	}
	if v, err := (Dir{Root: root, Raw: true}).Get(context.Background(), "token"); err != nil || v != "s3cr3t\n" {
		t.Fatalf("Raw Get(token) = %q, %v", v, err)
	}
	if _, err := d.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: want ErrNotFound, got %v", err)
	}
	// Path traversal must not escape root.
	if _, err := d.Get(context.Background(), "../../etc/hostname"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal: want confinement to root (ErrNotFound), got %v", err)
	}
}

func TestJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	body := `{"db":{"password":"pw","port":5432},"api_key":"ak"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	j := &JSONFile{Path: path}
	if v, err := j.Get(context.Background(), "db/password"); err != nil || v != "pw" {
		t.Fatalf("Get(db/password) = %q, %v", v, err)
	}
	if v, err := j.Get(context.Background(), "db/port"); err != nil || v != "5432" {
		t.Fatalf("Get(db/port) = %q, %v (non-string leaf as JSON)", v, err)
	}
	if v, err := j.Get(context.Background(), "api_key"); err != nil || v != "ak" {
		t.Fatalf("Get(api_key) = %q, %v", v, err)
	}
	if _, err := j.Get(context.Background(), "db/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: want ErrNotFound, got %v", err)
	}
}

func TestCached(t *testing.T) {
	var calls int
	now := time.Unix(0, 0)
	src := ProviderFunc(func(_ context.Context, key string) (string, error) {
		calls++
		if key == "gone" {
			return "", ErrNotFound
		}
		return "v", nil
	})
	c := &Cached{Provider: src, TTL: time.Minute, NegativeTTL: 10 * time.Second, now: func() time.Time { return now }}

	for i := 0; i < 3; i++ {
		if v, err := c.Get(context.Background(), "k"); err != nil || v != "v" {
			t.Fatalf("Get(k) = %q, %v", v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 backend call, got %d", calls)
	}

	now = now.Add(2 * time.Minute) // past TTL
	if _, err := c.Get(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected refetch after TTL, calls=%d", calls)
	}

	calls = 0
	for i := 0; i < 3; i++ {
		if _, err := c.Get(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("negative cache: expected 1 call, got %d", calls)
	}

	c.Invalidate("k")
	calls = 0
	if _, err := c.Get(context.Background(), "k"); err != nil || calls != 1 {
		t.Fatalf("after Invalidate: err=%v calls=%d", err, calls)
	}
}

func TestOpenChain(t *testing.T) {
	t.Setenv("MYAPP_OTHER", "from-env")
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "token"), []byte("from-dir"), 0o600)

	ch, err := OpenChain("dir:"+root, "env:MYAPP_")
	if err != nil {
		t.Fatal(err)
	}
	if v, err := ch.Get(context.Background(), "token"); err != nil || v != "from-dir" {
		t.Fatalf("OpenChain Get(token) = %q, %v", v, err)
	}
	if v, err := ch.Get(context.Background(), "other"); err != nil || v != "from-env" {
		t.Fatalf("fallthrough to env: %q, %v", v, err)
	}
	if _, err := Open("bogus:x"); err == nil {
		t.Fatal("Open with unknown scheme should error")
	}
}
