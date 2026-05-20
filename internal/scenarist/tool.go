package scenarist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

type ToolRunner struct {
	Runner           Runner
	OutputDir        string
	RepoRoot         string
	WorkspaceRoot    string
	VisiblePaths     []string
	ContextText      string
	Tools            bool
	LoreExpert       *loreexpert.ToolRunner
	LoreExpertRounds int
	MaxCalls         int
	Logger           *gamelog.Writer
	CallerRole       string
	CallerName       string

	mu     sync.Mutex
	nextID int
}

func (r *ToolRunner) LocalTool() codexoauth.LocalTool {
	return codexoauth.LocalTool{
		Name:        "ask_scenarist",
		Description: "Creative workshop tool for one Scenarist advisory session. Ask one clear staging, critique, risk check, reversal pass, character contradiction pass, scene-engine pass, or reader-plausibility question. The Scenarist writes one free-form Markdown planning file and returns its path. For follow-up, read the returned file first, then call ask_scenarist again with that path in relevant_paths. Scenarists may ask the cached Nerd Lore Expert for focused lore-reference grounding. The Writer still authors the final StoryBible, StoryPacket, and prose-facing structure.",
		Parameters:  scenaristToolSchema(),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, root string, arguments string) (any, error) {
	if r == nil || r.Runner == nil {
		return nil, errors.New("scenarist tool requires runner")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		err := errors.New("scenarist tool output dir is required")
		r.logRejected(arguments, err, nil)
		return nil, err
	}
	var request ToolRequest
	dec := json.NewDecoder(strings.NewReader(arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		r.logRejected(arguments, err, nil)
		return nil, err
	}
	if err := request.Validate(); err != nil {
		r.logRejected(arguments, err, []ToolRequest{request})
		return nil, err
	}
	workspaceRoot := firstNonEmpty(r.WorkspaceRoot, root, r.OutputDir)
	if err := r.normalizeRelevantPaths(workspaceRoot, &request); err != nil {
		r.logRejected(arguments, err, []ToolRequest{request})
		return nil, err
	}
	callID, callIndex, err := r.nextCallID()
	if err != nil {
		r.logRejected(arguments, err, []ToolRequest{request})
		return nil, err
	}
	callDir := filepath.Join(r.OutputDir, callID)
	if err := os.MkdirAll(callDir, 0o755); err != nil {
		return nil, err
	}
	systemPrompt := ScenaristSystemPrompt(workspaceRoot, r.Tools, r.LoreExpert != nil)
	metadata := runnerModelMetadata(r.Runner)
	r.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.ask.started",
		Message:  "writer asked scenarist for material",
		Fields: modelmeta.MergeFields(map[string]any{
			"call_dir":              filepath.ToSlash(callDir),
			"call_index":            callIndex,
			"topic_id":              request.TopicID,
			"topic":                 request.Topic,
			"tools_enabled":         r.Tools,
			"lore_expert_available": r.LoreExpert != nil,
			"agent_workspace_root":  filepath.ToSlash(workspaceRoot),
		}, metadata),
		Detail: map[string]any{
			"arguments":             arguments,
			"request":               request,
			"model_metadata":        metadata,
			"output_dir":            filepath.ToSlash(r.OutputDir),
			"call_dir":              filepath.ToSlash(callDir),
			"call_index":            callIndex,
			"tools_enabled":         r.Tools,
			"lore_expert_available": r.LoreExpert != nil,
			"lore_expert_rounds":    r.effectiveLoreExpertRounds(),
			"agent_workspace_root":  filepath.ToSlash(workspaceRoot),
			"visible_paths":         r.visiblePaths(workspaceRoot),
			"system_prompt":         systemPrompt,
			"context_chars":         len(r.ContextText),
			"context_text":          r.ContextText,
			"planning_scope":        "scenarist material is planning-only; Writer owns final StoryPacket",
		},
	})

	query := ScenaristToolQuery(r.ContextText, request)
	raw, materialMetadata, err := r.runScenarist(ctx, systemPrompt, query, request)
	if err != nil {
		r.log(gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "scenarist",
			Event:    "scenarist.ask.failed",
			Message:  err.Error(),
			Fields: map[string]any{
				"call_dir":   filepath.ToSlash(callDir),
				"call_index": callIndex,
				"topic_id":   request.TopicID,
			},
			Detail: map[string]any{
				"request":    request,
				"error":      err.Error(),
				"call_dir":   filepath.ToSlash(callDir),
				"call_index": callIndex,
			},
		})
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		err := fmt.Errorf("scenarist %q returned empty material", request.TopicID)
		r.log(gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "scenarist",
			Event:    "scenarist.ask.failed",
			Message:  err.Error(),
			Fields: map[string]any{
				"call_dir":   filepath.ToSlash(callDir),
				"call_index": callIndex,
				"topic_id":   request.TopicID,
			},
			Detail: map[string]any{
				"request":    request,
				"error":      err.Error(),
				"call_dir":   filepath.ToSlash(callDir),
				"call_index": callIndex,
			},
		})
		return nil, err
	}
	topicID := SanitizeID(request.TopicID)
	path := filepath.Join(callDir, fmt.Sprintf("01-%s.md", topicID))
	if err := writeTextAtomic(path, raw+"\n"); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	artifactMaterial := Material{
		TopicID:      topicID,
		Topic:        request.Topic,
		Path:         r.modelVisiblePath(workspaceRoot, path),
		ArtifactPath: filepath.ToSlash(path),
		Bytes:        int(info.Size()),
	}
	if !materialMetadata.Empty() {
		artifactMaterial.ModelMetadata = &materialMetadata
	}
	material := artifactMaterial
	material.ArtifactPath = ""
	completedMetadata := metadata
	if !materialMetadata.Empty() {
		completedMetadata = materialMetadata
	}
	materials := []Material{material}
	artifactMaterials := []Material{artifactMaterial}
	manifest := Manifest{Root: r.modelVisiblePath(workspaceRoot, callDir), Materials: materials}
	if err := writeJSONAtomic(filepath.Join(callDir, "manifest.json"), manifest); err != nil {
		r.log(gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "scenarist",
			Event:    "scenarist.ask.failed",
			Message:  err.Error(),
			Fields:   map[string]any{"call_dir": filepath.ToSlash(callDir), "call_index": callIndex},
			Detail: map[string]any{
				"request":   request,
				"materials": artifactMaterials,
				"call_dir":  filepath.ToSlash(callDir),
			},
		})
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.ask.completed",
		Message:  "scenarist material written",
		Fields: modelmeta.MergeFields(map[string]any{
			"material_count":       len(materials),
			"call_dir":             filepath.ToSlash(callDir),
			"call_index":           callIndex,
			"topic_id":             request.TopicID,
			"topic":                request.Topic,
			"tools_enabled":        r.Tools,
			"agent_workspace_root": filepath.ToSlash(workspaceRoot),
		}, completedMetadata),
		Detail: map[string]any{
			"request":                request,
			"manifest":               manifest,
			"model_metadata":         completedMetadata,
			"manifest_path":          filepath.ToSlash(filepath.Join(callDir, "manifest.json")),
			"materials":              artifactMaterials,
			"model_visible_manifest": manifest,
			"call_dir":               filepath.ToSlash(callDir),
			"call_index":             callIndex,
			"tools_enabled":          r.Tools,
			"lore_expert_available":  r.LoreExpert != nil,
			"agent_workspace_root":   filepath.ToSlash(workspaceRoot),
			"visible_paths":          r.visiblePaths(workspaceRoot),
		},
	})
	return map[string]any{
		"ok":        true,
		"root":      manifest.Root,
		"material":  material,
		"materials": materials,
		"note":      "Scenarist material is planning-only. Read the returned file before asking any dependent follow-up. Do not copy wholesale or commit as canon.",
	}, nil
}

