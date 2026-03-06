package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterComplete_TextOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}

		var req openRouterRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			t.Error("stream should be false for Complete")
		}

		// Single text-only message should use string content.
		if len(req.Messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(req.Messages))
		}
		if _, ok := req.Messages[0].Content.(string); !ok {
			t.Errorf("single text message content should be string, got %T", req.Messages[0].Content)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openRouterResponse{
			Model: "anthropic/claude-3-haiku",
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "The answer is 4."}}},
		})
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "test-key")
	resp, err := p.complete(context.Background(), "anthropic/claude-3-haiku", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("What is 2+2?")}}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
	if resp.Text != "The answer is 4." {
		t.Errorf("Text = %q, want %q", resp.Text, "The answer is 4.")
	}
}

func TestOpenRouterComplete_WithSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openRouterRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Messages) < 2 {
			t.Fatalf("expected system + user message, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("first role = %q, want system", req.Messages[0].Role)
		}
		if req.Messages[0].Content != "You are helpful." {
			t.Errorf("system content = %v, want %q", req.Messages[0].Content, "You are helpful.")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openRouterResponse{
			Model: "m",
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "ok"}}},
		})
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "k")
	_, err := p.complete(context.Background(), "m", Request{
		System:   "You are helpful.",
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
}

func TestOpenRouterComplete_MultiModal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openRouterRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Multi-part message should use array content, not string.
		if len(req.Messages) != 1 {
			t.Fatalf("messages = %d, want 1", len(req.Messages))
		}
		parts, ok := req.Messages[0].Content.([]interface{})
		if !ok {
			t.Fatalf("multi-modal content should be []interface{}, got %T", req.Messages[0].Content)
		}
		if len(parts) != 2 {
			t.Errorf("parts = %d, want 2 (image + text)", len(parts))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openRouterResponse{
			Model: "m",
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "a dog"}}},
		})
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "k")
	_, err := p.complete(context.Background(), "m", Request{
		Messages: []Message{{
			Role: RoleUser,
			Parts: []Part{
				ImagePart("image/jpeg", []byte{0xff, 0xd8, 0xff}),
				TextPart("What animal is this?"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("complete() error: %v", err)
	}
}

func TestOpenRouterComplete_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "bad-key")
	_, err := p.complete(context.Background(), "m", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want it to contain 401", err.Error())
	}
}

func TestOpenRouterStream_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openRouterRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("stream should be true for Stream call")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)

		for _, word := range []string{"Hello", " ", "world"} {
			chunk := openRouterStreamChunk{}
			chunk.Choices = []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			}{{Delta: struct {
				Content string `json:"content"`
			}{Content: word}}}
			b, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", b)
			f.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		f.Flush()
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "k")
	sr, err := p.stream(context.Background(), "m", Request{
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

func TestOpenRouterStream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exceeded", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := newOpenRouterProvider(srv.URL, "k")
	_, err := p.stream(context.Background(), "m", Request{
		Messages: []Message{{Role: RoleUser, Parts: []Part{TextPart("hi")}}},
	})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

// parseOpenRouterChunk tests

func TestParseOpenRouterChunk_DataLine(t *testing.T) {
	chunk := openRouterStreamChunk{}
	chunk.Choices = []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	}{{Delta: struct {
		Content string `json:"content"`
	}{Content: "hi"}}}
	b, _ := json.Marshal(chunk)
	line := []byte("data: " + string(b))

	text, done, err := parseOpenRouterChunk(line)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if done {
		t.Error("done should be false")
	}
	if text != "hi" {
		t.Errorf("text = %q, want %q", text, "hi")
	}
}

func TestParseOpenRouterChunk_Done(t *testing.T) {
	_, done, err := parseOpenRouterChunk([]byte("data: [DONE]"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !done {
		t.Error("done should be true")
	}
}

func TestParseOpenRouterChunk_NonDataLine(t *testing.T) {
	text, done, err := parseOpenRouterChunk([]byte("event: message"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if done {
		t.Error("non-data line should not be done")
	}
	if text != "" {
		t.Errorf("non-data line should return empty text, got %q", text)
	}
}

func TestParseOpenRouterChunk_InvalidJSON(t *testing.T) {
	_, _, err := parseOpenRouterChunk([]byte("data: not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// Compile-time check that openRouterProvider implements provider.
var _ provider = (*openRouterProvider)(nil)
