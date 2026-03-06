package llm

import (
	"fmt"
	"testing"
)

// stubLookPathNoClaude replaces lookPath for tests that should not find the claude binary.
func stubLookPathNoClaude(t *testing.T) {
	t.Helper()
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
}

func TestConfigFromEnv_ClaudePriority(t *testing.T) {
	orig := lookPath
	lookPath = func(name string) (string, error) { return "/usr/bin/claude", nil }
	t.Cleanup(func() { lookPath = orig })
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
