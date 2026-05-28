package bridgeproc

import (
	"context"
	"fmt"
	"net/http"
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

const defaultQdrantURL = "http://127.0.0.1:6333"

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
	if err := EnsureQdrant(ctx); err != nil {
		return nil, err
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

func EnsureQdrant(ctx context.Context) error {
	rawURL := qdrantURL()
	if err := qdrantHealth(ctx, rawURL, 750*time.Millisecond); err == nil {
		return nil
	}
	host, err := qdrantHost(rawURL)
	if err != nil {
		return err
	}
	if !localBridgeHost(host) {
		return fmt.Errorf("qdrant %s is not reachable and cannot be auto-started for non-local host %s", rawURL, host)
	}
	dockerPath, err := dockerPath()
	if err != nil {
		return fmt.Errorf("qdrant is not reachable at %s and docker CLI was not found: %w", rawURL, err)
	}
	root := repoRoot()
	cmd := exec.CommandContext(ctx, dockerPath, "compose", "up", "-d", "qdrant")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("qdrant is not reachable at %s and docker compose failed: %v\n%s", rawURL, err, commandOutput(output))
	}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := qdrantHealth(ctx, rawURL, 750*time.Millisecond); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("qdrant failed to become reachable at %s: %v", rawURL, lastErr)
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

func qdrantURL() string {
	if raw := strings.TrimSpace(os.Getenv("QDRANT_URL")); raw != "" {
		return raw
	}
	return defaultQdrantURL
}

func qdrantHealth(ctx context.Context, rawURL string, timeout time.Duration) error {
	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return fmt.Errorf("qdrant %s returned %s", rawURL, resp.Status)
	}
	return nil
}

func qdrantHost(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("qdrant URL %q has no host", rawURL)
	}
	return host, nil
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

func dockerPath() (string, error) {
	if path, err := exec.LookPath("docker"); err == nil {
		return path, nil
	}
	return dockerPathFromCandidates([]string{
		"/usr/local/bin/docker",
		"/opt/homebrew/bin/docker",
		"/Applications/Docker.app/Contents/Resources/bin/docker",
	})
}

func dockerPathFromCandidates(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("docker not found on PATH or common macOS locations")
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for dir := wd; dir != ""; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "docker-compose.yml")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return wd
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func commandOutput(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > 4000 {
		text = text[len(text)-4000:]
	}
	return text
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