func (r *ToolRunner) normalizeRelevantPaths(workspaceRoot string, request *ToolRequest) error {
	if request == nil || len(request.RelevantPaths) == 0 {
		return nil
	}
	scope := agentfs.New(workspaceRoot, "scenarist")
	normalized := make([]string, 0, len(request.RelevantPaths))
	for _, path := range request.RelevantPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		rel, err := scope.Normalize(path)
		if err != nil {
			return fmt.Errorf("relevant_path %q rejected: %w", path, err)
		}
		normalized = append(normalized, rel)
	}
	request.RelevantPaths = normalized
	return nil
}

func (r *ToolRunner) modelVisiblePath(workspaceRoot string, path string) string {
	scope := agentfs.New(workspaceRoot, "scenarist")
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	if rel, err := scope.Normalize(path); err == nil {
		return rel
	}
	return filepath.ToSlash(path)
}

func (r *ToolRunner) visiblePaths(workspaceRoot string) []string {
	if len(r.VisiblePaths) > 0 {
		return append([]string(nil), r.VisiblePaths...)
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil
	}
	return []string{"."}
}

func runnerModelMetadata(runner Runner) modelmeta.Metadata {
	if runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

type loreExpertAnswer struct {
	Question   string
	Preview    string
	AnswerPath string
	Cached     bool
	SourceRefs any
}

func (r *ToolRunner) runScenarist(ctx context.Context, systemPrompt string, query string, request ToolRequest) (string, modelmeta.Metadata, error) {
	raw, metadata, err := runScenaristWithMetadata(ctx, r.Runner, systemPrompt, query)
	if err != nil || r.LoreExpert == nil {
		return raw, metadata, err
	}
	seen := map[string]bool{}
	for round := 0; round < r.effectiveLoreExpertRounds(); round++ {
		questions := extractLoreExpertQuestions(raw)
		if len(questions) == 0 {
			return raw, metadata, nil
		}
		answers := make([]loreExpertAnswer, 0, len(questions))
		for _, question := range questions {
			key := strings.ToLower(strings.TrimSpace(question))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			answer, err := r.askLoreExpert(ctx, question, request)
			if err != nil {
				return "", metadata, err
			}
			answers = append(answers, answer)
		}
		if len(answers) == 0 {
			return raw, metadata, nil
		}
		query = query + "\n\nThe Scenarist asked the cached Nerd Lore Expert for lore-reference grounding.\n\nPrevious Scenarist material:\n" + strings.TrimSpace(raw) + "\n\nLore Expert answers:\n" + formatLoreExpertAnswers(answers) + "\n\nContinue now as the Scenarist. Use these answers as scenario-fidelity support, then return free-form Markdown creative material for the Writer to mine.\n"
		raw, metadata, err = runScenaristWithMetadata(ctx, r.Runner, systemPrompt, query)
		if err != nil {
			return "", metadata, err
		}
	}
	return raw, metadata, nil
}

type metadataRunner interface {
	RunScenaristWithMetadata(ctx context.Context, systemPrompt string, query string) (string, modelmeta.Metadata, error)
}

func runScenaristWithMetadata(ctx context.Context, runner Runner, systemPrompt string, query string) (string, modelmeta.Metadata, error) {
	if runner == nil {
		return "", modelmeta.Metadata{}, errors.New("scenarist runner is required")
	}
	if typed, ok := runner.(metadataRunner); ok {
		return typed.RunScenaristWithMetadata(ctx, systemPrompt, query)
	}
	raw, err := runner.RunScenarist(ctx, systemPrompt, query)
	return raw, runnerModelMetadata(runner), err
}

func (r *ToolRunner) askLoreExpert(ctx context.Context, question string, request ToolRequest) (loreExpertAnswer, error) {
	if r.LoreExpert == nil {
		return loreExpertAnswer{}, errors.New("lore expert tool is not available to scenarist")
	}
	tool := *r.LoreExpert
	tool.CallerRole = "scenarist"
	tool.CallerSpeaker = "Scenarist"
	payload := loreexpert.ToolRequest{
		Question:      question,
		Purpose:       "lore_reference",
		RelevantPaths: request.RelevantPaths,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return loreExpertAnswer{}, err
	}
	result, err := tool.Handle(ctx, r.RepoRoot, string(data))
	if err != nil {
		return loreExpertAnswer{}, err
	}
	mapped, ok := result.(map[string]any)
	if !ok {
		return loreExpertAnswer{}, fmt.Errorf("unexpected lore expert result %T", result)
	}
	answer := loreExpertAnswer{
		Question:   question,
		Preview:    stringField(mapped, "preview"),
		AnswerPath: stringField(mapped, "answer_path"),
		Cached:     boolField(mapped, "cached"),
		SourceRefs: mapped["source_refs"],
	}
	return answer, nil
}

func (r *ToolRunner) effectiveLoreExpertRounds() int {
	if r == nil || r.LoreExpert == nil {
		return 0
	}
	if r.LoreExpertRounds > 0 {
		if r.LoreExpertRounds > 4 {
			return 4
		}
		return r.LoreExpertRounds
	}
	return 2
}

func extractLoreExpertQuestions(text string) []string {
	var questions []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = trimListMarker(strings.TrimSpace(line))
		lower := strings.ToLower(line)
		for _, marker := range []string{"@loreexpert:", "@lore expert:", "@ask_lore_expert:"} {
			if !strings.HasPrefix(lower, marker) {
				continue
			}
			question := strings.TrimSpace(line[len(marker):])
			key := strings.ToLower(question)
			if question != "" && !seen[key] {
				seen[key] = true
				questions = append(questions, question)
			}
			break
		}
		if len(questions) >= 3 {
			break
		}
	}
	return questions
}

