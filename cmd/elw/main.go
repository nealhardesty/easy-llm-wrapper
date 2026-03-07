// Command elw is a minimal CLI for querying an LLM using easy-llm-wrapper.
//
// Usage:
//
//	elw [flags] [question]
//
// Provider and model are selected from environment variables (see README).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	llm "github.com/nealhardesty/easy-llm-wrapper"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit (shorthand)")
	debug := flag.Bool("d", false, "enable debug output (provider, model, timing, env)")
	system := flag.String("s", "", "system prompt")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: elw [flags] [question]\n\n")
		fmt.Fprintf(os.Stderr, "Sends a question to the configured LLM and streams the response to stdout.\n")
		fmt.Fprintf(os.Stderr, "If no question is given as an argument, the prompt is read from stdin.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  elw What is the capital of France?\n")
		fmt.Fprintf(os.Stderr, "  elw -s 'You are a poet.' Write a haiku about Go.\n")
		fmt.Fprintf(os.Stderr, "  echo 'Explain goroutines' | elw\n")
		fmt.Fprintf(os.Stderr, "  cat prompt.txt | elw\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  OLLAMA_HOST        Ollama base URL (e.g. http://localhost:11434).\n")
		fmt.Fprintf(os.Stderr, "                     When set, Ollama is used as the provider.\n")
		fmt.Fprintf(os.Stderr, "                     Ollama takes priority over OpenRouter.\n")
		fmt.Fprintf(os.Stderr, "  OPENROUTER_API_KEY OpenRouter API key.\n")
		fmt.Fprintf(os.Stderr, "                     Used when OLLAMA_HOST is not set.\n")
		fmt.Fprintf(os.Stderr, "  MODEL              Override the default model for the active provider.\n")
		fmt.Fprintf(os.Stderr, "                     Default: llama3.2 (Ollama), anthropic/claude-3-haiku (OpenRouter).\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("elw %s\n", llm.Version)
		return
	}

	var question string
	if flag.NArg() > 0 {
		question = strings.Join(flag.Args(), " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		question = strings.TrimSpace(string(data))
		if question == "" {
			flag.Usage()
			os.Exit(1)
		}
	}

	if *debug {
		debugf("=== elw %s ===", llm.Version)
		debugf("OLLAMA_HOST        = %q", os.Getenv("OLLAMA_HOST"))
		debugf("OPENROUTER_API_KEY = %q", mask(os.Getenv("OPENROUTER_API_KEY")))
		debugf("MODEL              = %q", os.Getenv("MODEL"))
		debugf("system             = %q", *system)
		debugf("question           = %q", question)
	}

	cfg, err := llm.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	cfg.Debug = *debug
	client, err := llm.NewClientWithConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *debug {
		debugf("provider = %s", client.Provider())
		debugf("model    = %s", client.Model())
	}

	start := time.Now()

	stream, err := client.Stream(context.Background(), llm.Request{
		System: *system,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart(question)}},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	if *debug {
		debugf("--- response (streaming) ---")
	}

	var chunks int
	for stream.Next() {
		fmt.Print(stream.Chunk())
		chunks++
	}
	fmt.Println()

	if err := stream.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *debug {
		debugf("--- done ---")
		debugf("chunks   = %d", chunks)
		debugf("elapsed  = %s", time.Since(start).Round(time.Millisecond))
	}
}

// debugf prints a debug line to stderr with a consistent prefix.
func debugf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", args...)
}

// mask redacts all but the first 4 characters of a secret for display.
func mask(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}
