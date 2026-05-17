package scenarist

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProviderRunner struct {
	Provider string
	Model    string
	WorkDir  string
	EnvFiles []string
	Tools    bool
	AddDirs  []string
	Timeout  time.Duration
}

type ProviderPoolRunner struct {
	Runners []ProviderRunner
	mu      sync.Mutex
	next    int
	once    sync.Once
	slots   []*providerSlot
}

type providerSlot struct {
	runner ProviderRunner
	sem    chan struct{}
}

func (r *ProviderPoolRunner) RunScenarist(ctx context.Context, systemPrompt string, query string) (string, error) {
	if r == nil || len(r.Runners) == 0 {
		return "", errors.New("scenarist provider pool is empty")
	}
	slot, release, err := r.acquireSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return slot.runner.RunScenarist(ctx, systemPrompt, query)
}

func (r *ProviderPoolRunner) ProviderCount() int {
	if r == nil {
		return 0
	}
	return len(r.Runners)
}

func (r *ProviderPoolRunner) acquireSlot(ctx context.Context) (*providerSlot, func(), error) {
	r.initSlots()
	if len(r.slots) == 0 {
		return nil, nil, errors.New("scenarist provider pool is empty")
	}
	for range r.slots {
		slot := r.nextSlot()
		select {
		case slot.sem <- struct{}{}:
			return slot, func() { <-slot.sem }, nil
		default:
		}
	}
	slot := r.nextSlot()
	select {
	case slot.sem <- struct{}{}:
		return slot, func() { <-slot.sem }, nil
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (r *ProviderPoolRunner) initSlots() {
	r.once.Do(func() {
		r.slots = make([]*providerSlot, 0, len(r.Runners))
		for _, runner := range r.Runners {
			r.slots = append(r.slots, &providerSlot{
				runner: runner,
				sem:    make(chan struct{}, 1),
			})
		}
	})
}

func (r *ProviderPoolRunner) nextSlot() *providerSlot {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.next % len(r.slots)
	r.next++
	return r.slots[index]
}

func (r ProviderRunner) RunScenarist(ctx context.Context, systemPrompt string, query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", errors.New("scenarist query is required")
	}
	env, model, err := ProviderEnv(r.Provider, r.Model, r.EnvFiles, os.Environ())
	if err != nil {
		return "", err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"--bare", "--model", model, "--system-prompt", systemPrompt, "-p", query}
	if r.Tools {
		args = append(args, "--permission-mode", "dontAsk", "--tools", "Read,LS,Grep")
		for _, dir := range r.AddDirs {
			if strings.TrimSpace(dir) != "" {
				args = append(args, "--add-dir", dir)
			}
		}
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	if r.WorkDir != "" {
		cmd.Dir = r.WorkDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("scenarist provider %s: %s: %w", strings.ToUpper(r.Provider), errMsg, err)
		}
		return "", fmt.Errorf("scenarist provider %s: %w", strings.ToUpper(r.Provider), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func ProviderEnv(provider string, modelOverride string, files []string, base []string) ([]string, string, error) {
	provider = strings.ToUpper(strings.TrimSpace(provider))
	if provider == "" {
		provider = strings.ToUpper(strings.TrimSpace(os.Getenv("SANS_NOM_SCENARIST_PROVIDER")))
	}
	if provider == "" {
		return nil, "", errors.New("scenarist provider is required; set -scenarist-provider or SANS_NOM_SCENARIST_PROVIDER")
	}
	env := envSliceToMap(base)
	for _, path := range files {
		values, err := readEnvFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, "", err
		}
		for key, value := range values {
			env[key] = value
		}
	}
	prefix := provider + "__"
	found := false
	for key, value := range copyMap(env) {
		if strings.HasPrefix(key, prefix) {
			found = true
			env[strings.TrimPrefix(key, prefix)] = value
		}
	}
	if !found {
		return nil, "", fmt.Errorf("no %s__ provider environment found", provider)
	}
	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = strings.TrimSpace(env["ANTHROPIC_MODEL"])
	}
	if missingProviderValue(env["ANTHROPIC_BASE_URL"]) {
		return nil, "", fmt.Errorf("%s__ANTHROPIC_BASE_URL is required", provider)
	}
	if missingProviderValue(env["ANTHROPIC_AUTH_TOKEN"]) {
		return nil, "", fmt.Errorf("%s__ANTHROPIC_AUTH_TOKEN is required", provider)
	}
	if missingProviderValue(model) {
		return nil, "", fmt.Errorf("%s__ANTHROPIC_MODEL or -scenarist-model is required", provider)
	}
	env["ANTHROPIC_MODEL"] = model
	return mapToEnvSlice(env), model, nil
}

func ProviderNames(value string) []string {
	if strings.TrimSpace(value) == "" {
		value = os.Getenv("SANS_NOM_SCENARIST_PROVIDERS")
	}
	if strings.TrimSpace(value) == "" {
		value = os.Getenv("SANS_NOM_SCENARIST_PROVIDER")
	}
	if strings.TrimSpace(value) == "" {
		value = strings.Join(DefaultProviderNames(), ",")
	}
	value = strings.NewReplacer(",", " ", ";", " ").Replace(value)
	fields := strings.Fields(value)
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		name := strings.ToUpper(strings.TrimSpace(field))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func DefaultProviderNames() []string {
	return []string{"ZAI", "MINIMAX", "XIAOMI"}
}

func UsableProviders(names []string, modelOverride string, files []string, base []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, _, err := ProviderEnv(name, modelOverride, files, base); err == nil {
			out = append(out, strings.ToUpper(strings.TrimSpace(name)))
		}
	}
	return out
}

func DefaultEnvFiles(root string) []string {
	if root == "" {
		root = "."
	}
	return []string{
		filepath.Join(root, ".env.example"),
		filepath.Join(root, ".env"),
	}
}

func readEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" {
			values[key] = value
		}
	}
	return values, scanner.Err()
}

func envSliceToMap(values []string) map[string]string {
	out := map[string]string{}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func mapToEnvSlice(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func copyMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func missingProviderValue(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "***"
}
