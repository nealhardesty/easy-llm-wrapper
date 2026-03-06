package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openRouterProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newOpenRouterProvider(baseURL, apiKey string, transport http.RoundTripper) *openRouterProvider {
	return &openRouterProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Transport: transport},
	}
}

// OpenRouter uses the OpenAI-compatible wire format.

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

// openRouterMessage.Content is either a plain string (text-only) or a slice
// of content parts (multi-modal). Using interface{} keeps both valid JSON.
type openRouterMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type openRouterContentPart struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *openRouterImageURL `json:"image_url,omitempty"`
}

type openRouterImageURL struct {
	URL string `json:"url"`
}

type openRouterResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			// Content is a plain string for text responses or a JSON array of
			// content parts for multi-modal responses (text + images).
			Content json.RawMessage `json:"content"`
			// Images holds generated images returned by image-generation models
			// (e.g. google/gemini-*-image-*). OpenRouter places them here rather
			// than inside Content.
			Images []openRouterContentPart `json:"images"`
		} `json:"message"`
	} `json:"choices"`
}

type openRouterStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (o *openRouterProvider) buildMessages(req Request) []openRouterMessage {
	var msgs []openRouterMessage

	if req.System != "" {
		msgs = append(msgs, openRouterMessage{Role: "system", Content: req.System})
	}

	for _, m := range req.Messages {
		// Single text-only part: use simple string content for maximum compatibility.
		if len(m.Parts) == 1 && m.Parts[0].Type == PartTypeText {
			msgs = append(msgs, openRouterMessage{
				Role:    string(m.Role),
				Content: m.Parts[0].Text,
			})
			continue
		}

		// Multi-part or image: build content part array.
		parts := make([]openRouterContentPart, 0, len(m.Parts))
		for _, p := range m.Parts {
			switch p.Type {
			case PartTypeText:
				parts = append(parts, openRouterContentPart{Type: "text", Text: p.Text})
			case PartTypeImage:
				dataURL := fmt.Sprintf("data:%s;base64,%s", p.MIMEType, p.base64Encoded())
				parts = append(parts, openRouterContentPart{
					Type:     "image_url",
					ImageURL: &openRouterImageURL{URL: dataURL},
				})
			}
		}
		msgs = append(msgs, openRouterMessage{Role: string(m.Role), Content: parts})
	}

	return msgs
}

func (o *openRouterProvider) do(ctx context.Context, body openRouterRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("openrouter: status %d: %s", resp.StatusCode, bytes.TrimSpace(errBody))
	}

	return resp, nil
}

func (o *openRouterProvider) complete(ctx context.Context, model string, req Request) (*Response, error) {
	resp, err := o.do(ctx, openRouterRequest{
		Model:    model,
		Messages: o.buildMessages(req),
		Stream:   false,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openrouter: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openrouter: no choices in response")
	}

	text, images, err := parseOpenRouterResponseContent(result.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	// Image-generation models (e.g. gemini-*-image-*) return images in
	// message.images rather than inside message.content.
	for _, p := range result.Choices[0].Message.Images {
		if p.Type == "image_url" {
			mimeType, data, parseErr := parseDataURI(p.ImageURL.URL)
			if parseErr != nil {
				return nil, fmt.Errorf("openrouter: parse image in message.images: %w", parseErr)
			}
			images = append(images, ImagePart(mimeType, data))
		}
	}

	return &Response{Text: text, Model: result.Model, Images: images}, nil
}

func (o *openRouterProvider) stream(ctx context.Context, model string, req Request) (*StreamResponse, error) {
	resp, err := o.do(ctx, openRouterRequest{
		Model:    model,
		Messages: o.buildMessages(req),
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}

	return newStreamResponse(resp.Body, parseOpenRouterChunk), nil
}

// parseOpenRouterResponseContent parses the content field from a non-streaming
// OpenRouter response. Content may be a plain string or an array of typed parts
// (text and/or image_url entries for multi-modal responses).
func parseOpenRouterResponseContent(raw json.RawMessage) (text string, images []Part, err error) {
	// Try plain string first (text-only response).
	var s string
	if jsonErr := json.Unmarshal(raw, &s); jsonErr == nil {
		return s, nil, nil
	}

	// Array of content parts (multi-modal response).
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if jsonErr := json.Unmarshal(raw, &parts); jsonErr != nil {
		return "", nil, fmt.Errorf("openrouter: parse response content: %w", jsonErr)
	}

	var texts []string
	for _, p := range parts {
		switch p.Type {
		case "text":
			texts = append(texts, p.Text)
		case "image_url":
			mimeType, data, parseErr := parseDataURI(p.ImageURL.URL)
			if parseErr != nil {
				return "", nil, fmt.Errorf("openrouter: parse image in response: %w", parseErr)
			}
			images = append(images, ImagePart(mimeType, data))
		}
	}
	return strings.Join(texts, ""), images, nil
}

// parseDataURI decodes a data URI of the form "data:<mime>;base64,<data>"
// and returns the MIME type and raw bytes.
func parseDataURI(uri string) (mimeType string, data []byte, err error) {
	if !strings.HasPrefix(uri, "data:") {
		return "", nil, fmt.Errorf("not a data URI")
	}
	rest := uri[len("data:"):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, fmt.Errorf("invalid data URI: missing comma")
	}
	meta := rest[:comma]
	encoded := rest[comma+1:]

	metaParts := strings.Split(meta, ";")
	mimeType = metaParts[0]
	if len(metaParts) < 2 || metaParts[len(metaParts)-1] != "base64" {
		return "", nil, fmt.Errorf("only base64 data URIs are supported")
	}

	data, err = base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("decode base64 data URI: %w", err)
	}
	return mimeType, data, nil
}

// parseOpenRouterChunk handles the SSE "data: {...}" line format.
func parseOpenRouterChunk(line []byte) (string, bool, error) {
	const prefix = "data: "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return "", false, nil // skip event/id/comment lines
	}

	payload := line[len(prefix):]
	if string(payload) == "[DONE]" {
		return "", true, nil
	}

	var chunk openRouterStreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return "", false, fmt.Errorf("openrouter: parse chunk: %w", err)
	}
	if len(chunk.Choices) == 0 {
		return "", false, nil
	}

	return chunk.Choices[0].Delta.Content, false, nil
}
