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
	Model      string              `json:"model"`
	Messages   []openRouterMessage `json:"messages"`
	Stream     bool                `json:"stream"`
	Modalities []string            `json:"modalities,omitempty"`
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

// openRouterError is the error object OpenRouter can embed in a 200 response
// or a mid-stream SSE event.
type openRouterError struct {
	Message string `json:"message"`
}

type openRouterResponse struct {
	Model   string           `json:"model"`
	Error   *openRouterError `json:"error"`
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
	Error   *openRouterError `json:"error"`
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
		Model:      model,
		Messages:   o.buildMessages(req),
		Stream:     false,
		Modalities: req.Modalities,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openrouter: decode response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openrouter: %s", result.Error.Message)
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
		if p.Type != "image_url" || p.ImageURL == nil {
			continue
		}
		img, err := imageFromURL(p.ImageURL.URL)
		if err != nil {
			return nil, fmt.Errorf("openrouter: image in message.images: %w", err)
		}
		images = append(images, img)
	}

	return &Response{Text: text, Model: result.Model, Images: images}, nil
}

func (o *openRouterProvider) stream(ctx context.Context, model string, req Request) (*StreamResponse, error) {
	resp, err := o.do(ctx, openRouterRequest{
		Model:      model,
		Messages:   o.buildMessages(req),
		Stream:     true,
		Modalities: req.Modalities,
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
	// Absent or null content (e.g. image-only or tool-call responses).
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil, nil
	}

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
			img, imgErr := imageFromURL(p.ImageURL.URL)
			if imgErr != nil {
				return "", nil, fmt.Errorf("openrouter: image in response: %w", imgErr)
			}
			images = append(images, img)
		}
	}
	return strings.Join(texts, ""), images, nil
}

// imageFromURL converts a returned image URL into an image Part. Data URIs are
// decoded in place; anything else (e.g. an https link) is reported clearly
// rather than as a decode failure, since Part carries bytes, not URLs.
func imageFromURL(url string) (Part, error) {
	if !strings.HasPrefix(url, "data:") {
		return Part{}, fmt.Errorf("unsupported image URL (only data URIs are supported): %.80s", url)
	}
	mimeType, data, err := parseDataURI(url)
	if err != nil {
		return Part{}, err
	}
	return ImagePart(mimeType, data), nil
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
	const prefix = "data:"
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return "", false, nil // skip event/id/comment lines
	}

	// The space after "data:" is optional per the SSE spec.
	payload := bytes.TrimPrefix(line[len(prefix):], []byte(" "))
	if string(payload) == "[DONE]" {
		return "", true, nil
	}

	var chunk openRouterStreamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return "", false, fmt.Errorf("openrouter: parse chunk: %w", err)
	}
	if chunk.Error != nil {
		return "", true, fmt.Errorf("openrouter: %s", chunk.Error.Message)
	}
	if len(chunk.Choices) == 0 {
		return "", false, nil
	}
	if fr := chunk.Choices[0].FinishReason; fr != nil && *fr == "error" {
		return "", true, fmt.Errorf("openrouter: stream finished with error")
	}

	return chunk.Choices[0].Delta.Content, false, nil
}
