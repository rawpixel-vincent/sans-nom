package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

type StaticRunner struct {
	Response string
	Err      error
}

func (r StaticRunner) Run(context.Context, string) (string, error) {
	return r.Response, r.Err
}

type ClaudeRunner struct {
	Model   string
	Effort  string
	WorkDir string
	Timeout time.Duration
}

func (r ClaudeRunner) Run(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("prompt text is required")
	}
	model := r.Model
	if model == "" {
		model = "sonnet"
	}
	effort := r.Effort
	if effort == "" {
		effort = "medium"
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", model,
		"-p", text,
		"--output-format", "text",
		"--effort", effort,
	)
	if r.WorkDir != "" {
		cmd.Dir = r.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("claude CLI: %s: %w", errMsg, err)
		}
		return "", fmt.Errorf("claude CLI: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}