func trimListMarker(value string) string {
	for {
		switch {
		case strings.HasPrefix(value, "- "):
			value = strings.TrimSpace(strings.TrimPrefix(value, "- "))
		case strings.HasPrefix(value, "* "):
			value = strings.TrimSpace(strings.TrimPrefix(value, "* "))
		case strings.HasPrefix(value, "> "):
			value = strings.TrimSpace(strings.TrimPrefix(value, "> "))
		default:
			return value
		}
	}
}

func formatLoreExpertAnswers(answers []loreExpertAnswer) string {
	var b strings.Builder
	for i, answer := range answers {
		fmt.Fprintf(&b, "\n### LoreExpert answer %d\n", i+1)
		fmt.Fprintf(&b, "Question: %s\n", strings.TrimSpace(answer.Question))
		if answer.AnswerPath != "" {
			fmt.Fprintf(&b, "Answer path: %s\n", answer.AnswerPath)
		}
		fmt.Fprintf(&b, "Cached: %t\n", answer.Cached)
		if answer.SourceRefs != nil {
			fmt.Fprintf(&b, "Source refs: %v\n", answer.SourceRefs)
		}
		if answer.Preview != "" {
			fmt.Fprintf(&b, "Preview:\n%s\n", strings.TrimSpace(answer.Preview))
		}
	}
	return strings.TrimSpace(b.String())
}

