# easy-llm-wrapper — Specification

## Overview

A Go library providing a unified, opinionated interface for making LLM prompt
queries across three backends — the **Claude Code CLI**, **OpenRouter**
(cloud), and **Ollama** (local). Designed for reuse across personal projects
with zero-config ergonomics: the right provider and model are selected
automatically from the environment.

Two CLIs ship with the library: `elw` (text prompts, streamed) and `elwi`
(image generation via OpenRouter).

---

## Requirements

| #   | Requirement |
|-----|-------------|
| R1  | Support the Claude Code CLI (`claude`) as a backend |
| R2  | Support OpenRouter (cloud) as a backend |
| R3  | Support Ollama (local) as a backend |
| R4  | Auto-select a provider: claude CLI on `PATH` > `OPENROUTER_API_KEY` > `OLLAMA_HOST` |
| R5  | `LLM_PROVIDER` forces a specific provider, overriding auto-detection |
| R6  | `MODEL` env var overrides the selected provider's default model |
| R7  | Each provider has a sensible hardcoded default model |
| R8  | Support system prompts |
| R9  | Support user prompts and multi-turn message history |
| R10 | Support multi-modal input (text + images) on every provider |
| R11 | Support streaming on every provider, preserving whitespace exactly |
| R12 | Provider errors (including ones reported mid-stream or with HTTP 200) surface as Go errors, never as silent empty/truncated output |

---

## Provider Selection Logic

```
if LLM_PROVIDER is set (claude | openrouter | ollama, case-insensitive):
    use that provider, or error if its prerequisite is missing
    (claude: binary on PATH; openrouter: OPENROUTER_API_KEY; ollama: none,
     OLLAMA_HOST defaults to http://localhost:11434)
else if `claude` is on PATH:
    use Claude CLI provider
else if OPENROUTER_API_KEY is set:
    use OpenRouter provider
else if OLLAMA_HOST is set:
    use Ollama provider
else:
    return error: no provider configured

model = MODEL env, else the provider default
```

`LLM_DEBUG=1` logs each detection step to stderr.

### Defaults

| Provider   | Default model                | Default base URL               |
|------------|------------------------------|--------------------------------|
| Claude CLI | `sonnet` (CLI alias → latest Sonnet) | n/a                    |
| OpenRouter | `anthropic/claude-haiku-5.5` | `https://openrouter.ai/api/v1` |
| Ollama     | `qwen3.8:27b`                | `http://localhost:11434`       |
| `elwi`     | `google/gemini-3.1-flash-image` (OpenRouter) | —              |

`NewClientWithConfig` applies the same defaults to any empty `Model` /
`BaseURL`, and adds `http://` to an Ollama `BaseURL` with no scheme.

---

## Package Structure

```
easy-llm-wrapper/
├── llm.go           # Client, NewClient, NewClientWithConfig, Complete/Ask/Stream
├── types.go         # Request, Message, Part, Response, Role, PartType
├── config.go        # Config, ConfigFromEnv, env detection, defaults
├── provider.go      # internal provider interface
├── stream.go        # StreamResponse (line-oriented iterator shared by all providers)
├── claude.go        # Claude Code CLI provider (subprocess, stream-json)
├── openrouter.go    # OpenRouter provider (OpenAI-compatible HTTP + SSE)
├── ollama.go        # Ollama provider (/api/chat + NDJSON)
├── version.go       # Version constant
├── functional_test.go  # live tests (build tag `functional`)
└── cmd/
    ├── elw/         # text CLI (streams the reply)
    └── elwi/        # image-generation CLI (OpenRouter only)
```

---

## API

### Client & config

```go
func NewClient() (*Client, error)                    // detect from environment
func NewClientWithConfig(cfg Config) (*Client, error) // explicit; empty fields get defaults
func ConfigFromEnv() (Config, error)                  // inspect/modify before NewClientWithConfig

type Config struct {
    Provider  ProviderType      // ProviderClaude | ProviderOpenRouter | ProviderOllama
    Model     string
    BaseURL   string            // Ollama host or OpenRouter API base
    APIKey    string            // OpenRouter only
    Transport http.RoundTripper // optional (HTTP providers)
    Debug     bool              // claude provider: log exec details to stderr
}

func (c *Client) Complete(ctx, Request) (*Response, error)
func (c *Client) Ask(ctx, system, user string) (string, error)
func (c *Client) Stream(ctx, Request) (*StreamResponse, error)
func (c *Client) Provider() ProviderType
func (c *Client) Model() string
```

### Request & response

```go
type Request struct {
    System     string     // optional system prompt
    Messages   []Message  // conversation; usually one user message
    Modalities []string   // OpenRouter only, e.g. {"image","text"} for image models
}

type Message struct { Role Role; Parts []Part }   // RoleUser | RoleAssistant
type Part struct {
    Type     PartType // PartTypeText | PartTypeImage
    Text     string
    MIMEType string
    Data     []byte   // raw image bytes
}
func TextPart(text string) Part
func ImagePart(mimeType string, data []byte) Part

type Response struct {
    Text   string
    Model  string  // model reported by the provider
    Images []Part  // generated images (OpenRouter image models)
}
```

