package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeClaudeScript emulates `claude -p --output-format stream-json`. It saves
// its argv and stdin under $FAKE_CLAUDE_DIR, then behaves per $FAKE_CLAUDE_MODE.
const fakeClaudeScript = `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_CLAUDE_DIR/args"
cat > "$FAKE_CLAUDE_DIR/stdin"
case "$FAKE_CLAUDE_MODE" in
ok)
  printf '%s\n' '{"type":"system","subtype":"init","model":"claude-sonnet-test"}'
  printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"line1\n\n"}}}'
  printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"    indented\nline3"}}}'
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"line1\n\n    indented\nline3"}'
  ;;
result_error)
  printf '%s\n' '{"type":"result","subtype":"success","is_error":true,"result":"There is an issue with the selected model (bogus)."}'
  exit 1
  ;;
crash)
  echo "Error: something broke" >&2
  exit 3
  ;;
no_result)
  printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}}'
  ;;
slow)
  printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"first"}}}'
  exec sleep 10
  ;;
esac
`

// installFakeClaude points lookPath at a fake claude script for the test.
func installFakeClaude(t *testing.T, mode string) (dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake claude script requires a POSIX shell")
	}
	dir = t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(fakeClaudeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_DIR", dir)
	t.Setenv("FAKE_CLAUDE_MODE", mode)
	orig := lookPath
	lookPath = func(name string) (string, error) { return bin, nil }
	t.Cleanup(func() { lookPath = orig })
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func textRequest(system, user string) Request {
	return Request{System: system, Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart(user)}}}}
}

const wantMultiline = "line1\n\n    indented\nline3"

func TestClaudeStream_PreservesWhitespace(t *testing.T) {
	installFakeClaude(t, "ok")
	sr, err := newClaudeProvider(false).stream(context.Background(), "sonnet", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer sr.Close()

	var sb strings.Builder
	chunks := 0
	for sr.Next() {
		sb.WriteString(sr.Chunk())
		chunks++
	}
	if err := sr.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if chunks != 2 {
		t.Errorf("chunks = %d, want 2", chunks)
	}
	if sb.String() != wantMultiline {
		t.Errorf("text = %q, want %q", sb.String(), wantMultiline)
	}
}

func TestClaudeComplete_TextAndModel(t *testing.T) {
	installFakeClaude(t, "ok")
	resp, err := newClaudeProvider(false).complete(context.Background(), "sonnet", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if resp.Text != wantMultiline {
		t.Errorf("Text = %q, want %q", resp.Text, wantMultiline)
	}
	if resp.Model != "claude-sonnet-test" {
		t.Errorf("Model = %q, want %q", resp.Model, "claude-sonnet-test")
	}
}

func TestClaudeComplete_PromptAndImagesViaStdin(t *testing.T) {
	dir := installFakeClaude(t, "ok")
	req := Request{
		System: "Be terse.",
		Messages: []Message{{Role: RoleUser, Parts: []Part{
			TextPart("--looks-like-a-flag"),
			ImagePart("image/png", []byte{0x89, 0x50, 0x4e, 0x47}),
		}}},
	}
	if _, err := newClaudeProvider(false).complete(context.Background(), "opus", req); err != nil {
		t.Fatalf("complete: %v", err)
	}

	args := readFile(t, filepath.Join(dir, "args"))
	if strings.Contains(args, "--looks-like-a-flag") {
		t.Error("prompt must be sent on stdin, not argv")
	}
	for _, want := range []string{"--system-prompt\nBe terse.\n", "--model\nopus\n", "--input-format\nstream-json\n"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q:\n%s", want, args)
		}
	}

	var in claudeInput
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "stdin"))), &in); err != nil {
		t.Fatalf("stdin is not a stream-json message: %v", err)
	}
	blocks := in.Message.Content
	if len(blocks) != 2 {
		t.Fatalf("content blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "--looks-like-a-flag" {
		t.Errorf("block[0] = %+v", blocks[0])
	}
	if blocks[1].Type != "image" || blocks[1].Source == nil ||
		blocks[1].Source.MediaType != "image/png" || blocks[1].Source.Data != "iVBORw==" {
		t.Errorf("block[1] = %+v", blocks[1])
	}
}

func TestClaudeComplete_ResultError(t *testing.T) {
	installFakeClaude(t, "result_error")
	_, err := newClaudeProvider(false).complete(context.Background(), "bogus", textRequest("", "hi"))
	if err == nil || !strings.Contains(err.Error(), "selected model (bogus)") {
		t.Fatalf("err = %v, want the CLI's error message", err)
	}
}

func TestClaudeStream_ProcessFailureIncludesStderr(t *testing.T) {
	installFakeClaude(t, "crash")
	sr, err := newClaudeProvider(false).stream(context.Background(), "", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer sr.Close()
	for sr.Next() {
	}
	if err := sr.Err(); err == nil || !strings.Contains(err.Error(), "something broke") {
		t.Fatalf("Err() = %v, want error containing stderr", err)
	}
}

func TestClaudeStream_MissingResultIsError(t *testing.T) {
	installFakeClaude(t, "no_result")
	sr, err := newClaudeProvider(false).stream(context.Background(), "", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer sr.Close()
	for sr.Next() {
	}
	if sr.Err() == nil {
		t.Fatal("expected an error when the CLI exits without a result event")
	}
}

func TestClaudeStream_ContextCancel(t *testing.T) {
	installFakeClaude(t, "slow")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	sr, err := newClaudeProvider(false).stream(ctx, "", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer sr.Close()
	for sr.Next() {
	}
	if err := sr.Err(); err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("Err() = %v, want deadline exceeded", err)
	}
}

func TestClaudeStream_CloseKillsProcess(t *testing.T) {
	installFakeClaude(t, "slow")
	sr, err := newClaudeProvider(false).stream(context.Background(), "", textRequest("", "hi"))
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if !sr.Next() || sr.Chunk() != "first" {
		t.Fatalf("first chunk = %q, err = %v", sr.Chunk(), sr.Err())
	}
	start := time.Now()
	sr.Close()
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Close took %s; should kill the subprocess promptly", d)
	}
}
