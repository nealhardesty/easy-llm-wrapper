// Command elwi generates images from a text prompt using easy-llm-wrapper.
// Images are saved to disk; any accompanying text is printed to stdout.
//
// Usage:
//
//	elwi [flags] [prompt]
//
// Requires OPENROUTER_API_KEY. Defaults to google/gemini-2.0-flash-exp for
// image generation unless overridden by -model or the MODEL env var.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	llm "github.com/nealhardesty/easy-llm-wrapper"
)

const defaultImageModel = "google/gemini-3.1-flash-image-preview"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit (shorthand)")
	debug := flag.Bool("d", false, "enable debug output (model, timing, image info)")
	system := flag.String("s", "", "system prompt")
	output := flag.String("o", "output", "output file base name (extension appended from MIME type)")
	model := flag.String("model", "", fmt.Sprintf("override image generation model (default: %s)", defaultImageModel))

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: elwi [flags] [prompt]\n\n")
		fmt.Fprintf(os.Stderr, "Generate an image from a text prompt and save to disk.\n")
		fmt.Fprintf(os.Stderr, "If no prompt is given as an argument, it is read from stdin.\n")
		fmt.Fprintf(os.Stderr, "Any text in the response is printed to stdout.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  elwi A photorealistic cat wearing a top hat\n")
		fmt.Fprintf(os.Stderr, "  elwi -o cat A photorealistic cat wearing a top hat\n")
		fmt.Fprintf(os.Stderr, "  elwi -s 'oil painting style' A sunset over mountains\n")
		fmt.Fprintf(os.Stderr, "  echo 'A red balloon' | elwi\n\n")
		fmt.Fprintf(os.Stderr, "Environment variables:\n")
		fmt.Fprintf(os.Stderr, "  OPENROUTER_API_KEY OpenRouter API key (required).\n")
		fmt.Fprintf(os.Stderr, "  MODEL              Override the model.\n")
		fmt.Fprintf(os.Stderr, "                     Default: %s\n\n", defaultImageModel)
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("elwi %s\n", llm.Version)
		return
	}

	var prompt string
	if flag.NArg() > 0 {
		prompt = strings.Join(flag.Args(), " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		prompt = strings.TrimSpace(string(data))
		if prompt == "" {
			flag.Usage()
			os.Exit(1)
		}
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "error: OPENROUTER_API_KEY is required for image generation\n")
		os.Exit(1)
	}

	// Model priority: -model flag > MODEL env > built-in default.
	effectiveModel := defaultImageModel
	if m := os.Getenv("MODEL"); m != "" {
		effectiveModel = m
	}
	if *model != "" {
		effectiveModel = *model
	}

	if *debug {
		debugf("=== elwi %s ===", llm.Version)
		debugf("OPENROUTER_API_KEY = %q", mask(apiKey))
		debugf("model              = %s", effectiveModel)
		debugf("system             = %q", *system)
		debugf("prompt             = %q", prompt)
		debugf("output base        = %q", *output)
	} else {
		fmt.Fprintf(os.Stderr, "[openrouter / %s]\n", effectiveModel)
	}

	var transport http.RoundTripper
	if *debug {
		transport = &dumpTransport{w: os.Stderr}
	}

	client, err := llm.NewClientWithConfig(llm.Config{
		Provider:  llm.ProviderOpenRouter,
		Model:     effectiveModel,
		BaseURL:   "https://openrouter.ai/api/v1",
		APIKey:    apiKey,
		Transport: transport,
		Debug:     *debug,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	start := time.Now()

	resp, err := client.Complete(context.Background(), llm.Request{
		System: *system,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Parts: []llm.Part{llm.TextPart(prompt)}},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *debug {
		debugf("elapsed  = %s", time.Since(start).Round(time.Millisecond))
		debugf("images   = %d", len(resp.Images))
		debugf("text     = %q", resp.Text)
	}

	if len(resp.Images) == 0 {
		if resp.Text != "" {
			fmt.Println(resp.Text)
		}
		fmt.Fprintf(os.Stderr, "no images in response\n")
		os.Exit(1)
	}

	for i, img := range resp.Images {
		ext := extFromMIME(img.MIMEType)
		filename := *output
		if i > 0 {
			filename = fmt.Sprintf("%s_%d", *output, i+1)
		}
		filename += ext

		if err := os.WriteFile(filename, img.Data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", filename, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "saved: %s (%d bytes)\n", filename, len(img.Data))
	}

	if resp.Text != "" {
		fmt.Println(resp.Text)
	}
}

// extFromMIME returns the file extension for a given MIME type.
func extFromMIME(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

// dumpTransport is an http.RoundTripper that logs raw request and response to w.
type dumpTransport struct {
	w io.Writer
}

func (t *dumpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqDump, _ := httputil.DumpRequestOut(req, true)
	fmt.Fprintf(t.w, "[debug] >>> REQUEST\n%s\n", reqDump)

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	fmt.Fprintf(t.w, "[debug] <<< RESPONSE\n%s\n", respDump)
	return resp, nil
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
