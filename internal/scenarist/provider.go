package scenarist

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

type ProviderRunner struct {
	Provider string
	Model    string
	WorkDir  string
	EnvFiles []string
	Tools    bool
	AddDirs  []string
	Timeout  time.Duration
	Logger   *gamelog.Writer
}

type ProviderPoolRunner struct {
	Runners []ProviderRunner
	mu      sync.Mutex
	next    int
	once    sync.Once
	slots   []*providerSlot
}

func (r ProviderRunner) ModelMetadata() modelmeta.Metadata {
	provider := strings.ToUpper(strings.TrimSpace(r.Provider))
	if provider == "" {
		provider = "UNKNOWN"
	}
	return modelmeta.Metadata{
		ProviderType:  "claude_compatible",
		ModelProvider: provider,
		Model:         strings.TrimSpace(r.Model),
	}
}

func (r *ProviderPoolRunner) ModelMetadata() modelmeta.Metadata {
	if r == nil || len(r.Runners) == 0 {
		return modelmeta.Metadata{}
	}
	providers := make([]string, 0, len(r.Runners))
	models := make([]string, 0, len(r.Runners))
	for _, runner := range r.Runners {
		meta := runner.ModelMetadata()
		if strings.TrimSpace(meta.ModelProvider) != "" {
			providers = append(providers, meta.ModelProvider)
		}
		if strings.TrimSpace(meta.Model) != "" {
			models = append(models, meta.Model)
		}
	}
	return modelmeta.Metadata{
		ProviderType:  "claude_compatible_pool",
		ModelProvider: strings.Join(uniqueSorted(providers), ","),
		Model:         strings.Join(uniqueSorted(models), ","),
	}
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

type providerSlot struct {
	runner ProviderRunner
	sem    chan struct{}
}

func (r *ProviderPoolRunner) RunScenarist(ctx context.Context, systemPrompt string, query string) (string, error) {
	raw, _, err := r.RunScenaristWithMetadata(ctx, systemPrompt, query)
	return raw, err
}

func (r *ProviderPoolRunner) RunScenaristWithMetadata(ctx context.Context, systemPrompt string, query string) (string, modelmeta.Metadata, error) {
	if r == nil || len(r.Runners) == 0 {
		return "", modelmeta.Metadata{}, errors.New("scenarist provider pool is empty")
	}
	slot, release, err := r.acquireSlot(ctx)
	if err != nil {
		return "", modelmeta.Metadata{}, err
	}
	defer release()
	return slot.runner.RunScenaristWithMetadata(ctx, systemPrompt, query)
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
	raw, _, err := r.RunScenaristWithMetadata(ctx, systemPrompt, query)
	return raw, err
}

func (r ProviderRunner) RunScenaristWithMetadata(ctx context.Context, systemPrompt string, query string) (string, modelmeta.Metadata, error) {
	raw, model, err := r.runScenarist(ctx, systemPrompt, query)
	metadata := r.ModelMetadata()
	if model != "" {
		metadata.Model = model
	}
	return raw, metadata, err
}

func (r ProviderRunner) runScenarist(ctx context.Context, systemPrompt string, query string) (string, string, error) {
	if strings.TrimSpace(query) == "" {
		return "", "", errors.New("scenarist query is required")
	}
	env, model, err := ProviderEnv(r.Provider, r.Model, r.EnvFiles, os.Environ())
	if err != nil {
		return "", "", err
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if r.Tools {
		raw, err := r.runStreamScenarist(ctx, model, systemPrompt, query)
		return raw, model, err
	}

	args := []string{"--bare", "--model", model, "--system-prompt", systemPrompt, "-p", query}
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
			return "", model, fmt.Errorf("scenarist provider %s: %s: %w", strings.ToUpper(r.Provider), errMsg, err)
		}
		return "", model, fmt.Errorf("scenarist provider %s: %w", strings.ToUpper(r.Provider), err)
	}
	return strings.TrimSpace(stdout.String()), model, nil
}

func (r ProviderRunner) runStreamScenarist(ctx context.Context, model string, systemPrompt string, query string) (string, error) {
	env, _, err := ProviderEnv(r.Provider, model, r.EnvFiles, os.Environ())
	if err != nil {
		return "", err
	}
	r.Model = model
	args := []string{"--bare", "--model", model, "--system-prompt", systemPrompt, "--output-format", "stream-json", "--verbose", "-p", query, "--permission-mode", "dontAsk", "--tools", "Read,LS,Grep"}
	for _, dir := range r.AddDirs {
		if strings.TrimSpace(dir) != "" {
			args = append(args, "--add-dir", dir)
		}
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	if r.WorkDir != "" {
		cmd.Dir = r.WorkDir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stderrBuf, stderr)
	}()
	parser := newClaudeStreamParser(r)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		parser.consume(scanner.Text())
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	wg.Wait()
	if scanErr != nil {
		return "", fmt.Errorf("scenarist provider %s stream: %w", strings.ToUpper(r.Provider), scanErr)
	}
	if waitErr != nil {
		errMsg := strings.TrimSpace(stderrBuf.String())
		if errMsg != "" {
			return "", fmt.Errorf("scenarist provider %s: %s: %w", strings.ToUpper(r.Provider), errMsg, waitErr)
		}
		return "", fmt.Errorf("scenarist provider %s: %w", strings.ToUpper(r.Provider), waitErr)
	}
	return strings.TrimSpace(parser.result()), nil
}

