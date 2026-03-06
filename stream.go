package llm

import (
	"bufio"
	"bytes"
	"io"
)

// lineParser parses a single raw line from a streaming response.
// Returns the text chunk, whether the stream is done, and any parse error.
type lineParser func(line []byte) (chunk string, done bool, err error)

// StreamResponse provides incremental access to generated text chunks.
// Usage mirrors bufio.Scanner: call Next in a loop, read Chunk on true,
// check Err after Next returns false. Always call Close when done.
type StreamResponse struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
	parse   lineParser
	current string
	err     error
}

func newStreamResponse(body io.ReadCloser, parse lineParser) *StreamResponse {
	return &StreamResponse{
		scanner: bufio.NewScanner(body),
		body:    body,
		parse:   parse,
	}
}

// Next advances to the next non-empty chunk. Returns false when the stream
// is exhausted or an error occurs.
func (s *StreamResponse) Next() bool {
	for s.scanner.Scan() {
		line := bytes.TrimSpace(s.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		chunk, done, err := s.parse(line)
		if err != nil {
			s.err = err
			return false
		}
		if done {
			return false
		}
		if chunk == "" {
			continue
		}
		s.current = chunk
		return true
	}
	if err := s.scanner.Err(); err != nil {
		s.err = err
	}
	return false
}

// Chunk returns the current text chunk. Only valid after Next returns true.
func (s *StreamResponse) Chunk() string {
	return s.current
}

// Err returns any error encountered during streaming. Check after Next returns false.
func (s *StreamResponse) Err() error {
	return s.err
}

// Close releases the underlying HTTP response body.
func (s *StreamResponse) Close() error {
	return s.body.Close()
}
