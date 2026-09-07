package llmguard

import "testing"

func TestCheck(t *testing.T) {
	g := New("api.anthropic.com", "*.openai.azure.com")

	pass := []string{
		"https://api.anthropic.com/v1/messages",
		"api.anthropic.com",
		"api.anthropic.com:443",
		"https://acme.openai.azure.com/openai/deployments/x",
		"openai.azure.com",
	}
	for _, e := range pass {
		if err := g.Check(e); err != nil {
			t.Errorf("Check(%q) = %v, want nil", e, err)
		}
	}

	fail := []string{
		"https://evil.example/v1",
		"api.openai.com",
		"notopenai.azure.com.evil.com",
		"",
	}
	for _, e := range fail {
		if err := g.Check(e); err == nil {
			t.Errorf("Check(%q) = nil, want error", e)
		}
	}
}

func TestEmptyAllowlistDeniesAll(t *testing.T) {
	if err := New().Check("https://api.anthropic.com"); err == nil {
		t.Fatal("empty allowlist should deny everything")
	}
}

func TestAllowAll(t *testing.T) {
	if err := AllowAll().Check("https://anything.example"); err != nil {
		t.Fatalf("AllowAll should permit everything: %v", err)
	}
}

func TestFromEnvFallback(t *testing.T) {
	g := FromEnv("LLMGUARD_TEST_UNSET", "a.example, b.example")
	if err := g.Check("https://b.example/x"); err != nil {
		t.Fatalf("fallback not applied: %v", err)
	}
	if err := g.Check("https://c.example/x"); err == nil {
		t.Fatal("c.example should be rejected")
	}
}
