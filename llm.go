// Package llm provides a unified, opinionated interface for making LLM prompt
// queries via Ollama (local) or OpenRouter (cloud). Provider and model are
// selected automatically from environment variables with sensible defaults.
//
// Quick start:
//
//	client, err := llm.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	answer, err := client.Ask(ctx, "", "What is the capital of France?")
package llm

import (
	"context"
	"fmt"
)

// Client is the main entry point for making LLM requests.
// Create once with NewClient or NewClientWithConfig and reuse across calls.
type Client struct {
	cfg      Config
	provider provider
}

// NewClient creates a Client by auto-detecting provider and model from
// environment variables. Returns an error if neither OLLAMA_HOST nor
// OPENROUTER_API_KEY is set.
func NewClient() (*Client, error) {
	cfg, err := configFromEnv()
	if err != nil {
		return nil, err
	}
	return NewClientWithConfig(cfg)
}

// NewClientWithConfig creates a Client with explicit configuration,
// bypassing environment-based auto-detection. Useful for testing or overrides.
func NewClientWithConfig(cfg Config) (*Client, error) {
	p, err := newProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{cfg: cfg, provider: p}, nil
}

func newProvider(cfg Config) (provider, error) {
	switch cfg.Provider {
	case ProviderClaude:
		return newClaudeProvider(cfg.Debug), nil
	case ProviderOllama:
		return newOllamaProvider(cfg.BaseURL, cfg.Transport), nil
	case ProviderOpenRouter:
		return newOpenRouterProvider(cfg.BaseURL, cfg.APIKey, cfg.Transport), nil
	default:
		return nil, fmt.Errorf("unknown provider: %q", cfg.Provider)
	}
}

// Complete sends a request and returns the full response.
func (c *Client) Complete(ctx context.Context, req Request) (*Response, error) {
	return c.provider.complete(ctx, c.cfg.Model, req)
}

// Ask is a convenience wrapper for simple single-turn text queries.
// system may be empty. Returns only the response text.
func (c *Client) Ask(ctx context.Context, system, user string) (string, error) {
	resp, err := c.Complete(ctx, Request{
		System: system,
		Messages: []Message{
			{Role: RoleUser, Parts: []Part{TextPart(user)}},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// Stream sends a request and returns a StreamResponse for incremental reading.
// The caller must always call Close when done, even if Next returns false.
func (c *Client) Stream(ctx context.Context, req Request) (*StreamResponse, error) {
	return c.provider.stream(ctx, c.cfg.Model, req)
}

// Provider returns which backend is active.
func (c *Client) Provider() ProviderType {
	return c.cfg.Provider
}

// Model returns the model currently configured.
func (c *Client) Model() string {
	return c.cfg.Model
}
