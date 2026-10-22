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
	body := io.NopCloser(strings.NewReader("chunk1\n\n  chunk2  \nchunk3\nDONE\nafter\n"))
	sr := newStreamResponse(body, func(line []byte) (string, bool, error) {
		if string(line) == "DONE" {
			return "", true, nil
		}
		return "<" + string(line) + ">", false, nil
	})
	defer sr.Close()

	var got []string
	for sr.Next() {
		got = append(got, sr.Chunk())
	}
	if sr.Err() != nil {
		t.Fatalf("Err() = %v", sr.Err())
	}
	want := []string{"<chunk1>", "<chunk2>", "<chunk3>"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("chunks = %q, want %q", got, want)
	}
	if sr.Next() {
		t.Error("Next() after done marker should stay false")
	}
}

func TestStreamResponse_LongLine(t *testing.T) {
	long := strings.Repeat("x", 1<<20) // well past bufio.Scanner's 64 KB default
	body := io.NopCloser(strings.NewReader(long + "\n"))
	sr := newStreamResponse(body, func(line []byte) (string, bool, error) {
		return string(line), false, nil
	})
	defer sr.Close()

	if !sr.Next() {
		t.Fatalf("Next() = false, Err() = %v", sr.Err())
	}
	if len(sr.Chunk()) != len(long) {
		t.Errorf("chunk length = %d, want %d", len(sr.Chunk()), len(long))
	}
}

func TestStreamResponse_AtEOF(t *testing.T) {
	body := io.NopCloser(strings.NewReader("a\n"))
	sr := newStreamResponse(body, func(line []byte) (string, bool, error) {
		return string(line), false, nil
	})
	sr.atEOF = func() error { return errors.New("truncated") }
	defer sr.Close()

	for sr.Next() {
	}
	if sr.Err() == nil || sr.Err().Error() != "truncated" {
		t.Errorf("Err() = %v, want truncated", sr.Err())
	}
}

func TestStreamResponse_ParseErrorStops(t *testing.T) {
	body := io.NopCloser(strings.NewReader("a\nbad\nc\n"))
	sr := newStreamResponse(body, func(line []byte) (string, bool, error) {
		if string(line) == "bad" {
			return "", false, errors.New("boom")
		}
		return string(line), false, nil
	})
	defer sr.Close()

	n := 0
	for sr.Next() {
		n++
	}
	if n != 1 || sr.Err() == nil {
		t.Errorf("chunks = %d, Err() = %v; want 1 chunk then boom", n, sr.Err())
	}
	if sr.Next() {
		t.Error("Next() after error should stay false")
	}
}
