//go:build functional

package llm

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Functional tests exercise real provider endpoints.
// Run with: make test-functional
// Requires OLLAMA_HOST or OPENROUTER_API_KEY to be set.

func TestFunctional_OllamaAsk(t *testing.T) {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		t.Skip("OLLAMA_HOST not set")
	}

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Provider() != ProviderOllama {
		t.Fatalf("expected Ollama provider, got %q", c.Provider())
	}

	answer, err := c.Ask(context.Background(), "", "Reply with only the word PONG.")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(strings.ToUpper(answer), "PONG") {
		t.Errorf("unexpected answer: %q", answer)
	}
}

func TestFunctional_OllamaStream(t *testing.T) {
	if os.Getenv("OLLAMA_HOST") == "" {
		t.Skip("OLLAMA_HOST not set")
	}

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sr, err := c.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{TextPart("Reply with only the word PONG.")}},
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
	if sb.Len() == 0 {
		t.Error("expected non-empty streamed response")
	}
}

func TestFunctional_OpenRouterAsk(t *testing.T) {
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	// Ensure Ollama is not also set so OpenRouter is selected.
	if os.Getenv("OLLAMA_HOST") != "" {
		t.Skip("OLLAMA_HOST also set; Ollama takes priority — test would not hit OpenRouter")
	}

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.Provider() != ProviderOpenRouter {
		t.Fatalf("expected OpenRouter provider, got %q", c.Provider())
	}

	answer, err := c.Ask(context.Background(), "", "Reply with only the word PONG.")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if !strings.Contains(strings.ToUpper(answer), "PONG") {
		t.Errorf("unexpected answer: %q", answer)
	}
}

func TestFunctional_OpenRouterStream(t *testing.T) {
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	if os.Getenv("OLLAMA_HOST") != "" {
		t.Skip("OLLAMA_HOST also set; Ollama takes priority")
	}

	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	sr, err := c.Stream(context.Background(), Request{
		System: "You are a concise assistant.",
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{TextPart("Reply with only the word PONG.")}},
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
	if sb.Len() == 0 {
		t.Error("expected non-empty streamed response")
	}
}
