# easy-llm-wrapper

A simple, opinionated Go library for making LLM prompt queries via the [Claude Code](https://claude.com/claude-code) CLI, [OpenRouter](https://openrouter.ai) (cloud), or [Ollama](https://ollama.com) (local). Provider and model are selected automatically from the environment.

## Installation

**Library:**

```sh
go get github.com/nealhardesty/easy-llm-wrapper
```

**CLI tools:**

```sh
go install github.com/nealhardesty/easy-llm-wrapper/cmd/elw@latest
go install github.com/nealhardesty/easy-llm-wrapper/cmd/elwi@latest
```

## Quick Start

```go
import llm "github.com/nealhardesty/easy-llm-wrapper"

client, err := llm.NewClient()
if err != nil {
    log.Fatal(err) // no provider configured
}

// Simplest possible call
answer, err := client.Ask(ctx, "", "What is the capital of France?")
```

## Environment Variables

| Variable             | Description |
|----------------------|-------------|
| `LLM_PROVIDER`       | Force a provider: `claude`, `openrouter`, or `ollama`. Overrides auto-detection. |
| `OPENROUTER_API_KEY` | OpenRouter API key. Used when the `claude` CLI is not on `PATH`. |
| `OLLAMA_HOST`        | Ollama base URL (e.g. `http://localhost:11434`). Used when neither the `claude` CLI nor `OPENROUTER_API_KEY` is available. |
| `MODEL`              | Overrides the default model for whichever provider is active. |
| `LLM_DEBUG`          | Set to `1` to log provider detection to stderr. |

**Priority:** `claude` CLI (if on `PATH`) > OpenRouter > Ollama.

**Default models:** `sonnet` (claude CLI) · `anthropic/claude-haiku-5.5` (OpenRouter) · `qwen3.8:27b` (Ollama)

All three providers support system prompts, multi-turn messages, image input, and streaming.
The claude provider runs `claude -p` as a plain LLM call (no tools or MCP servers); you must be logged in to the CLI.

## Usage

### Simple text query

```go
// system prompt is optional (pass "" to omit)
answer, err := client.Ask(ctx, "You are a helpful assistant.", "Explain Go interfaces.")
```

### Full request (multi-turn, system prompt)

```go
resp, err := client.Complete(ctx, llm.Request{
    System: "You are a concise assistant.",
    Messages: []llm.Message{
        {Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart("What is 2+2?")}},
    },
})
fmt.Println(resp.Text)
```

### Multi-modal (image + text)

```go
imageBytes, _ := os.ReadFile("screenshot.png")

resp, err := client.Complete(ctx, llm.Request{
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Parts: []llm.Part{
            llm.ImagePart("image/png", imageBytes),
            llm.TextPart("Describe what you see."),
        },
    }},
})
```

### Streaming

```go
stream, err := client.Stream(ctx, llm.Request{
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

### Explicit configuration (bypass environment)

```go
client, err := llm.NewClientWithConfig(llm.Config{
    Provider: llm.ProviderOpenRouter,
    Model:    "openai/gpt-4o",               // empty = provider default
    APIKey:   os.Getenv("OPENROUTER_API_KEY"), // BaseURL defaults to OpenRouter's API
})
```

### Image generation (OpenRouter)

```go
resp, err := client.Complete(ctx, llm.Request{
    Messages:   []llm.Message{{Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart("A red balloon")}}},
    Modalities: []string{"image", "text"},
})
// resp.Images[i].MIMEType / resp.Images[i].Data
```

## elw — Command Line Tool

A minimal CLI for testing your LLM configuration from the terminal.

### Build

```sh
make build            # builds both ./elw and ./elwi in the project root
make build-elw        # builds ./elw only
make install-elw      # installs to $(GOPATH)/bin
```

Or install directly without cloning:

```sh
go install github.com/nealhardesty/easy-llm-wrapper/cmd/elw@latest
go install github.com/nealhardesty/easy-llm-wrapper/cmd/elwi@latest
```

> **Note:** `go install github.com/nealhardesty/easy-llm-wrapper` will **not** install the CLI tools — the root package is the library. Use the `cmd/elw` and `cmd/elwi` paths above.

### Usage

```sh
elw [flags] [question]
```

Multi-word questions do not need quoting. If no question is given, the prompt is read from stdin.

```sh
./elw What is the capital of France?
./elw Explain Go interfaces in one sentence.

echo "Summarise this:" | ./elw
cat prompt.txt | ./elw
```

Only the response goes to stdout; errors go to stderr with exit status 1. Use `-d` to see which provider and model were chosen.

### Flags

| Flag | Description |
|------|-------------|
| `-s` | System prompt |
| `-v`, `--version` | Print version and exit |
| `-d` | Debug mode: prints env vars, provider selection, chunk count, and elapsed time |

### Debug mode

```sh
./elw -d What is 2+2?
```

```
[debug] === elw 0.1.14 ===
[debug] LLM_PROVIDER       = ""
[debug] OLLAMA_HOST        = "http://localhost:11434"
[debug] OPENROUTER_API_KEY = ""
[debug] MODEL              = ""
[debug] system             = ""
[debug] question           = "What is 2+2?"
[debug] provider = ollama
[debug] model    = qwen3.8:27b
[debug] --- response (streaming) ---
4
[debug] --- done ---
[debug] chunks   = 3
[debug] elapsed  = 312ms
```

API keys are masked in debug output (first 4 chars shown).

## elwi — Image Generation Tool

Generates images from a text prompt via OpenRouter (requires `OPENROUTER_API_KEY`). Default model: `google/gemini-3.1-flash-image`.

```sh
elwi A photorealistic cat wearing a top hat        # → output.png
elwi -o cat -model google/gemini-3-pro-image A cat # → cat.png
echo 'A red balloon' | elwi -f                     # overwrite existing output.*
```

| Flag | Description |
|------|-------------|
| `-o` | Output file base name; the extension comes from the image type (default `output`) |
| `-f` | Overwrite existing output files (otherwise elwi refuses before calling the API) |
| `-model` | Image model (default `google/gemini-3.1-flash-image`; `MODEL` env also works) |
| `-s` | System prompt |
| `-d` | Debug: dumps HTTP traffic (API key masked, bodies truncated) |
| `-v`, `--version` | Print version and exit |

## Development

```sh
make                 # list all targets (same as make help)
make build           # compile library, elw, and elwi
make run ARGS="..."  # build and run elw
make test            # unit tests (race detector on)
make test-functional # live tests against every available provider
make lint            # go vet + golangci-lint
make fmt             # gofmt + goimports
make tidy            # go mod tidy
make version         # show current version
make version-increment # bump the patch version only
make push            # bump patch, test, commit, push, tag (needs `gitsum` on PATH)
```

## License

See [LICENSE](LICENSE).
