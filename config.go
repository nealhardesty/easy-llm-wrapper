package llm

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// debugf writes to stderr when LLM_DEBUG=1.
func debugf(format string, args ...any) {
	if os.Getenv("LLM_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[llm debug] "+format+"\n", args...)
	}
}

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
	envProvider         = "LLM_PROVIDER"
	envOllamaHost       = "OLLAMA_HOST"
	envOpenRouterAPIKey = "OPENROUTER_API_KEY"
	envModel            = "MODEL"

	defaultClaudeModel       = "sonnet" // claude CLI alias; resolves to the latest Sonnet
	defaultOllamaModel       = "qwen3.8:27b"
	defaultOpenRouterModel   = "anthropic/claude-haiku-5.5"
	defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	defaultOllamaHost        = "http://localhost:11434"
)

// Config holds explicit provider configuration. Used with NewClientWithConfig
// to bypass environment-based auto-detection.
type Config struct {
	Provider  ProviderType
	Model     string
	BaseURL   string            // Ollama: OLLAMA_HOST value; OpenRouter: API base URL
	APIKey    string            // OpenRouter only
	Transport http.RoundTripper // optional; uses http.DefaultTransport when nil
	Debug     bool              // emit debug lines to stderr
}

// ConfigFromEnv auto-detects provider and model from environment variables.
// The returned Config can be modified (e.g. set Debug) before passing to NewClientWithConfig.
func ConfigFromEnv() (Config, error) {
	return configFromEnv()
}

// normalizeURL ensures the URL has an http:// scheme if none is present.
func normalizeURL(u string) string {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return "http://" + u
	}
	return u
}

// withDefaults fills in the default model and base URL for the provider
// wherever cfg leaves them empty.
func (cfg Config) withDefaults() Config {
	switch cfg.Provider {
	case ProviderClaude:
		if cfg.Model == "" {
			cfg.Model = defaultClaudeModel
		}
	case ProviderOpenRouter:
		if cfg.Model == "" {
			cfg.Model = defaultOpenRouterModel
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = defaultOpenRouterBaseURL
		}
	case ProviderOllama:
		if cfg.Model == "" {
			cfg.Model = defaultOllamaModel
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = defaultOllamaHost
		}
		cfg.BaseURL = normalizeURL(cfg.BaseURL)
	}
	return cfg
}

// configFromEnv auto-detects provider and model from environment variables.
// LLM_PROVIDER forces a provider; otherwise the priority is
// claude CLI (if on PATH) > OpenRouter (if API key set) > Ollama (if host set).
func configFromEnv() (Config, error) {
	cfg, err := detectProvider()
	if err != nil {
		return Config{}, err
	}
	cfg.Model = os.Getenv(envModel)
	return cfg.withDefaults(), nil
}

func detectProvider() (Config, error) {
	key := os.Getenv(envOpenRouterAPIKey)
	host := os.Getenv(envOllamaHost)

	if forced := strings.ToLower(strings.TrimSpace(os.Getenv(envProvider))); forced != "" {
		debugf("%s=%s forces provider", envProvider, forced)
		switch ProviderType(forced) {
		case ProviderClaude:
			if _, err := lookPath("claude"); err != nil {
				return Config{}, fmt.Errorf("%s=claude but claude binary not found in PATH: %w", envProvider, err)
			}
			return Config{Provider: ProviderClaude}, nil
		case ProviderOpenRouter:
			if key == "" {
				return Config{}, fmt.Errorf("%s=openrouter but %s is not set", envProvider, envOpenRouterAPIKey)
			}
			return Config{Provider: ProviderOpenRouter, APIKey: key}, nil
		case ProviderOllama:
			return Config{Provider: ProviderOllama, BaseURL: host}, nil
		default:
			return Config{}, fmt.Errorf("%s=%q: want claude, openrouter, or ollama", envProvider, forced)
		}
	}

	if path, err := lookPath("claude"); err == nil {
		debugf("claude binary found at %s", path)
		return Config{Provider: ProviderClaude}, nil
	} else {
		debugf("claude binary not found: %v", err)
	}

	if key != "" {
		debugf("using OpenRouter (API key set)")
		return Config{Provider: ProviderOpenRouter, APIKey: key}, nil
	}
	debugf("OpenRouter skipped: %s not set", envOpenRouterAPIKey)

	if host != "" {
		debugf("using Ollama at %s", host)
		return Config{Provider: ProviderOllama, BaseURL: host}, nil
	}
	debugf("Ollama skipped: %s not set", envOllamaHost)

	return Config{}, fmt.Errorf("no LLM provider configured: install claude CLI, or set %s or %s", envOpenRouterAPIKey, envOllamaHost)
}
