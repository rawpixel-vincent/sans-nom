package scenarist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
)

type ToolRunner struct {
	Runner      Runner
	OutputDir   string
	RepoRoot    string
	ContextText string
	Tools       bool
	MaxParallel int

	mu     sync.Mutex
	nextID int
}

func (r *ToolRunner) LocalTool() codexoauth.LocalTool {
	maxParallel := r.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}
	return codexoauth.LocalTool{
		Name:        "call_scenarists",
		Description: fmt.Sprintf("Ask up to %d Scenarist agents for free-form creative help in parallel. Outputs are written to markdown files and returned as paths only.", maxParallel),
		Parameters:  scenaristToolSchema(maxParallel),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, _ string, arguments string) (any, error) {
	if r == nil || r.Runner == nil {
		return nil, errors.New("scenarist tool requires runner")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		return nil, errors.New("scenarist tool output dir is required")
	}
	maxParallel := r.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 1
	}
	var payload struct {
		Requests []ToolRequest `json:"requests"`
	}
	dec := json.NewDecoder(strings.NewReader(arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Requests) == 0 {
		return nil, errors.New("call_scenarists requires at least one request")
	}
	if len(payload.Requests) > maxParallel {
		return nil, fmt.Errorf("call_scenarists received %d requests; provider pool allows %d parallel calls", len(payload.Requests), maxParallel)
	}
	for i, request := range payload.Requests {
		if err := request.Validate(); err != nil {
			return nil, fmt.Errorf("request %d: %w", i+1, err)
		}
	}
	callDir := filepath.Join(r.OutputDir, r.nextCallID())
	if err := os.MkdirAll(callDir, 0o755); err != nil {
		return nil, err
	}

	materials := make([]Material, len(payload.Requests))
	errs := make([]error, len(payload.Requests))
	var wg sync.WaitGroup
	for i, request := range payload.Requests {
		i, request := i, request
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := r.Runner.RunScenarist(ctx, ScenaristSystemPrompt(r.RepoRoot, r.Tools), ScenaristToolQuery(r.ContextText, request))
			if err != nil {
				errs[i] = err
				return
			}
			raw = strings.TrimSpace(raw)
			if raw == "" {
				errs[i] = fmt.Errorf("scenarist %q returned empty material", request.TopicID)
				return
			}
			topicID := SanitizeID(request.TopicID)
			path := filepath.Join(callDir, fmt.Sprintf("%02d-%s.md", i+1, topicID))
			if err := writeTextAtomic(path, raw+"\n"); err != nil {
				errs[i] = err
				return
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				errs[i] = statErr
				return
			}
			materials[i] = Material{
				TopicID: topicID,
				Topic:   request.Topic,
				Path:    filepath.ToSlash(path),
				Bytes:   int(info.Size()),
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	sort.SliceStable(materials, func(i, j int) bool {
		return materials[i].TopicID < materials[j].TopicID
	})
	manifest := Manifest{Root: filepath.ToSlash(callDir), Materials: materials}
	if err := writeJSONAtomic(filepath.Join(callDir, "manifest.json"), manifest); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":           true,
		"root":         filepath.ToSlash(callDir),
		"max_parallel": maxParallel,
		"materials":    materials,
		"note":         "Scenarist material is planning-only. Read returned files selectively; do not copy wholesale or commit as canon.",
	}, nil
}

func (r *ToolRunner) nextCallID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return fmt.Sprintf("tool-call-%03d", r.nextID)
}

func scenaristToolSchema(maxParallel int) map[string]any {
	if maxParallel <= 0 {
		maxParallel = 1
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"requests"},
		"properties": map[string]any{
			"requests": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": maxParallel,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"topic_id", "topic", "request", "hard_boundaries", "relevant_paths"},
					"properties": map[string]any{
						"topic_id":        map[string]any{"type": "string"},
						"topic":           map[string]any{"type": "string"},
						"request":         map[string]any{"type": "string"},
						"hard_boundaries": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"relevant_paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
				},
			},
		},
	}
}
