package env

import (
	"testing"
	"time"
)

func TestString(t *testing.T) {
	t.Setenv("X_STR", "hello")
	if got := String("X_STR", "fb"); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got := String("X_MISSING", "fb"); got != "fb" {
		t.Fatalf("got %q", got)
	}
}

func TestInt(t *testing.T) {
	t.Setenv("X_INT", " 42 ")
	if got := Int("X_INT", 7); got != 42 {
		t.Fatalf("got %d", got)
	}
	t.Setenv("X_BAD", "notanint")
	if got := Int("X_BAD", 7); got != 7 {
		t.Fatalf("got %d", got)
	}
}

func TestBool(t *testing.T) {
	cases := map[string]bool{"1": true, "true": true, "ON": true, "0": false, "no": false}
	for in, want := range cases {
		t.Setenv("X_BOOL", in)
		if got := Bool("X_BOOL", !want); got != want {
			t.Fatalf("Bool(%q) = %v, want %v", in, got, want)
		}
	}
	t.Setenv("X_BOOL", "maybe")
	if !Bool("X_BOOL", true) {
		t.Fatal("unrecognised value should return fallback")
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("X_DUR", "1500ms")
	if got := Duration("X_DUR", time.Second); got != 1500*time.Millisecond {
		t.Fatalf("got %v", got)
	}
	if got := Duration("X_NOPE", time.Second); got != time.Second {
		t.Fatalf("got %v", got)
	}
}

func TestList(t *testing.T) {
	t.Setenv("X_LIST", " a , b ,,c ")
	got := List("X_LIST", nil)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("got %#v", got)
	}
	if got := List("X_EMPTY", []string{"fb"}); len(got) != 1 || got[0] != "fb" {
		t.Fatalf("got %#v", got)
	}
}

func TestRequired(t *testing.T) {
	if _, err := Required("X_ABSENT"); err == nil {
		t.Fatal("expected error")
	}
	t.Setenv("X_PRESENT", "v")
	if v, err := Required("X_PRESENT"); err != nil || v != "v" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}
