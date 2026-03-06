package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaComplete_TextOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/chat") {
			t.Errorf("path = %q, want /api/chat", r.URL.Path)
		}

		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Stream {
			t.Error("stream should be false for Complete")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChunk{
			Model:   req.Model,
			Message: ollamaMessage{Role: "assistant", Content: "Hello, world!"},
			Done:    true,
		})
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	resp, err := p.complete(context.Background(), "llama3.2", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("Hi")}}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
	if resp.Text != "Hello, world!" {
		t.Errorf("Text = %q, want %q", resp.Text, "Hello, world!")
	}
}

func TestOllamaComplete_WithSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Messages) < 2 {
			t.Errorf("expected at least 2 messages (system + user), got %d", len(req.Messages))
		} else {
			if req.Messages[0].Role != "system" {
				t.Errorf("first message role = %q, want system", req.Messages[0].Role)
			}
			if req.Messages[0].Content != "Be helpful." {
				t.Errorf("system content = %q, want %q", req.Messages[0].Content, "Be helpful.")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChunk{
			Model:   "llama3.2",
			Message: ollamaMessage{Role: "assistant", Content: "ok"},
			Done:    true,
		})
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	_, err := p.complete(context.Background(), "llama3.2", Request{
		System:   "Be helpful.",
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("Hi")}}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
}

func TestOllamaComplete_WithImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)

		var userMsg *ollamaMessage
		for _, m := range req.Messages {
			if m.Role == "user" {
				m := m
				userMsg = &m
				break
			}
		}
		if userMsg == nil {
			t.Error("no user message found")
		} else if len(userMsg.Images) != 1 {
			t.Errorf("images count = %d, want 1", len(userMsg.Images))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChunk{
			Model:   "llava",
			Message: ollamaMessage{Role: "assistant", Content: "a cat"},
			Done:    true,
		})
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	_, err := p.complete(context.Background(), "llava", Request{
		Messages: []Message{{
			Role: RoleUser,
			Parts: []Part{
				ImagePart("image/png", []byte{0x89, 0x50, 0x4e, 0x47}),
				TextPart("What is this?"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
}

func TestOllamaComplete_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	_, err := p.complete(context.Background(), "bad-model", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want it to contain 404", err.Error())
	}
}

func TestOllamaStream_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("stream should be true for Stream call")
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		f := w.(http.Flusher)

		for _, word := range []string{"Hello", " ", "world"} {
			fmt.Fprintf(w, "%s\n", mustJSON(ollamaChunk{
				Model:   "llama3.2",
				Message: ollamaMessage{Role: "assistant", Content: word},
				Done:    false,
			}))
			f.Flush()
		}
		fmt.Fprintf(w, "%s\n", mustJSON(ollamaChunk{
			Model:   "llama3.2",
			Message: ollamaMessage{Role: "assistant", Content: ""},
			Done:    true,
		}))
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	sr, err := p.stream(context.Background(), "llama3.2", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("Hi")}}},
	})
	if err != nil {
		t.Fatalf("stream() error: %v", err)
	}
	defer sr.Close()

	var got strings.Builder
	for sr.Next() {
		got.WriteString(sr.Chunk())
	}
	if sr.Err() != nil {
		t.Fatalf("stream error: %v", sr.Err())
	}
	if got.String() != "Hello world" {
		t.Errorf("streamed text = %q, want %q", got.String(), "Hello world")
	}
}

func TestOllamaStream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL, nil)
	_, err := p.stream(context.Background(), "m", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestOllamaBaseURL_TrailingSlashStripped(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaChunk{
			Model:   "m",
			Message: ollamaMessage{Role: "assistant", Content: "ok"},
			Done:    true,
		})
	}))
	defer srv.Close()

	p := newOllamaProvider(srv.URL+"/", nil) // trailing slash
	_, err := p.complete(context.Background(), "m", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
	if !called {
		t.Error("server was never called")
	}
}

// parseOllamaChunk tests

func TestParseOllamaChunk_Content(t *testing.T) {
	line := mustJSON(ollamaChunk{
		Model:   "llama3.2",
		Message: ollamaMessage{Role: "assistant", Content: "hi"},
		Done:    false,
	})
	chunk, done, err := parseOllamaChunk([]byte(line))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if done {
		t.Error("done should be false")
	}
	if chunk != "hi" {
		t.Errorf("chunk = %q, want %q", chunk, "hi")
	}
}

func TestParseOllamaChunk_Done(t *testing.T) {
	line := mustJSON(ollamaChunk{Done: true})
	_, done, err := parseOllamaChunk([]byte(line))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !done {
		t.Error("done should be true")
	}
}

func TestParseOllamaChunk_InvalidJSON(t *testing.T) {
	_, _, err := parseOllamaChunk([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// mustJSON marshals v to a JSON string, panicking on error.
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Compile-time check that ollamaProvider implements provider.
var _ provider = (*ollamaProvider)(nil)

// Compile-time check that StreamResponse.Close satisfies io.Closer.
var _ io.Closer = (*StreamResponse)(nil)