### Streaming

`StreamResponse` mirrors `bufio.Scanner`: loop on `Next()`, read `Chunk()`,
check `Err()` after the loop, and always `Close()`.

- Lines up to 16 MB are accepted (base64 payloads exceed the 64 KB default).
- Once `Next()` returns false it stays false.
- A provider may install an end-of-stream check (`atEOF`) so that a stream
  that ends without its completion marker is an error, not a silent truncation.
- `Close()` on a claude stream kills the subprocess if it is still running, so
  abandoning a stream returns immediately.

---

## Provider Details

### Claude Code CLI (`claude.go`)

Invocation (verified against Claude Code 2.1.308):

```
claude -p --input-format stream-json --output-format stream-json --verbose \
       --include-partial-messages --tools "" --strict-mcp-config \
       --no-session-persistence [--system-prompt S] [--model M]
```

- **Input** is one stream-json line on stdin:
  `{"type":"user","message":{"role":"user","content":[{"type":"text",...},{"type":"image","source":{"type":"base64","media_type":...,"data":...}}]}}`.
  Using stdin (not argv) means prompts may start with `-`, can be larger than
  the OS argv limit, and don't show up in `ps`. Multi-turn history is
  flattened into a `Human:`/`Assistant:` transcript in the text block; every
  image from the conversation follows it.
- **Output** is one JSON event per line. The provider reads:
  - `{"type":"system","subtype":"init","model":...}` → `Response.Model`
  - `{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":...}}}` → chunks
  - `{"type":"result","is_error":bool,"result":...}` → end of stream; when
    `is_error` is true, `result` holds the human-readable error (e.g. an
    unknown model) and becomes the Go error.
- `--output-format text` is **not** used: it emits nothing until the end (no
  real streaming), and the line-based reader would strip its newlines.
- `--verbose` is required by the CLI for stream-json output in print mode.
- `--include-partial-messages` only works with `--output-format stream-json`.
- `--tools ""` and `--strict-mcp-config` make this a plain LLM call rather
  than an agent session. `--system-prompt` *replaces* Claude Code's system
  prompt when `System` is set; otherwise the CLI default applies.
- If the process exits non-zero before a result, the error includes the last
  line of stderr; on context cancellation it wraps `ctx.Err()`.

### OpenRouter (`openrouter.go`)

- `POST {BaseURL}/chat/completions`, OpenAI format, `Authorization: Bearer`.
- A single text-only part is sent as string `content` (widest compatibility);
  anything else as a content-part array with images as base64 data URLs.
- `Request.Modalities` is sent as `"modalities"`; image generation requires
  `["image","text"]`.
- Generated images arrive in `message.images` (Gemini image models) or inside
  `message.content` parts. Only `data:` URIs are supported; any other URL is
  an explicit error. Entries without an `image_url` are skipped. Absent or
  `null` content is treated as empty text.
- Errors: non-200 status, a top-level `"error"` object in a 200 body, and
  mid-stream `"error"` events / `finish_reason: "error"` all become Go errors.
- SSE: `data:` with or without a following space; `data: [DONE]` ends the
  stream; other SSE fields/comments are ignored.

### Ollama (`ollama.go`)

- `POST {BaseURL}/api/chat`; images go in `messages[].images` as bare base64.
- Streaming is NDJSON (one JSON object per line, not SSE); `"done": true`
  ends it.
- An `"error"` field (in a 200 response or mid-stream) becomes a Go error.

---

## CLIs

### elw

`elw [-s system] [-d] [question...]` — streams the reply to stdout. Reads the
prompt from stdin when no args are given; prints usage instead of blocking
when stdin is a terminal. Exits 1 on any error (including mid-stream).

### elwi

`elwi [-model M] [-o base] [-f] [-s system] [-d] [prompt...]` — OpenRouter
only; sends `modalities: ["image","text"]` and writes each image as
`<base>.<ext>`, `<base>_2.<ext>`, …. Refuses to overwrite existing files
unless `-f`, checking before making the (paid) API call. `-d` dumps HTTP
traffic with the API key masked and bodies truncated to 4 KB.

---

## Resolved Decisions

| Decision | Resolution |
|----------|-----------|
| Module path | `github.com/nealhardesty/easy-llm-wrapper` |
| Provider priority | `LLM_PROVIDER` > claude CLI > OpenRouter > Ollama |
| Streaming | `Stream()` + `StreamResponse` iterator on all providers |
| Claude transport | stream-json in and out over stdin/stdout |
| Timeout / retry | Left to the caller via `context.Context`; no built-in retry |
| Conversation state | None; callers build `Messages` themselves |
