package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

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

func (p *claudeProvider) buildArgs(model string, req Request, outputFormat string) []string {
	args := []string{"-p", p.buildPrompt(req)}
	if req.System != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "--output-format", outputFormat)
	return args
}

func (p *claudeProvider) complete(ctx context.Context, model string, req Request) (*Response, error) {
	bin, err := p.resolveBinary()
	if err != nil {
		return nil, err
	}

	args := p.buildArgs(model, req, "text")
	p.debugf("claude exec: %s %v", bin, args)
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		p.debugf("claude exec failed: %v", err)
		return nil, fmt.Errorf("claude: run: %w", err)
	}

	return &Response{Text: strings.TrimSpace(string(out))}, nil
}

func (p *claudeProvider) stream(ctx context.Context, model string, req Request) (*StreamResponse, error) {
	bin, err := p.resolveBinary()
	if err != nil {
		return nil, err
	}

	args := p.buildArgs(model, req, "text")
	p.debugf("claude stream exec: %s %v", bin, args)
	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.debugf("claude stdout pipe failed: %v", err)
		return nil, fmt.Errorf("claude: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		p.debugf("claude start failed: %v", err)
		return nil, fmt.Errorf("claude: start: %w", err)
	}

	body := &cmdReadCloser{ReadCloser: stdout, cmd: cmd}
	return newStreamResponse(body, claudeLineParser), nil
}

// cmdReadCloser wraps a pipe and calls cmd.Wait() on Close to reap the subprocess.
type cmdReadCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	err := c.ReadCloser.Close()
	_ = c.cmd.Wait()
	return err
}

// claudeLineParser passes each line through as-is (text output).
func claudeLineParser(line []byte) (string, bool, error) {
	return string(line), false, nil
}