type claudeStreamParser struct {
	runner     ProviderRunner
	toolNames  map[string]string
	toolInput  map[string]map[string]any
	resultText string
	textParts  []string
}

func newClaudeStreamParser(runner ProviderRunner) *claudeStreamParser {
	return &claudeStreamParser{
		runner:    runner,
		toolNames: map[string]string{},
		toolInput: map[string]map[string]any{},
	}
}

func (p *claudeStreamParser) consume(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return
	}
	switch stringAny(event["type"]) {
	case "result":
		p.resultText = stringAny(event["result"])
	case "assistant", "user":
		p.consumeMessage(event)
	}
}

func (p *claudeStreamParser) consumeMessage(event map[string]any) {
	message, _ := event["message"].(map[string]any)
	for _, raw := range contentBlocks(message["content"]) {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch stringAny(block["type"]) {
		case "text":
			if text := strings.TrimSpace(stringAny(block["text"])); text != "" {
				p.textParts = append(p.textParts, text)
			}
		case "tool_use":
			p.logToolUse(block, event)
		case "tool_result":
			p.logToolResult(block, event)
		}
	}
}

func (p *claudeStreamParser) logToolUse(block map[string]any, event map[string]any) {
	id := stringAny(block["id"])
	name := firstNonEmpty(stringAny(block["name"]), "tool")
	input, _ := block["input"].(map[string]any)
	if id != "" {
		p.toolNames[id] = name
		p.toolInput[id] = input
	}
	metadata := p.runner.ModelMetadata()
	p.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.provider.tool_use",
		Message:  providerToolMessage(name, input),
		Fields: modelmeta.MergeFields(map[string]any{
			"provider":       strings.ToUpper(p.runner.Provider),
			"model":          p.runner.Model,
			"tool_name":      name,
			"tool_use_id":    id,
			"target":         providerToolTarget(input),
			"caller_role":    "scenarist",
			"caller_speaker": "Scenarist",
		}, metadata),
		Detail: map[string]any{
			"provider":       strings.ToUpper(p.runner.Provider),
			"model":          p.runner.Model,
			"model_metadata": metadata,
			"tool_name":      name,
			"tool_use_id":    id,
			"input":          input,
			"raw_event":      event,
			"raw_block":      block,
		},
	})
}

func (p *claudeStreamParser) logToolResult(block map[string]any, event map[string]any) {
	id := stringAny(block["tool_use_id"])
	name := firstNonEmpty(p.toolNames[id], "tool")
	metadata := p.runner.ModelMetadata()
	p.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.provider.tool_result",
		Message:  providerToolResultMessage(name, p.toolInput[id], block),
		Fields: modelmeta.MergeFields(map[string]any{
			"provider":       strings.ToUpper(p.runner.Provider),
			"model":          p.runner.Model,
			"tool_name":      name,
			"tool_use_id":    id,
			"target":         providerToolTarget(p.toolInput[id]),
			"caller_role":    "scenarist",
			"caller_speaker": "Scenarist",
		}, metadata),
		Detail: map[string]any{
			"provider":       strings.ToUpper(p.runner.Provider),
			"model":          p.runner.Model,
			"model_metadata": metadata,
			"tool_name":      name,
			"tool_use_id":    id,
			"input":          p.toolInput[id],
			"result":         block,
			"raw_event":      event,
		},
	})
}

func (p *claudeStreamParser) log(event gamelog.Event) {
	if p == nil || p.runner.Logger == nil {
		return
	}
	_ = p.runner.Logger.Append(event)
}

func (p *claudeStreamParser) result() string {
	if strings.TrimSpace(p.resultText) != "" {
		return p.resultText
	}
	return strings.Join(p.textParts, "\n")
}

func contentBlocks(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case string:
		return []any{map[string]any{"type": "text", "text": typed}}
	default:
		return nil
	}
}

func providerToolMessage(name string, input map[string]any) string {
	target := providerToolTarget(input)
	if target == "" {
		return "@" + name
	}
	return "@" + name + " " + target
}

func providerToolResultMessage(name string, input map[string]any, block map[string]any) string {
	target := providerToolTarget(input)
	content := previewToolContent(block["content"], 700)
	if target != "" && content != "" {
		return name + " result for " + target + "\n\n" + content
	}
	if content != "" {
		return name + " result\n\n" + content
	}
	if target != "" {
		return name + " result for " + target
	}
	return name + " result"
}

func providerToolTarget(input map[string]any) string {
	for _, key := range []string{"file_path", "path", "glob", "pattern", "query"} {
		if value := strings.TrimSpace(stringAny(input[key])); value != "" {
			return value
		}
	}
	if len(input) == 0 {
		return ""
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprint(input)
	}
	return string(data)
}

func previewToolContent(value any, max int) string {
	text := strings.TrimSpace(toolContentText(value))
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func toolContentText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				parts = append(parts, firstNonEmpty(stringAny(mapped["text"]), stringAny(mapped["content"])))
			} else {
				parts = append(parts, stringAny(item))
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