func stringField(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func boolField(values map[string]any, key string) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return false
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
		Event:    "scenarist.ask.rejected",
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

func (r *ToolRunner) nextCallID() (string, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.MaxCalls > 0 && r.nextID >= r.MaxCalls {
		return "", r.nextID + 1, fmt.Errorf("ask_scenarist exceeded max calls %d", r.MaxCalls)
	}
	r.nextID++
	return fmt.Sprintf("tool-call-%03d", r.nextID), r.nextID, nil
}

func scenaristToolSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"topic_id", "topic", "request", "hard_boundaries", "relevant_paths"},
		"properties": map[string]any{
			"topic_id": map[string]any{
				"type":        "string",
				"description": "Stable short id for this one advisory request, used in the artifact filename.",
			},
			"topic": map[string]any{
				"type":        "string",
				"description": "Human-readable title for this advisory request.",
			},
			"request": map[string]any{
				"type":        "string",
				"description": "One focused creative or critical request: brainstorm, suggest, critique, stress-test, react to character/conflict/scene material, or propose vivid pressure material. If this depends on an earlier answer, include that file in relevant_paths.",
			},
			"hard_boundaries": map[string]any{
				"type":        "array",
				"description": "Concise boundaries for this brainstorm, such as original material, planning-only, preserve current story premise, or avoid direct canon equivalents.",
				"items":       map[string]any{"type": "string"},
			},
			"relevant_paths": map[string]any{
				"type":        "array",
				"description": "Optional workshop or story artifact paths the Scenarist may read. Use source-lore research through LoreExpert instead of passing lore-pack files here.",
				"items":       map[string]any{"type": "string"},
			},
		},
	}
}
