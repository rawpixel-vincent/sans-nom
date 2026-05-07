package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
)

func main() {
	var inputPath string
	var outputPath string
	var authPath string
	var modelOverride string
	var toolRoot string
	var maxToolRounds int
	var fullResponse bool
	var timeout time.Duration
	flag.StringVar(&inputPath, "input", "", "JSON request file. Use either a Responses body directly or {\"body\": ...}.")
	flag.StringVar(&outputPath, "output", "", "Optional output path. Defaults to stdout.")
	flag.StringVar(&authPath, "auth-path", "", "Codex auth path. Defaults to CODEX_HOME/auth.json or ~/.codex/auth.json.")
	flag.StringVar(&modelOverride, "model-override", "", "Force all requests to this Codex model.")
	flag.StringVar(&toolRoot, "tool-root", "", "Enable read-only local file tools rooted at this directory.")
	flag.IntVar(&maxToolRounds, "max-tool-rounds", 24, "Maximum local tool rounds when --tool-root is set.")
	flag.BoolVar(&fullResponse, "full-response", false, "Write the full aggregated Responses object instead of response.output_text.")
	flag.DurationVar(&timeout, "timeout", 30*time.Minute, "Request timeout.")
	flag.Parse()

	if inputPath == "" {
		fail("missing --input")
	}

	body, err := readBody(inputPath)
	if err != nil {
		fail(err.Error())
	}

	client, err := codexoauth.NewClient(codexoauth.Options{
		AuthPath:      authPath,
		ModelOverride: modelOverride,
	})
	if err != nil {
		fail(err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var response map[string]any
	if toolRoot != "" {
		response, err = client.CreateWithLocalTools(ctx, body, codexoauth.LocalToolOptions{
			Root:      toolRoot,
			MaxRounds: maxToolRounds,
		})
	} else {
		response, err = client.Create(ctx, body)
	}
	if err != nil {
		fail(err.Error())
	}

	var output []byte
	if fullResponse {
		output, err = json.MarshalIndent(response, "", "  ")
		if err == nil {
			output = append(output, '\n')
		}
	} else {
		text, _ := response["output_text"].(string)
		output = []byte(text)
	}
	if err != nil {
		fail(err.Error())
	}

	if outputPath == "" {
		_, _ = os.Stdout.Write(output)
		return
	}
	if err := os.WriteFile(outputPath, output, 0o644); err != nil {
		fail(err.Error())
	}
}

func readBody(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Body != nil {
		return envelope.Body, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "codex-responses-json:", message)
	os.Exit(1)
}
