# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Debug logging via `LLM_DEBUG=1`: logs to stderr when `claude` binary is not found, when each provider is skipped or selected, and when exec calls fail in `claude.go`

### Added
- `ProviderClaude` — new provider that shells out to the local `claude` CLI binary
  - `claude.go`: `claudeProvider` implementing `complete` and `stream` via `exec.CommandContext`
  - Single-message prompts passed directly via `-p`; multi-turn formatted as `Human:/Assistant:` pairs
  - System prompt passed via `--append-system-prompt`
  - `cmdReadCloser` ensures subprocess is reaped on stream close
- `claude` CLI auto-detection in `configFromEnv()` via `exec.LookPath` — highest priority (claude > openrouter > ollama)

### Changed
- Provider priority reversed: `OPENROUTER_API_KEY` (OpenRouter) now takes precedence over `OLLAMA_HOST` (Ollama) when both are set
- `make build` now builds both `elw` and `elwi` binaries in addition to compiling all packages
- README updated to clarify correct `go install` paths for CLI tools (`cmd/elw` and `cmd/elwi`) and that the root package is a library only

### Added
- `cmd/elw` — CLI tool for testing LLM configuration from the terminal
  - `elw <question>` sends a streaming request using auto-detected provider/model
  - `-v` / `--version` flag prints version and exits
  - `-d` flag enables debug output: env vars (API keys masked), provider, model, chunk count, elapsed time
  - stdin fallback: if no question arg is given, prompt is read from stdin (`echo "..." | elw`)
- `make build-elw` — builds `./elw` binary
- `make install-elw` — installs `elw` to `GOPATH/bin`
- `make run ARGS="..."` — build and run elw inline

## [0.1.0] - 2026-03-05

### Added
- Initial library implementation
- `NewClient()` — auto-detects provider from environment (`OLLAMA_HOST` → Ollama, `OPENROUTER_API_KEY` → OpenRouter; Ollama takes priority)
- `NewClientWithConfig(cfg Config)` — explicit provider configuration bypassing env detection
- `Client.Complete(ctx, req)` — full non-streaming LLM response
- `Client.Ask(ctx, system, user)` — convenience single-turn text query returning plain string
- `Client.Stream(ctx, req)` — streaming response via `bufio.Scanner`-style `StreamResponse` iterator
- `StreamResponse` with `Next()`, `Chunk()`, `Err()`, `Close()` — idiomatic Go streaming API
- Multi-modal support: `ImagePart(mimeType, data)` alongside `TextPart(text)` for image+text messages
- Ollama backend (`/api/chat`, NDJSON streaming, base64 image encoding)
- OpenRouter backend (OpenAI-compatible `/chat/completions`, SSE streaming, data-URI image encoding)
- `MODEL` env var to override default model for whichever provider is active
- Default models: `llama3.2` (Ollama), `anthropic/claude-3-haiku` (OpenRouter)
- Comprehensive unit tests with `httptest` for both backends
- Functional/integration tests (build tag: `functional`)
- `Makefile` with `build`, `test`, `test-functional`, `lint`, `fmt`, `tidy`, `clean`, `version`, `version-increment`, `push`, `help`
- `DESIGN.md` — architecture and API design document
