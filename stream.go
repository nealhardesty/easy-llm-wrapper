package llm

import (
	"bufio"
	"bytes"
	"io"
)

// maxLineSize caps a single streamed line. Providers can emit very long lines
// (minified JSON, base64 image data), far beyond bufio.Scanner's 64 KB default.
const maxLineSize = 16 << 20

// lineParser parses a single raw line from a streaming response.
// Returns the text chunk, whether the stream is done, and any parse error.
type lineParser func(line []byte) (chunk string, done bool, err error)

// StreamResponse provides incremental access to generated text chunks.
// Usage mirrors bufio.Scanner: call Next in a loop, read Chunk on true,
// check Err after Next returns false. Always call Close when done.
type StreamResponse struct {
	scanner  *bufio.Scanner
	body     io.ReadCloser
	parse    lineParser
	atEOF    func() error // optional; reports why the stream ended without a done marker
	current  string
	err      error
	finished bool
}

func newStreamResponse(body io.ReadCloser, parse lineParser) *StreamResponse {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), maxLineSize)
	return &StreamResponse{
		scanner: scanner,
		body:    body,
		parse:   parse,
	}
}

// Next advances to the next non-empty chunk. Returns false when the stream
// is exhausted or an error occurs; once false, it stays false.
func (s *StreamResponse) Next() bool {
	if s.finished {
		return false
	}
	for s.scanner.Scan() {
		line := bytes.TrimSpace(s.scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		chunk, done, err := s.parse(line)
		if err != nil {
			return s.finish(err)
		}
		if done {
			return s.finish(nil)
		}
		if chunk == "" {
			continue
		}
		s.current = chunk
		return true
	}
	if err := s.scanner.Err(); err != nil {
		return s.finish(err)
	}
	if s.atEOF != nil {
		return s.finish(s.atEOF())
	}
	return s.finish(nil)
}

func (s *StreamResponse) finish(err error) bool {
	s.finished = true
	s.current = ""
	s.err = err
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

// Close releases the underlying response body (or subprocess).
func (s *StreamResponse) Close() error {
	return s.body.Close()
}
