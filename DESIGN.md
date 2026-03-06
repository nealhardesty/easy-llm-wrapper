# easy-llm-wrapper — Design Document

## Overview

A Go library providing a unified, opinionated interface for making LLM prompt queries across multiple backends (Ollama and OpenRouter). Designed for reuse across personal projects with zero-config ergonomics — the right provider and model are selected automatically from the environment.

---

## Requirements

| # | Requirement |
|---|-------------|
| R1 | Support Ollama (local) as a backend |
| R2 | Support OpenRouter (cloud) as a backend |
| R3 | Auto-select Ollama if `OLLAMA_HOST` env var is present |
| R4 | Auto-select OpenRouter if `OPENROUTER_API_KEY` env var is present |
| R5 | Ollama takes precedence over OpenRouter when both are configured |
| R6 | `MODEL` env var overrides the selected model |
| R7 | Each provider has a sensible hardcoded default model |
| R8 | Support system prompts |
| R9 | Support user prompts |
| R10 | Support multi-modal input (text + images) |

---

## Provider Selection Logic

```
if OLLAMA_HOST is set:
    use Ollama provider
    model = OLLAMA_DEFAULT_MODEL (override with MODEL env)
else if OPENROUTER_API_KEY is set:
    use OpenRouter provider
    model = OPENROUTER_DEFAULT_MODEL (override with MODEL env)
else:
    return error: no provider configured
```

### Default Models

| Provider   | Default Model              |
|------------|---------------------------|
| Ollama     | `llama3.2`                |
| OpenRouter | `anthropic/claude-3-haiku` |

---

## Package Structure

```
easy-llm-wrapper/
├── llm.go           # Public API: Client, NewClient, Request, Response types
├── provider.go      # Provider interface
├── config.go        # Environment-based configuration and auto-detection
├── version.go       # Semantic version constant
├── internal/
│   ├── ollama/
│   │   └── ollama.go    # Ollama HTTP client implementation
│   └── openrouter/
│       └── openrouter.go # OpenRouter HTTP client implementation
├── go.mod
├── Makefile
└── README.md
```

---

## API Design

### Core Types

```go
// Client is the main entry point. Created once, reused across calls.
type Client struct { ... }

// NewClient auto-detects provider and model from environment.
// Returns an error if no provider can be configured.
func NewClient() (*Client, error)

// NewClientWithConfig creates a client with explicit configuration,
// bypassing environment detection. Useful for testing or overrides.
func NewClientWithConfig(cfg Config) (*Client, error)

// Config holds explicit provider configuration.
type Config struct {
    Provider  ProviderType // ProviderOllama or ProviderOpenRouter
    Model     string
    BaseURL   string       // Ollama: OLLAMA_HOST, OpenRouter: hardcoded
    APIKey    string       // OpenRouter only
}

type ProviderType string

const (
    ProviderOllama     ProviderType = "ollama"
    ProviderOpenRouter ProviderType = "openrouter"
)
```

### Request & Response

```go
// Request represents a prompt request to the LLM.
type Request struct {
    // System sets the system prompt. Optional.
    System string

    // Messages is the conversation. Typically one user message
    // for single-turn use, or multiple for multi-turn.
    Messages []Message
}

// Message is a single turn in the conversation.
type Message struct {
    Role    Role      // RoleUser or RoleAssistant
    Parts   []Part    // One or more content parts (text, image)
}

type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)

// Part is a single piece of content within a message.
// Use TextPart() or ImagePart() constructors.
type Part struct {
    Type     PartType
    Text     string     // valid when Type == PartTypeText
    MIMEType string     // valid when Type == PartTypeImage (e.g. "image/png")
    Data     []byte     // raw image bytes when Type == PartTypeImage
}

type PartType string

const (
    PartTypeText  PartType = "text"
    PartTypeImage PartType = "image"
)

// Constructors for convenience
func TextPart(text string) Part
func ImagePart(mimeType string, data []byte) Part

// Response is the LLM's reply.
type Response struct {
    Text  string // The generated text content
    Model string // The model that was used
}
```

### Client Methods

```go
// Complete sends a request and returns the full response.
func (c *Client) Complete(ctx context.Context, req Request) (*Response, error)

// Ask is a convenience wrapper for simple single-turn text queries.
// system may be empty. Returns only the response text.
func (c *Client) Ask(ctx context.Context, system, user string) (string, error)

// Stream sends a request and returns a StreamResponse for incremental reading.
func (c *Client) Stream(ctx context.Context, req Request) (*StreamResponse, error)

// Provider returns which backend is active.
func (c *Client) Provider() ProviderType

// Model returns the model currently configured.
func (c *Client) Model() string
```

