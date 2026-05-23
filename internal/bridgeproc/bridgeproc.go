package bridgeproc

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
)

type Config struct {
	Backend   string
	BridgeURL string
	Needed    bool
}

func EnsureFlagBridge(ctx context.Context, cfg Config) (func(), error) {
	if !cfg.Needed {
		return func() {}, nil
	}
	return Ensure(ctx, cfg.Backend, cfg.BridgeURL)
}

func Ensure(ctx context.Context, backend string, bridgeURL string) (func(), error) {
	if backend == "static" {
		return func() {}, nil
	}
	client := bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN"))
	if err := Health(ctx, client, 750*time.Millisecond); err == nil {
		return func() {}, nil
	}
	host, port, err := ListenAddress(client.BaseURL)
	if err != nil {
		return nil, err
	}
	cmd := bridgeCommand(ctx, host, port)
	cmd.Env = append(os.Environ(),
		"HOST="+host,
		"PORT="+port,
	)
	logPath := filepath.Join("tmp", "bridge.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	stop := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
	}
	deadline := time.Now().Add(12 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := Health(ctx, client, 750*time.Millisecond); err == nil {
			return stop, nil
		} else {
			lastErr = err
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	stop()
	return nil, fmt.Errorf("bridge failed to start at %s: %v\n%s", client.BaseURL, lastErr, LogTail(logPath))
}

func bridgeCommand(ctx context.Context, host string, port string) *exec.Cmd {
	return exec.CommandContext(ctx,
		filepath.Join("bridge", ".venv", "bin", "python"),
		"-m", "uvicorn", "bridge.main:app",
		"--host", host,
		"--port", port,
	)
}

func Health(ctx context.Context, client bridgeclient.Client, timeout time.Duration) error {
	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Health(healthCtx)
}

func ListenAddress(rawURL string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", "", fmt.Errorf("bridge URL %q has no port", rawURL)
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if !localBridgeHost(host) {
		return "", "", fmt.Errorf("bridge %s is not reachable and cannot be auto-started for non-local host %s", rawURL, host)
	}
	return host, port, nil
}

func localBridgeHost(host string) bool {
	host = strings.Trim(host, "[]")
	return host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func DialFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "cannot assign requested address") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "dial tcp")
}

func LogTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return strings.Join(lines, "\n")
}
