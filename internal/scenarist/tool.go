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
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
)

type ToolRunner struct {
	Runner      Runner
	OutputDir   string
	RepoRoot    string
	ContextText string
	Tools       bool
	MaxParallel int
	Logger      *gamelog.Writer
	CallerRole  string
	CallerName  string

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
		Description: fmt.Sprintf("Preferred creative workshop tool. Ask up to %d Scenarist agents first when exploring what the generated story could become: brainstorming, suggestions, critique, story seed ideas, doctrine stress tests, character contradictions, scene engines, choice pressure, or suspension-of-disbelief checks. Outputs are free-form Markdown planning files and returned as paths; the Writer still authors the final StoryBible, StoryPacket, and prose-facing structure.", maxParallel),
		Parameters:  scenaristToolSchema(maxParallel),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, _ string, arguments string) (any, error) {
	if r == nil || r.Runner == nil {
		return nil, errors.New("scenarist tool requires runner")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		err := errors.New("scenarist tool output dir is required")
		r.logRejected(arguments, err, nil)
		return nil, err
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
		r.logRejected(arguments, err, nil)
		return nil, err
	}
	if len(payload.Requests) == 0 {
		err := errors.New("call_scenarists requires at least one request")
		r.logRejected(arguments, err, payload.Requests)
		return nil, err
	}
	if len(payload.Requests) > maxParallel {
		err := fmt.Errorf("call_scenarists received %d requests; provider pool allows %d parallel calls", len(payload.Requests), maxParallel)
		r.logRejected(arguments, err, payload.Requests)
		return nil, err
	}
	for i, request := range payload.Requests {
		if err := request.Validate(); err != nil {
			err = fmt.Errorf("request %d: %w", i+1, err)
			r.logRejected(arguments, err, payload.Requests)
			return nil, err
		}
	}
	callDir := filepath.Join(r.OutputDir, r.nextCallID())
	if err := os.MkdirAll(callDir, 0o755); err != nil {
		return nil, err
	}
	systemPrompt := ScenaristSystemPrompt(r.RepoRoot, r.Tools)
	r.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.call.started",
		Message:  "writer requested scenarist material",
		Fields: map[string]any{
			"request_count": len(payload.Requests),
			"max_parallel":  maxParallel,
			"call_dir":      filepath.ToSlash(callDir),
		},
		Detail: map[string]any{
			"arguments":      arguments,
			"requests":       payload.Requests,
			"output_dir":     filepath.ToSlash(r.OutputDir),
			"call_dir":       filepath.ToSlash(callDir),
			"max_parallel":   maxParallel,
			"tools_enabled":  r.Tools,
			"system_prompt":  systemPrompt,
			"context_chars":  len(r.ContextText),
			"context_text":   r.ContextText,
			"planning_scope": "scenarist material is planning-only; Writer owns final StoryPacket",
		},
	})

	materials := make([]Material, len(payload.Requests))
	errs := make([]error, len(payload.Requests))
	var wg sync.WaitGroup
	for i, request := range payload.Requests {
		i, request := i, request
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := ScenaristToolQuery(r.ContextText, request)
			raw, err := r.Runner.RunScenarist(ctx, systemPrompt, query)
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
			r.log(gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "scenarist",
				Event:    "scenarist.call.failed",
				Message:  err.Error(),
				Fields:   map[string]any{"call_dir": filepath.ToSlash(callDir)},
				Detail: map[string]any{
					"requests": payload.Requests,
					"errors":   errorStrings(errs),
					"call_dir": filepath.ToSlash(callDir),
				},
			})
			return nil, err
		}
	}
	sort.SliceStable(materials, func(i, j int) bool {
		return materials[i].TopicID < materials[j].TopicID
	})
	manifest := Manifest{Root: filepath.ToSlash(callDir), Materials: materials}
	if err := writeJSONAtomic(filepath.Join(callDir, "manifest.json"), manifest); err != nil {
		r.log(gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "scenarist",
			Event:    "scenarist.call.failed",
			Message:  err.Error(),
			Fields:   map[string]any{"call_dir": filepath.ToSlash(callDir)},
			Detail: map[string]any{
				"requests":  payload.Requests,
				"materials": materials,
				"call_dir":  filepath.ToSlash(callDir),
			},
		})
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.call.completed",
		Message:  "scenarist material written",
		Fields: map[string]any{
			"request_count":  len(payload.Requests),
			"material_count": len(materials),
			"call_dir":       filepath.ToSlash(callDir),
		},
		Detail: map[string]any{
			"requests":      payload.Requests,
			"manifest":      manifest,
			"manifest_path": filepath.ToSlash(filepath.Join(callDir, "manifest.json")),
			"materials":     materials,
			"call_dir":      filepath.ToSlash(callDir),
		},
	})
	return map[string]any{
		"ok":           true,
		"root":         filepath.ToSlash(callDir),
		"max_parallel": maxParallel,
		"materials":    materials,
		"note":         "Scenarist material is planning-only. Read returned files selectively; do not copy wholesale or commit as canon.",
	}, nil
}

func (r *ToolRunner) log(event gamelog.Event) {
	if r == nil || r.Logger == nil {
		return
	}
	event = r.withCaller(event)
	_ = r.Logger.Append(event)
}

func (r *ToolRunner) withCaller(event gamelog.Event) gamelog.Event {
	role := strings.TrimSpace(r.CallerRole)
	if role == "" {
		return event
	}
	speaker := strings.TrimSpace(r.CallerName)
	if speaker == "" {
		speaker = role
	}
	if event.Fields == nil {
		event.Fields = map[string]any{}
	}
	if _, ok := event.Fields["caller_role"]; !ok {
		event.Fields["caller_role"] = role
	}
	if _, ok := event.Fields["caller_speaker"]; !ok {
		event.Fields["caller_speaker"] = speaker
	}
	if detail, ok := event.Detail.(map[string]any); ok {
		if _, exists := detail["caller_role"]; !exists {
			detail["caller_role"] = role
		}
		if _, exists := detail["caller_speaker"]; !exists {
			detail["caller_speaker"] = speaker
		}
	}
	return event
}

func (r *ToolRunner) logRejected(arguments string, err error, requests []ToolRequest) {
	r.log(gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "scenarist",
		Event:    "scenarist.call.rejected",
		Message:  err.Error(),
		Fields: map[string]any{
			"argument_chars": len(arguments),
		},
		Detail: map[string]any{
			"arguments": arguments,
			"requests":  requests,
			"error":     err.Error(),
		},
	})
}

func (r *ToolRunner) nextCallID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return fmt.Sprintf("tool-call-%03d", r.nextID)
}

func errorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			out = append(out, err.Error())
		}
	}
	return out
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
						"topic_id": map[string]any{"type": "string"},
						"topic":    map[string]any{"type": "string"},
						"request": map[string]any{
							"type":        "string",
							"description": "Creative or critical request for the scenarist: brainstorm, suggest, critique, stress-test, react to a character/doctrine/scene, or propose vivid pressure material. The request can be broad.",
						},
						"hard_boundaries": map[string]any{
							"type":        "array",
							"description": "Concise boundaries for this brainstorm, such as original material, planning-only, preserve the current story premise, or avoid direct canon equivalents.",
							"items":       map[string]any{"type": "string"},
						},
						"relevant_paths": map[string]any{
							"type":        "array",
							"description": "Optional planning files the scenarist may consider, such as workshop outputs or current packet artifacts.",
							"items":       map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}
