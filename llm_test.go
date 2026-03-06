package llm

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// mockProvider implements provider for unit testing Client methods.
type mockProvider struct {
	resp       *Response
	respErr    error
	streamResp *StreamResponse
	streamErr  error

	lastModel string
	lastReq   Request
}

func (m *mockProvider) complete(_ context.Context, model string, req Request) (*Response, error) {
	m.lastModel = model
	m.lastReq = req
	return m.resp, m.respErr
}

func (m *mockProvider) stream(_ context.Context, model string, req Request) (*StreamResponse, error) {
	m.lastModel = model
	m.lastReq = req
	return m.streamResp, m.streamErr
}

func newTestClient(p provider, model string) *Client {
	return &Client{
		cfg:      Config{Provider: ProviderOllama, Model: model},
		provider: p,
	}
}

func TestClient_Provider(t *testing.T) {
	c := &Client{cfg: Config{Provider: ProviderOpenRouter}}
	if c.Provider() != ProviderOpenRouter {
		t.Errorf("Provider() = %q, want %q", c.Provider(), ProviderOpenRouter)
	}
}

func TestClient_Model(t *testing.T) {
	c := &Client{cfg: Config{Model: "llama3.2"}}
	if c.Model() != "llama3.2" {
		t.Errorf("Model() = %q, want %q", c.Model(), "llama3.2")
	}
}

func TestClient_Ask(t *testing.T) {
	mock := &mockProvider{resp: &Response{Text: "42", Model: "test-model"}}
	c := newTestClient(mock, "test-model")

	got, err := c.Ask(context.Background(), "sys", "user input")
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if got != "42" {
		t.Errorf("Ask() = %q, want %q", got, "42")
	}
	if mock.lastReq.System != "sys" {
		t.Errorf("system = %q, want %q", mock.lastReq.System, "sys")
	}
	if len(mock.lastReq.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(mock.lastReq.Messages))
	}
	if mock.lastReq.Messages[0].Role != RoleUser {
		t.Errorf("role = %q, want %q", mock.lastReq.Messages[0].Role, RoleUser)
	}
	if mock.lastReq.Messages[0].Parts[0].Text != "user input" {
		t.Errorf("text = %q, want %q", mock.lastReq.Messages[0].Parts[0].Text, "user input")
	}
}

func TestClient_Ask_EmptySystem(t *testing.T) {
	mock := &mockProvider{resp: &Response{Text: "ok"}}
	c := newTestClient(mock, "m")

	_, err := c.Ask(context.Background(), "", "hello")
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if mock.lastReq.System != "" {
		t.Errorf("expected empty system, got %q", mock.lastReq.System)
	}
}

func TestClient_Ask_PropagatesError(t *testing.T) {
	sentinel := errors.New("backend failure")
	mock := &mockProvider{respErr: sentinel}
	c := newTestClient(mock, "m")

	_, err := c.Ask(context.Background(), "", "hi")
	if !errors.Is(err, sentinel) {
		t.Errorf("Ask() error = %v, want %v", err, sentinel)
	}
}

func TestClient_Complete_PassesModel(t *testing.T) {
	mock := &mockProvider{resp: &Response{Text: "result", Model: "mymodel"}}
	c := newTestClient(mock, "mymodel")

	req := Request{Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}}}
	_, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if mock.lastModel != "mymodel" {
		t.Errorf("model passed = %q, want %q", mock.lastModel, "mymodel")
	}
}

func TestClient_Stream_PassesRequest(t *testing.T) {
	body := io.NopCloser(strings.NewReader(""))
	sr := newStreamResponse(body, func([]byte) (string, bool, error) { return "", true, nil })
	mock := &mockProvider{streamResp: sr}
	c := newTestClient(mock, "m")

	req := Request{Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}}}
	got, err := c.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if got != sr {
		t.Error("Stream() did not return the provider's StreamResponse")
	}
	got.Close()
}

func TestNewClientWithConfig_UnknownProvider(t *testing.T) {
	_, err := NewClientWithConfig(Config{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestTextPart(t *testing.T) {
	p := TextPart("hello")
	if p.Type != PartTypeText {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeText)
	}
	if p.Text != "hello" {
		t.Errorf("Text = %q, want %q", p.Text, "hello")
	}
}

func TestImagePart(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4e, 0x47}
	p := ImagePart("image/png", data)
	if p.Type != PartTypeImage {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeImage)
	}
	if p.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want %q", p.MIMEType, "image/png")
	}
	if string(p.Data) != string(data) {
		t.Error("Data mismatch")
	}
}

func TestStreamResponse_Basic(t *testing.T) {
	chunks := "chunk1\nchunk2\nchunk3\n"
	body := io.NopCloser(strings.NewReader(chunks))

	idx := 0
	lines := []string{"chunk1", "chunk2", "chunk3"}
	sr := newStreamResponse(body, func(line []byte) (string, bool, error) {
		if idx >= len(lines) {
			return "", true, nil
		}
		s := lines[idx]
		idx++
		return s, false, nil
	})
	defer sr.Close()

	var got []string
	for sr.Next() {
		got = append(got, sr.Chunk())
	}
	if sr.Err() != nil {
		t.Fatalf("Err() = %v", sr.Err())
	}
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3", len(got))
	}
	for i, want := range lines {
		if got[i] != want {
			t.Errorf("chunk[%d] = %q, want %q", i, got[i], want)
		}
	}
}
