//go:build functional

package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Functional tests exercise real provider endpoints.
// Run with: make test-functional
// Each provider is targeted explicitly and skipped when it isn't available:
// claude CLI on PATH, OPENROUTER_API_KEY, or OLLAMA_HOST.

func functionalClients(t *testing.T) map[string]*Client {
	t.Helper()
	clients := map[string]*Client{}
	add := func(name string, cfg Config) {
		c, err := NewClientWithConfig(cfg)
		if err != nil {
			t.Fatalf("%s: NewClientWithConfig: %v", name, err)
		}
		clients[name] = c
	}
	if _, err := lookPath("claude"); err == nil {
		add("claude", Config{Provider: ProviderClaude, Model: "haiku"})
	}
	if key := os.Getenv(envOpenRouterAPIKey); key != "" {
		add("openrouter", Config{Provider: ProviderOpenRouter, APIKey: key})
	}
	if host := os.Getenv(envOllamaHost); host != "" {
		add("ollama", Config{Provider: ProviderOllama, BaseURL: host})
	}
	if len(clients) == 0 {
		t.Skip("no provider available (claude CLI, OPENROUTER_API_KEY, or OLLAMA_HOST)")
	}
	return clients
}

func TestFunctional_Ask(t *testing.T) {
	for name, c := range functionalClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			answer, err := c.Ask(ctx, "You are a concise assistant.", "Reply with only the word PONG.")
			if err != nil {
				t.Fatalf("Ask: %v", err)
			}
			if !strings.Contains(strings.ToUpper(answer), "PONG") {
				t.Errorf("unexpected answer: %q", answer)
			}
		})
	}
}

func TestFunctional_StreamMultiline(t *testing.T) {
	for name, c := range functionalClients(t) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			sr, err := c.Stream(ctx, Request{
				Messages: []Message{
					{Role: RoleUser, Parts: []Part{TextPart("Print the numbers 1, 2 and 3, each on its own line, and nothing else.")}},
				},
			})
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			defer sr.Close()

			var sb strings.Builder
			for sr.Next() {
				sb.WriteString(sr.Chunk())
			}
			if err := sr.Err(); err != nil {
				t.Fatalf("stream error: %v", err)
			}
			if !strings.Contains(sb.String(), "\n") {
				t.Errorf("expected newlines to survive streaming, got %q", sb.String())
			}
		})
	}
}
