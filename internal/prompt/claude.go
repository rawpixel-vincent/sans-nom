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
	Model          string
	Effort         string
	WorkDir        string
	Timeout        time.Duration
	Bare           bool
	SystemPrompt   string
	PermissionMode string
	AllowedTools   []string
	AddDirs        []string
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
		timeout = 15 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"--model", model}
	if r.Bare {
		args = append([]string{"--bare"}, args...)
	}
	if strings.TrimSpace(r.SystemPrompt) != "" {
		args = append(args, "--system-prompt", r.SystemPrompt)
	}
	args = append(args,
		"-p", text,
		"--output-format", "text",
		"--effort", effort,
	)
	if strings.TrimSpace(r.PermissionMode) != "" {
		args = append(args, "--permission-mode", r.PermissionMode)
	}
	if len(r.AllowedTools) > 0 {
		args = append(args, "--tools", strings.Join(r.AllowedTools, ","))
	}
	for _, dir := range r.AddDirs {
		if strings.TrimSpace(dir) != "" {
			args = append(args, "--add-dir", dir)
		}
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
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
