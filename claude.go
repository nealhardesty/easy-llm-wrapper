package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// claudeProvider drives the Claude Code CLI in non-interactive print mode.
//
// The request is written to stdin as a single stream-json user message (so
// prompts can be arbitrarily large, start with "-", and carry images), and the
// reply is read as stream-json events with partial messages enabled, which
// gives true token-level streaming with whitespace intact.
type claudeProvider struct {
	debug bool
}

func newClaudeProvider(debug bool) *claudeProvider {
	return &claudeProvider{debug: debug}
}

func (p *claudeProvider) debugf(format string, args ...any) {
	if p.debug {
		fmt.Fprintf(os.Stderr, "[llm debug] "+format+"\n", args...)
	}
}

func (p *claudeProvider) resolveBinary() (string, error) {
	path, err := lookPath("claude")
	if err != nil {
		p.debugf("claude binary lookup failed: %v", err)
		return "", fmt.Errorf("claude: binary not found in PATH: %w", err)
	}
	p.debugf("claude binary resolved to %s", path)
	return path, nil
}

// buildPrompt converts messages into a single prompt string.
// Single user message → text directly; multi-turn → Human:/Assistant: format.
func (p *claudeProvider) buildPrompt(req Request) string {
	if len(req.Messages) == 1 && req.Messages[0].Role == RoleUser {
		return extractText(req.Messages[0])
	}

	var sb strings.Builder
	for i, m := range req.Messages {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		switch m.Role {
		case RoleUser:
			sb.WriteString("Human: ")
		case RoleAssistant:
			sb.WriteString("Assistant: ")
		}
		sb.WriteString(extractText(m))
	}
	return sb.String()
}

func extractText(m Message) string {
	var sb strings.Builder
	for _, p := range m.Parts {
		if p.Type == PartTypeText {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// Claude stream-json wire types (stdin).

type claudeInput struct {
	Type    string             `json:"type"`
	Message claudeInputMessage `json:"message"`
}

type claudeInputMessage struct {
	Role    string               `json:"role"`
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type   string             `json:"type"`
	Text   string             `json:"text,omitempty"`
	Source *claudeImageSource `json:"source,omitempty"`
}

type claudeImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// buildInput renders the request as one stream-json user message: the text
// prompt followed by every image part from the conversation.
func (p *claudeProvider) buildInput(req Request) ([]byte, error) {
	blocks := []claudeContentBlock{{Type: "text", Text: p.buildPrompt(req)}}
	for _, m := range req.Messages {
		for _, part := range m.Parts {
			if part.Type == PartTypeImage {
				blocks = append(blocks, claudeContentBlock{
					Type: "image",
					Source: &claudeImageSource{
						Type:      "base64",
						MediaType: part.MIMEType,
						Data:      part.base64Encoded(),
					},
				})
			}
		}
	}
	data, err := json.Marshal(claudeInput{
		Type:    "user",
		Message: claudeInputMessage{Role: "user", Content: blocks},
	})
	if err != nil {
		return nil, fmt.Errorf("claude: marshal input: %w", err)
	}
	return append(data, '\n'), nil
}

func (p *claudeProvider) buildArgs(model string, req Request) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose", // required by stream-json output in print mode
		"--include-partial-messages",
		"--tools", "", // plain LLM call: no agent tools
		"--strict-mcp-config", // and no MCP servers
		"--no-session-persistence",
	}
	if req.System != "" {
		args = append(args, "--system-prompt", req.System)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

// Claude stream-json wire types (stdout). Only the fields we use are decoded.

type claudeEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Model   string `json:"model"` // system/init
	IsError bool   `json:"is_error"`
	Result  string `json:"result"` // result
	Event   struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	} `json:"event"` // stream_event
}

// claudeStreamState records what the event parser has seen.
type claudeStreamState struct {
	model     string
	result    string
	gotResult bool
}

func (st *claudeStreamState) parse(line []byte) (string, bool, error) {
	var ev claudeEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return "", false, fmt.Errorf("claude: parse event: %w", err)
	}
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" && ev.Model != "" {
			st.model = ev.Model
		}
	case "stream_event":
		if ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta" {
			return ev.Event.Delta.Text, false, nil
		}
	case "result":
		st.gotResult = true
		if ev.IsError {
			msg := strings.TrimSpace(ev.Result)
			if msg == "" {
				msg = ev.Subtype
			}
			return "", true, fmt.Errorf("claude: %s", msg)
		}
		st.result = ev.Result
		return "", true, nil
	}
	return "", false, nil
}

func (p *claudeProvider) start(ctx context.Context, model string, req Request) (*StreamResponse, *claudeStreamState, error) {
	bin, err := p.resolveBinary()
	if err != nil {
		return nil, nil, err
	}
	input, err := p.buildInput(req)
	if err != nil {
		return nil, nil, err
	}

	args := p.buildArgs(model, req)
	p.debugf("claude exec: %s %q", bin, args)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(input)
	proc := &claudeProcess{cmd: cmd, ctx: ctx}
	cmd.Stderr = &proc.stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("claude: stdout pipe: %w", err)
	}
	proc.stdout = stdout
	if err := cmd.Start(); err != nil {
		p.debugf("claude start failed: %v", err)
		return nil, nil, fmt.Errorf("claude: start: %w", err)
	}

	state := &claudeStreamState{}
	sr := newStreamResponse(proc, state.parse)
	sr.atEOF = func() error {
		if err := proc.wait(); err != nil {
			p.debugf("claude exited: %v", err)
			return err
		}
		if !state.gotResult {
			return fmt.Errorf("claude: output ended without a result")
		}
		return nil
	}
	return sr, state, nil
}

func (p *claudeProvider) complete(ctx context.Context, model string, req Request) (*Response, error) {
	sr, state, err := p.start(ctx, model, req)
	if err != nil {
		return nil, err
	}
	defer sr.Close()

	var sb strings.Builder
	for sr.Next() {
		sb.WriteString(sr.Chunk())
	}
	if err := sr.Err(); err != nil {
		return nil, err
	}

	text := state.result
	if text == "" {
		text = sb.String()
	}
	return &Response{Text: text, Model: state.model}, nil
}

func (p *claudeProvider) stream(ctx context.Context, model string, req Request) (*StreamResponse, error) {
	sr, _, err := p.start(ctx, model, req)
	return sr, err
}

// claudeProcess is the read side of a running claude subprocess. Close kills
// the process if it is still running, so abandoning a stream returns promptly.
type claudeProcess struct {
	cmd    *exec.Cmd
	ctx    context.Context
	stdout io.ReadCloser
	stderr bytes.Buffer

	once    sync.Once
	waitErr error
}

func (c *claudeProcess) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

// wait reaps the process once and returns a descriptive error on failure.
func (c *claudeProcess) wait() error {
	c.once.Do(func() {
		err := c.cmd.Wait()
		switch {
		case err == nil:
		case c.ctx.Err() != nil:
			c.waitErr = fmt.Errorf("claude: %w", c.ctx.Err())
		default:
			if msg := lastLine(c.stderr.String()); msg != "" {
				c.waitErr = fmt.Errorf("claude: %w: %s", err, msg)
			} else {
				c.waitErr = fmt.Errorf("claude: %w", err)
			}
		}
	})
	return c.waitErr
}

func (c *claudeProcess) Close() error {
	if c.cmd.ProcessState == nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.wait()
	return nil
}

// lastLine returns the last non-empty line of s, which is where CLIs put the
// actual error after any banners or warnings.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
