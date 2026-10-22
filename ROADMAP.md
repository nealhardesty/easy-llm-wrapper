# Roadmap

Things that do not exist yet. When one ships, move its facts into `SPEC.md`
and delete it from here.

## Library

- **Native multi-turn for the claude provider.** History is currently
  flattened into a `Human:`/`Assistant:` transcript inside one user message.
  stream-json input could replay real turns instead.
- **Image output from the claude and Ollama providers.** Only OpenRouter
  populates `Response.Images`.
- **Image URLs in responses.** OpenRouter responses with `https://` image
  URLs are rejected (only `data:` URIs are decoded). Downloading them would
  need an HTTP fetch policy (size limits, timeouts).
- **Usage / token accounting in `Response`.** All three providers report it;
  none of it is surfaced.
- **Truncation detection for HTTP streams.** Claude streams error if they end
  without a result event; OpenRouter/Ollama streams that drop before
  `[DONE]` / `done:true` still end cleanly.
- **Retries with backoff** for transient HTTP errors (429/5xx), opt-in.

## CLIs

- `elw`: image attachments (`-i file.png`) to exercise multi-modal input.
- `elwi`: input images for edit/variation prompts.

## Tooling

- `make push` depends on the external `gitsum` command for the commit
  message; consider vendoring or documenting an install target.
- CI workflow calling `make lint test`.
