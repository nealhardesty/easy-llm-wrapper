package llm

import "context"

// provider is the internal interface that all LLM backends implement.
type provider interface {
	complete(ctx context.Context, model string, req Request) (*Response, error)
	stream(ctx context.Context, model string, req Request) (*StreamResponse, error)
}
