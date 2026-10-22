package llm

import (
	"fmt"
	"testing"
)

// stubLookPathNoClaude replaces lookPath for tests that should not find the claude binary.
func stubLookPathNoClaude(t *testing.T) {
	t.Helper()
	t.Setenv(envProvider, "")
	orig := lookPath
	lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "", fmt.Errorf("not found")
		}
		return orig(name)
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestConfigFromEnv_Claude(t *testing.T) {
	orig := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = orig })
	t.Setenv(envProvider, "")
	t.Setenv(envOllamaHost, "")
	t.Setenv(envOpenRouterAPIKey, "")
	t.Setenv(envModel, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != ProviderClaude {
		t.Errorf("provider = %q, want %q", cfg.Provider, ProviderClaude)
	}
	if cfg.Model != defaultClaudeModel {
		t.Errorf("model = %q, want %q", cfg.Model, defaultClaudeModel)
	}
}

func TestConfigFromEnv_ModelOverride_Claude(t *testing.T) {
	orig := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = orig })
	t.Setenv(envProvider, "")
	t.Setenv(envOllamaHost, "")
	t.Setenv(envOpenRouterAPIKey, "")
	t.Setenv(envModel, "opus")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "opus" {
		t.Errorf("model = %q, want %q", cfg.Model, "opus")
	}
}

func TestConfigFromEnv_ClaudePriority(t *testing.T) {
	orig := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = orig })
	t.Setenv(envProvider, "")
	t.Setenv(envOllamaHost, "http://localhost:11434")
	t.Setenv(envOpenRouterAPIKey, "sk-test-key")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != ProviderClaude {
		t.Errorf("expected Claude to win priority, got %q", cfg.Provider)
	}
}

func TestConfigFromEnv_Ollama(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "http://localhost:11434")
	t.Setenv(envOpenRouterAPIKey, "")
	t.Setenv(envModel, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != ProviderOllama {
		t.Errorf("provider = %q, want %q", cfg.Provider, ProviderOllama)
	}
	if cfg.Model != defaultOllamaModel {
		t.Errorf("model = %q, want %q", cfg.Model, defaultOllamaModel)
	}
	if cfg.BaseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want %q", cfg.BaseURL, "http://localhost:11434")
	}
}

func TestConfigFromEnv_OpenRouter(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "")
	t.Setenv(envOpenRouterAPIKey, "sk-test-key")
	t.Setenv(envModel, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != ProviderOpenRouter {
		t.Errorf("provider = %q, want %q", cfg.Provider, ProviderOpenRouter)
	}
	if cfg.Model != defaultOpenRouterModel {
		t.Errorf("model = %q, want %q", cfg.Model, defaultOpenRouterModel)
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("apiKey = %q, want %q", cfg.APIKey, "sk-test-key")
	}
}

func TestConfigFromEnv_OpenRouterPriority(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "http://localhost:11434")
	t.Setenv(envOpenRouterAPIKey, "sk-test-key")
	t.Setenv(envModel, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Provider != ProviderOpenRouter {
		t.Errorf("expected OpenRouter to win priority, got %q", cfg.Provider)
	}
}

func TestConfigFromEnv_ModelOverride_Ollama(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "http://localhost:11434")
	t.Setenv(envOpenRouterAPIKey, "")
	t.Setenv(envModel, "llama3.1")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "llama3.1" {
		t.Errorf("model = %q, want %q", cfg.Model, "llama3.1")
	}
}

func TestConfigFromEnv_ModelOverride_OpenRouter(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "")
	t.Setenv(envOpenRouterAPIKey, "sk-test-key")
	t.Setenv(envModel, "openai/gpt-4o")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "openai/gpt-4o" {
		t.Errorf("model = %q, want %q", cfg.Model, "openai/gpt-4o")
	}
}

func TestConfigFromEnv_OllamaNoScheme(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "192.168.4.252:11434")
	t.Setenv(envOpenRouterAPIKey, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "http://192.168.4.252:11434" {
		t.Errorf("baseURL = %q, want %q", cfg.BaseURL, "http://192.168.4.252:11434")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://localhost:11434", "http://localhost:11434"},
		{"https://localhost:11434", "https://localhost:11434"},
		{"localhost:11434", "http://localhost:11434"},
		{"192.168.4.252:11434", "http://192.168.4.252:11434"},
	}
	for _, c := range cases {
		got := normalizeURL(c.in)
		if got != c.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestConfigFromEnv_NoProvider(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOllamaHost, "")
	t.Setenv(envOpenRouterAPIKey, "")

	_, err := configFromEnv()
	if err == nil {
		t.Fatal("expected error when no provider is configured")
	}
}

func stubLookPathWithClaude(t *testing.T) {
	t.Helper()
	orig := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = orig })
}

func TestConfigFromEnv_ForcedProvider(t *testing.T) {
	stubLookPathWithClaude(t)
	t.Setenv(envOpenRouterAPIKey, "sk-test-key")
	t.Setenv(envOllamaHost, "ollama.local:11434")
	t.Setenv(envModel, "")

	cases := []struct {
		forced    string
		want      ProviderType
		wantModel string
	}{
		{"claude", ProviderClaude, defaultClaudeModel},
		{"OpenRouter", ProviderOpenRouter, defaultOpenRouterModel},
		{" ollama ", ProviderOllama, defaultOllamaModel},
	}
	for _, c := range cases {
		t.Setenv(envProvider, c.forced)
		cfg, err := configFromEnv()
		if err != nil {
			t.Fatalf("%s=%q: %v", envProvider, c.forced, err)
		}
		if cfg.Provider != c.want || cfg.Model != c.wantModel {
			t.Errorf("%s=%q: got %s/%s, want %s/%s", envProvider, c.forced, cfg.Provider, cfg.Model, c.want, c.wantModel)
		}
	}
	if cfg, _ := configFromEnv(); cfg.BaseURL != "http://ollama.local:11434" {
		t.Errorf("ollama baseURL = %q", cfg.BaseURL)
	}
}

func TestConfigFromEnv_ForcedProviderErrors(t *testing.T) {
	stubLookPathNoClaude(t)
	t.Setenv(envOpenRouterAPIKey, "")

	for _, forced := range []string{"claude", "openrouter", "gpt"} {
		t.Setenv(envProvider, forced)
		if _, err := configFromEnv(); err == nil {
			t.Errorf("%s=%q: expected error", envProvider, forced)
		}
	}
}

func TestConfigWithDefaults(t *testing.T) {
	cases := []struct {
		in          Config
		wantModel   string
		wantBaseURL string
	}{
		{Config{Provider: ProviderClaude}, defaultClaudeModel, ""},
		{Config{Provider: ProviderOpenRouter}, defaultOpenRouterModel, defaultOpenRouterBaseURL},
		{Config{Provider: ProviderOllama}, defaultOllamaModel, defaultOllamaHost},
		{Config{Provider: ProviderOllama, Model: "m", BaseURL: "host:1"}, "m", "http://host:1"},
	}
	for _, c := range cases {
		got := c.in.withDefaults()
		if got.Model != c.wantModel || got.BaseURL != c.wantBaseURL {
			t.Errorf("%+v.withDefaults() = model %q baseURL %q, want %q %q",
				c.in, got.Model, got.BaseURL, c.wantModel, c.wantBaseURL)
		}
	}
}
