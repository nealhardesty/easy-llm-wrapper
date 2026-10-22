// Command elwi generates images from a text prompt using easy-llm-wrapper.
// Images are saved to disk; any accompanying text is printed to stdout.
//
// Usage:
//
//	elwi [flags] [prompt]
//
// Requires OPENROUTER_API_KEY. Defaults to google/gemini-3.1-flash-image for
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

const defaultImageModel = "google/gemini-3.1-flash-image"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit (shorthand)")
	debug := flag.Bool("d", false, "enable debug output (model, timing, image info)")
	system := flag.String("s", "", "system prompt")
	output := flag.String("o", "output", "output file base name (extension appended from MIME type)")
	force := flag.Bool("f", false, "overwrite existing output files")
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

	// Fail before paying for a generation whose result we couldn't save.
	if !*force {
		for _, ext := range []string{".png", ".jpg", ".gif", ".webp", ".bin"} {
			if _, err := os.Stat(*output + ext); err == nil {
				fmt.Fprintf(os.Stderr, "error: %s exists (use -f to overwrite, or -o to pick another name)\n", *output+ext)
				os.Exit(1)
			}
		}
	}

	var transport http.RoundTripper
	if *debug {
		transport = &dumpTransport{w: os.Stderr}
	}

	client, err := llm.NewClientWithConfig(llm.Config{
		Provider:  llm.ProviderOpenRouter,
		Model:     effectiveModel,
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
		Modalities: []string{"image", "text"},
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

		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if !*force {
			flags |= os.O_EXCL
		}
		if err := writeFile(filename, img.Data, flags); err != nil {
			if os.IsExist(err) {
				err = fmt.Errorf("file exists (use -f to overwrite)")
			}
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

// writeFile writes data to name using the given open flags.
func writeFile(name string, data []byte, flags int) error {
	f, err := os.OpenFile(name, flags, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// maxDumpBytes caps each dumped request/response so base64 images don't flood the terminal.
const maxDumpBytes = 4096

// dumpTransport is an http.RoundTripper that logs raw request and response to w.
// The Authorization header is masked and dumps are truncated.
type dumpTransport struct {
	w io.Writer
}

func (t *dumpTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	redacted := req.Clone(req.Context())
	if auth := redacted.Header.Get("Authorization"); auth != "" {
		redacted.Header.Set("Authorization", "Bearer "+mask(strings.TrimPrefix(auth, "Bearer ")))
	}
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			redacted.Body = body
		}
	}
	reqDump, _ := httputil.DumpRequestOut(redacted, true)
	fmt.Fprintf(t.w, "[debug] >>> REQUEST\n%s\n", truncate(reqDump))

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)
	fmt.Fprintf(t.w, "[debug] <<< RESPONSE\n%s\n", truncate(respDump))
	return resp, nil
}

func truncate(b []byte) string {
	if len(b) <= maxDumpBytes {
		return string(b)
	}
	return fmt.Sprintf("%s\n... [%d more bytes truncated]", b[:maxDumpBytes], len(b)-maxDumpBytes)
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
