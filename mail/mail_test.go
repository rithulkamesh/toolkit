package mail

import "testing"

func TestBareAddress(t *testing.T) {
	cases := map[string]string{
		"Acme <no-reply@acme.example>": "no-reply@acme.example",
		"plain@acme.example":           "plain@acme.example",
		"<x@y>":                        "x@y",
	}
	for in, want := range cases {
		if got := bareAddress(in); got != want {
			t.Errorf("bareAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogSender(t *testing.T) {
	var s LogSender
	var _ Sender = &s // compile-time interface check

	if err := s.Send("a@x", "hi", "<p>hi</p>"); err != nil {
		t.Fatal(err)
	}
	if len(s.Sent) != 1 || s.Sent[0].To != "a@x" || s.Sent[0].Subject != "hi" {
		t.Fatalf("unexpected: %#v", s.Sent)
	}
}
