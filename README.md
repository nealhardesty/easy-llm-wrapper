# easy-llm-wrapper

A simple, opinionated Go library for making LLM prompt queries via [Ollama](https://ollama.com) (local) or [OpenRouter](https://openrouter.ai) (cloud). Provider and model are selected automatically from environment variables.

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
| `OPENROUTER_API_KEY` | OpenRouter API key. When set, OpenRouter is used (takes priority). |
| `OLLAMA_HOST`        | Ollama base URL (e.g. `http://localhost:11434`). Used when `OPENROUTER_API_KEY` is not set. |
| `MODEL`              | Overrides the default model for whichever provider is active. |

**Priority:** OpenRouter > Ollama when both are configured.

**Default models:** `llama3.2` (Ollama) · `anthropic/claude-3-haiku` (OpenRouter)

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
    Model:    "openai/gpt-4o",
    BaseURL:  "https://openrouter.ai/api/v1",
    APIKey:   os.Getenv("OPENROUTER_API_KEY"),
})
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

The provider and model are printed to stderr before the response:

```
[ollama / llama3.2]
Paris is the capital of France.
```

### Flags

| Flag | Description |
|------|-------------|
| `-v`, `--version` | Print version and exit |
| `-d` | Debug mode: prints env vars, provider selection, chunk count, and elapsed time |

### Debug mode

```sh
./elw -d What is 2+2?
```

```
[debug] === elw 0.1.0 ===
[debug] OLLAMA_HOST        = "http://localhost:11434"
[debug] OPENROUTER_API_KEY = ""
[debug] MODEL              = ""
[debug] question           = "What is 2+2?"
[debug] provider = ollama
[debug] model    = llama3.2
[debug] --- response (streaming) ---
4
[debug] --- done ---
[debug] chunks   = 3
[debug] elapsed  = 312ms
```

API keys are masked in debug output (first 4 chars shown).

## Development

```sh
make build          # compile
make test           # unit tests (race detector on)
make test-functional # integration tests (requires real env vars)
make lint           # go vet + golangci-lint
make fmt            # gofmt + goimports
make tidy           # go mod tidy
make version        # show current version
make push           # bump patch, commit, push, tag
make help           # list all targets
```

## License

See [LICENSE](LICENSE).
