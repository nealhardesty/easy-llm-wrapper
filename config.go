package llm

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// lookPath is a variable so tests can override it to control PATH scanning.
var lookPath = exec.LookPath

// ProviderType identifies which LLM backend to use.
type ProviderType string

const (
	ProviderClaude     ProviderType = "claude"
	ProviderOllama     ProviderType = "ollama"
	ProviderOpenRouter ProviderType = "openrouter"
)

const (
	envOllamaHost       = "OLLAMA_HOST"
	envOpenRouterAPIKey = "OPENROUTER_API_KEY"
	envModel            = "MODEL"

	defaultOllamaModel       = "gpt-oss:20b"
	defaultOpenRouterModel   = "anthropic/claude-3-haiku"
	defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// Config holds explicit provider configuration. Used with NewClientWithConfig
// to bypass environment-based auto-detection.
type Config struct {
	Provider  ProviderType
	Model     string
	BaseURL   string            // Ollama: OLLAMA_HOST value; OpenRouter: API base URL
	APIKey    string            // OpenRouter only
	Transport http.RoundTripper // optional; uses http.DefaultTransport when nil
}

// normalizeURL ensures the URL has an http:// scheme if none is present.
func normalizeURL(u string) string {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return "http://" + u
	}
	return u
}

// configFromEnv auto-detects provider and model from environment variables.
// Priority: claude CLI (if on PATH) > OpenRouter (if API key set) > Ollama (if host set).
func configFromEnv() (Config, error) {
	if _, err := lookPath("claude"); err == nil {
		model := os.Getenv(envModel)
		return Config{
			Provider: ProviderClaude,
			Model:    model,
		}, nil
	}

	if key := os.Getenv(envOpenRouterAPIKey); key != "" {
		model := defaultOpenRouterModel
		if m := os.Getenv(envModel); m != "" {
			model = m
		}
		return Config{
			Provider: ProviderOpenRouter,
			Model:    model,
			BaseURL:  defaultOpenRouterBaseURL,
			APIKey:   key,
		}, nil
	}

	if host := os.Getenv(envOllamaHost); host != "" {
		model := defaultOllamaModel
		if m := os.Getenv(envModel); m != "" {
			model = m
		}
		return Config{
			Provider: ProviderOllama,
			Model:    model,
			BaseURL:  normalizeURL(host),
		}, nil
	}

	return Config{}, fmt.Errorf("no LLM provider configured: install claude CLI, or set %s or %s", envOpenRouterAPIKey, envOllamaHost)
}
