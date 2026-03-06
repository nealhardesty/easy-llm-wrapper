package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ollamaProvider struct {
	baseURL string
	client  *http.Client
}

func newOllamaProvider(baseURL string) *ollamaProvider {
	return &ollamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

// Ollama /api/chat wire types.

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // base64-encoded, no data-URI prefix
}

type ollamaChunk struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

func (o *ollamaProvider) buildMessages(req Request) []ollamaMessage {
	var msgs []ollamaMessage

	if req.System != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: req.System})
	}

	for _, m := range req.Messages {
		msg := ollamaMessage{Role: string(m.Role)}
		for _, p := range m.Parts {
			switch p.Type {
			case PartTypeText:
				msg.Content += p.Text
			case PartTypeImage:
				msg.Images = append(msg.Images, p.base64Encoded())
			}
		}
		msgs = append(msgs, msg)
	}

	return msgs
}

func (o *ollamaProvider) do(ctx context.Context, body ollamaRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	return resp, nil
}

func (o *ollamaProvider) complete(ctx context.Context, model string, req Request) (*Response, error) {
	resp, err := o.do(ctx, ollamaRequest{
		Model:    model,
		Messages: o.buildMessages(req),
		Stream:   false,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chunk ollamaChunk
	if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	return &Response{Text: chunk.Message.Content, Model: chunk.Model}, nil
}

func (o *ollamaProvider) stream(ctx context.Context, model string, req Request) (*StreamResponse, error) {
	resp, err := o.do(ctx, ollamaRequest{
		Model:    model,
		Messages: o.buildMessages(req),
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}

	return newStreamResponse(resp.Body, parseOllamaChunk), nil
}

func parseOllamaChunk(line []byte) (string, bool, error) {
	var chunk ollamaChunk
	if err := json.Unmarshal(line, &chunk); err != nil {
		return "", false, fmt.Errorf("ollama: parse chunk: %w", err)
	}
	if chunk.Done {
		return "", true, nil
	}
	return chunk.Message.Content, false, nil
}
