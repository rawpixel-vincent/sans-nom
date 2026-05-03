package council

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	claudeTimeout  = 3 * time.Minute
	claudeMaxChars = 24000
)

// claudeAvailable checks if the claude CLI is in PATH.
func claudeAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// runClaude executes `claude -p <prompt> --model <model> --output-format text`
// and returns the response text.
func runClaude(ctx context.Context, prompt string, model string, repoPath string, effort string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, claudeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude",
		"--model", model,
		"-p", prompt,
		"--output-format", "text",
		"--effort", effort,
	)
	cmd.Dir = repoPath

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

	out := strings.TrimSpace(stdout.String())
	if len(out) > claudeMaxChars {
		out = out[:claudeMaxChars] + "\n[truncated]"
	}

	return out, nil
}
