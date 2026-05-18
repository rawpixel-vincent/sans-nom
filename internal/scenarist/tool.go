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
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

type ToolRunner struct {
	Runner           Runner
	OutputDir        string
	RepoRoot         string
	ContextText      string
	Tools            bool
	LoreExpert       *loreexpert.ToolRunner
	LoreExpertRounds int
	MaxParallel      int
	Logger           *gamelog.Writer
	CallerRole       string
	CallerName       string

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
		Description: fmt.Sprintf("Preferred creative workshop tool. Ask up to %d Scenarist agents first when exploring what the generated story could become: brainstorming, suggestions, critique, story seed ideas, doctrine stress tests, character contradictions, scene engines, choice pressure, or reader-plausibility checks. Requests inside one call run in parallel and do not see each other's answers; put only independent lenses in the same call. For dependent follow-up, read returned files, then call again with those paths. Scenarists may ask the cached Nerd Lore Expert for focused source-influence grounding. Outputs are free-form Markdown planning files and returned as paths; the Writer still authors the final StoryBible, StoryPacket, and prose-facing structure.", maxParallel),
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
	systemPrompt := ScenaristSystemPrompt(r.RepoRoot, r.Tools, r.LoreExpert != nil)
	dependencyWarnings := dependencyWarnings(payload.Requests)
	metadata := runnerModelMetadata(r.Runner)
	r.log(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.call.started",
		Message:  "writer requested scenarist material",
		Fields: modelmeta.MergeFields(map[string]any{
			"request_count":         len(payload.Requests),
			"max_parallel":          maxParallel,
			"call_dir":              filepath.ToSlash(callDir),
			"tools_enabled":         r.Tools,
			"lore_expert_available": r.LoreExpert != nil,
		}, metadata),
		Detail: map[string]any{
			"arguments":             arguments,
			"requests":              payload.Requests,
			"model_metadata":        metadata,
			"output_dir":            filepath.ToSlash(r.OutputDir),
			"call_dir":              filepath.ToSlash(callDir),
			"max_parallel":          maxParallel,
			"tools_enabled":         r.Tools,
			"lore_expert_available": r.LoreExpert != nil,
			"lore_expert_rounds":    r.effectiveLoreExpertRounds(),
			"system_prompt":         systemPrompt,
			"context_chars":         len(r.ContextText),
			"context_text":          r.ContextText,
			"planning_scope":        "scenarist material is planning-only; Writer owns final StoryPacket",
			"parallel_contract":     "requests in one call run in parallel and do not see each other's answers",
			"dependency_warnings":   dependencyWarnings,
		},
	})
	if len(dependencyWarnings) > 0 {
		r.log(gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "scenarist",
			Event:    "scenarist.call.dependency_warning",
			Message:  "parallel scenarist call contains dependent-looking requests",
			Fields: map[string]any{
				"request_count": len(payload.Requests),
				"warning_count": len(dependencyWarnings),
				"call_dir":      filepath.ToSlash(callDir),
			},
			Detail: map[string]any{
				"requests": payload.Requests,
				"warnings": dependencyWarnings,
				"guidance": "Requests in one call run in parallel and do not see each other's answers. For dependent work, read returned files and make another call with those files in relevant_paths.",
				"call_dir": filepath.ToSlash(callDir),
			},
		})
	}

	materials := make([]Material, len(payload.Requests))
	errs := make([]error, len(payload.Requests))
	var wg sync.WaitGroup
	for i, request := range payload.Requests {
		i, request := i, request
		wg.Add(1)
		go func() {
			defer wg.Done()
			query := ScenaristToolQuery(r.ContextText, request)
			raw, materialMetadata, err := r.runScenarist(ctx, systemPrompt, query, request)
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
			material := Material{
				TopicID: topicID,
				Topic:   request.Topic,
				Path:    filepath.ToSlash(path),
				Bytes:   int(info.Size()),
			}
			if !materialMetadata.Empty() {
				material.ModelMetadata = &materialMetadata
			}
			materials[i] = material
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
		Fields: modelmeta.MergeFields(map[string]any{
			"request_count":  len(payload.Requests),
			"material_count": len(materials),
			"call_dir":       filepath.ToSlash(callDir),
			"tools_enabled":  r.Tools,
		}, metadata),
		Detail: map[string]any{
			"requests":              payload.Requests,
			"manifest":              manifest,
			"model_metadata":        metadata,
			"manifest_path":         filepath.ToSlash(filepath.Join(callDir, "manifest.json")),
			"materials":             materials,
			"call_dir":              filepath.ToSlash(callDir),
			"tools_enabled":         r.Tools,
			"lore_expert_available": r.LoreExpert != nil,
			"parallel_contract":     "materials in one call were generated independently",
		},
	})
	return map[string]any{
		"ok":           true,
		"root":         filepath.ToSlash(callDir),
		"max_parallel": maxParallel,
		"materials":    materials,
		"note":         "Scenarist material is planning-only and each material in this call was generated independently. Read returned files selectively; for synthesis or dependent follow-up, call again with those paths. Do not copy wholesale or commit as canon.",
	}, nil
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
		query = query + "\n\nThe Scenarist asked the cached Nerd Lore Expert for source-influence grounding.\n\nPrevious Scenarist material:\n" + strings.TrimSpace(raw) + "\n\nLore Expert answers:\n" + formatLoreExpertAnswers(answers) + "\n\nContinue now as the Scenarist. Use these answers as influence-only support, then return free-form Markdown creative material for the Writer to mine.\n"
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
		Purpose:       "general_influence",
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

type dependencyWarning struct {
	Index        int      `json:"index"`
	TopicID      string   `json:"topic_id"`
	Topic        string   `json:"topic"`
	MatchedTerms []string `json:"matched_terms"`
}

func dependencyWarnings(requests []ToolRequest) []dependencyWarning {
	if len(requests) < 2 {
		return nil
	}
	var warnings []dependencyWarning
	for i, request := range requests {
		terms := dependencyTerms(request)
		if len(terms) == 0 {
			continue
		}
		warnings = append(warnings, dependencyWarning{
			Index:        i,
			TopicID:      request.TopicID,
			Topic:        request.Topic,
			MatchedTerms: terms,
		})
	}
	return warnings
}

func dependencyTerms(request ToolRequest) []string {
	text := strings.ToLower(strings.Join([]string{request.Topic, request.Request}, "\n"))
	candidates := []string{
		"previous",
		"prior",
		"strongest germ",
		"selected germ",
		"best candidate",
		"likely strongest",
		"choose the strongest",
		"for the likely",
		"after reading",
		"after reviewing",
		"based on the answer",
	}
	var terms []string
	for _, candidate := range candidates {
		if strings.Contains(text, candidate) {
			terms = append(terms, candidate)
		}
	}
	return terms
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
				"type":        "array",
				"description": "Parallel independent Scenarist requests. Requests in the same call run concurrently and cannot read each other's answers. If one question depends on another answer, call this tool again after reading returned material paths.",
				"minItems":    1,
				"maxItems":    maxParallel,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"topic_id", "topic", "request", "hard_boundaries", "relevant_paths"},
					"properties": map[string]any{
						"topic_id": map[string]any{"type": "string"},
						"topic":    map[string]any{"type": "string"},
						"request": map[string]any{
							"type":        "string",
							"description": "Creative or critical request for this one scenarist: brainstorm, suggest, critique, stress-test, react to a character/doctrine/scene, or propose vivid pressure material. The request can be broad, but it should be independently answerable unless this is the only request in the call.",
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