### Ask example

```go
answer, err := client.Ask(ctx, "You are a helpful assistant.", "What is 2+2?")

// system prompt is optional
answer, err := client.Ask(ctx, "", "Summarize this for me: ...")
```

---

## Internal Provider Interface

```go
// provider is the internal interface both backends implement.
type provider interface {
    complete(ctx context.Context, model string, req Request) (*Response, error)
}
```

Both `internal/ollama` and `internal/openrouter` implement this interface. The public `Client` wraps a `provider` and holds the resolved model name.

---

## Usage Examples

### Simplest case (zero config)

```go
client, err := llm.NewClient()
if err != nil {
    log.Fatal(err) // no provider configured in environment
}

resp, err := client.Complete(ctx, llm.Request{
    System: "You are a helpful assistant.",
    Messages: []llm.Message{
        {Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart("What is 2+2?")}},
    },
})
```

### Multi-modal (image + text)

```go
imageBytes, _ := os.ReadFile("screenshot.png")

resp, err := client.Complete(ctx, llm.Request{
    Messages: []llm.Message{
        {
            Role: llm.RoleUser,
            Parts: []llm.Part{
                llm.ImagePart("image/png", imageBytes),
                llm.TextPart("Describe what you see in this image."),
            },
        },
    },
})
```

### Explicit config (override environment)

```go
client, err := llm.NewClientWithConfig(llm.Config{
    Provider: llm.ProviderOpenRouter,
    Model:    "openai/gpt-4o",
    APIKey:   os.Getenv("OPENROUTER_API_KEY"),
})
```

---

## Environment Variables

| Variable            | Description                                      |
|---------------------|--------------------------------------------------|
| `OLLAMA_HOST`       | Ollama base URL (e.g. `http://localhost:11434`). Presence triggers Ollama provider. |
| `OPENROUTER_API_KEY`| OpenRouter API key. Presence triggers OpenRouter provider. |
| `MODEL`             | Overrides the default model for whichever provider is selected. |

---

## Streaming

Both `Complete` (full response) and `Stream` (incremental chunks) are supported in v1.

`Stream` uses the `bufio.Scanner`-style iterator pattern — idiomatic Go, no channels or callbacks required.

```go
// Stream sends a request and returns a StreamResponse for incremental reading.
// The caller must always call Close() when done.
func (c *Client) Stream(ctx context.Context, req Request) (*StreamResponse, error)

// StreamResponse provides incremental access to generated text chunks.
// Usage mirrors bufio.Scanner.
type StreamResponse struct { ... }

// Next advances to the next chunk. Returns false when the stream
// is exhausted or an error occurs.
func (s *StreamResponse) Next() bool

// Chunk returns the current text chunk. Only valid after a true return from Next.
func (s *StreamResponse) Chunk() string

// Err returns any error encountered during streaming. Check after Next returns false.
func (s *StreamResponse) Err() error

// Close releases the underlying HTTP response body. Always call this.
func (s *StreamResponse) Close() error
```

### Streaming example

```go
stream, err := client.Stream(ctx, llm.Request{
    System: "You are a helpful assistant.",
    Messages: []llm.Message{
        {Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart("Tell me a story.")}},
    },
})
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    fmt.Print(stream.Chunk())
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
```

### Internal streaming interface

```go
type provider interface {
    complete(ctx context.Context, model string, req Request) (*Response, error)
    stream(ctx context.Context, model string, req Request) (*StreamResponse, error)
}
```

Both Ollama (`/api/chat` with `"stream": true`) and OpenRouter (OpenAI-compatible SSE) support server-sent event streaming. The internal adapters parse their respective SSE formats and expose a unified `StreamResponse`.

---

## Resolved Decisions

| Decision | Resolution |
|----------|-----------|
| Module path | `github.com/nealhardesty/easy-llm-wrapper` |
| Provider priority | Ollama > OpenRouter when both env vars are present |
| Streaming | Supported in v1 via `Stream()` + `StreamResponse` iterator |
| Default OpenRouter model | `anthropic/claude-3-haiku` |

## Open Questions

1. **Multi-turn / conversation history** — The `Messages []Message` slice supports it structurally, but no session management is provided. Callers build history themselves.
2. **Ollama image encoding** — Ollama's API takes base64-encoded images directly. OpenRouter follows OpenAI's format (base64 data URLs). The internal adapters handle the translation.
3. **Timeout / retry policy** — Leave to the caller via `context.Context` for now. No built-in retry logic.
