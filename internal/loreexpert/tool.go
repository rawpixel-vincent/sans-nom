package loreexpert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

const PromptVersion = "lore-expert-v4"

type Runner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

type ToolRunner struct {
	Runner            Runner
	Retriever         lore.Retriever
	CacheRoot         string
	RepoRoot          string
	PackID            string
	PackLabel         string
	PackDescription   string
	Collection        string
	PackManifestPath  string
	IndexManifestPath string
	MasterLorePath    string
	MaxSources        int
	Logger            *gamelog.Writer
	CallerRole        string
	CallerSpeaker     string
}

type ToolRequest struct {
	PackID        string   `json:"pack_id,omitempty"`
	Question      string   `json:"question"`
	Purpose       string   `json:"purpose,omitempty"`
	RelevantPaths []string `json:"relevant_paths,omitempty"`
	AvoidTerms    []string `json:"avoid_terms,omitempty"`
}

type SourceRef struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind,omitempty"`
	Source string  `json:"source,omitempty"`
	Score  float64 `json:"score,omitempty"`
}

func (r *ToolRunner) LocalTool() codexoauth.LocalTool {
	return codexoauth.LocalTool{
		Name:        "ask_lore_expert",
		Description: "Lore reference support for the selected lorepack. Use when a question needs source references, tone, relationship pressure, contradiction checks, or scenario-fidelity support. Results are cached per lorepack and returned as a workspace-readable answer_path plus a short preview.",
		Parameters:  toolSchema(),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, workspaceRoot string, arguments string) (any, error) {
	if r == nil {
		return nil, errors.New("lore expert tool requires runner config")
	}
	var request ToolRequest
	dec := json.NewDecoder(strings.NewReader(arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		r.logFailed(arguments, nil, "", err)
		return nil, err
	}
	request.PackID = firstNonEmpty(request.PackID, r.PackID, "metabarons")
	request.Purpose = firstNonEmpty(request.Purpose, "general_lore_reference")
	if err := r.validateRequest(request); err != nil {
		r.logFailed(arguments, &request, "", err)
		return nil, err
	}
	cacheKey, err := r.cacheKey(request)
	if err != nil {
		r.logFailed(arguments, &request, "", err)
		return nil, err
	}
	cacheDir := filepath.Join(r.cacheRoot(), request.PackID, cacheKey)
	answerPath := filepath.Join(cacheDir, "answer.md")
	metadata := r.modelMetadata()
	if answer, err := os.ReadFile(answerPath); err == nil && strings.TrimSpace(string(answer)) != "" {
		refs := readSourceRefs(filepath.Join(cacheDir, "sources.json"))
		modelVisibleAnswerPath, err := r.materializeAnswer(workspaceRoot, request.PackID, cacheKey, cacheDir, string(answer), refs, true)
		if err != nil {
			r.logFailed(arguments, &request, cacheKey, err)
			return nil, err
		}
		r.log(gamelog.Event{
			Category: "lore",
			Event:    "lore.expert.cache_hit",
			Message:  "lore expert cache hit",
			Fields: modelmeta.MergeFields(map[string]any{
				"pack_id":                   request.PackID,
				"purpose":                   request.Purpose,
				"cache_key":                 cacheKey,
				"answer_path":               filepath.ToSlash(answerPath),
				"model_visible_answer_path": modelVisibleAnswerPath,
			}, metadata),
			Detail: map[string]any{
				"request":                   request,
				"cache_key":                 cacheKey,
				"cache_dir":                 filepath.ToSlash(cacheDir),
				"answer_path":               filepath.ToSlash(answerPath),
				"model_visible_answer_path": modelVisibleAnswerPath,
				"source_refs":               refs,
				"model_metadata":            metadata,
			},
		})
		return r.result(true, cacheDir, cacheKey, modelVisibleAnswerPath, string(answer), refs), nil
	}
	if r.Runner == nil {
		err := errors.New("lore expert runner is required on cache miss")
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	snippets, err := r.searchSources(ctx, request)
	if err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	refs := sourceRefs(snippets)
	promptText := r.promptText(request, snippets)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	if err := writeJSONAtomic(filepath.Join(cacheDir, "request.json"), request); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	if err := writeJSONAtomic(filepath.Join(cacheDir, "sources.json"), refs); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	if err := writeTextAtomic(filepath.Join(cacheDir, "prompt.txt"), promptText); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "lore",
		Event:    "lore.expert.request",
		Message:  "lore expert request prepared",
		Fields: modelmeta.MergeFields(map[string]any{
			"pack_id":      request.PackID,
			"purpose":      request.Purpose,
			"cache_key":    cacheKey,
			"source_count": len(refs),
			"prompt_chars": len(promptText),
			"prompt_path":  filepath.ToSlash(filepath.Join(cacheDir, "prompt.txt")),
		}, metadata),
		Detail: map[string]any{
			"request":        request,
			"cache_key":      cacheKey,
			"cache_dir":      filepath.ToSlash(cacheDir),
			"prompt":         promptText,
			"source_refs":    refs,
			"model_metadata": metadata,
		},
	})
	answer, err := r.Runner.Run(ctx, promptText)
	if err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		_ = writeTextAtomic(filepath.Join(cacheDir, "error.txt"), err.Error()+"\n")
		return nil, err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		err := errors.New("lore expert returned empty answer")
		r.logFailed(arguments, &request, cacheKey, err)
		_ = writeTextAtomic(filepath.Join(cacheDir, "error.txt"), err.Error()+"\n")
		return nil, err
	}
	if err := writeTextAtomic(answerPath, answer+"\n"); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	meta := map[string]any{
		"prompt_version": PromptVersion,
		"pack_id":        request.PackID,
		"pack_label":     r.packLabel(),
		"collection":     r.Collection,
		"cache_key":      cacheKey,
		"answer_path":    filepath.ToSlash(answerPath),
		"answer_chars":   len(answer),
		"source_count":   len(refs),
		"model":          metadata,
		"created_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"planning_only":  true,
	}
	if err := writeJSONAtomic(filepath.Join(cacheDir, "metadata.json"), meta); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	modelVisibleAnswerPath, err := r.materializeAnswer(workspaceRoot, request.PackID, cacheKey, cacheDir, answer, refs, false)
	if err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "lore",
		Event:    "lore.expert.response",
		Message:  "lore expert answer written",
		Fields: modelmeta.MergeFields(map[string]any{
			"pack_id":                   request.PackID,
			"purpose":                   request.Purpose,
			"cache_key":                 cacheKey,
			"answer_chars":              len(answer),
			"answer_path":               filepath.ToSlash(answerPath),
			"model_visible_answer_path": modelVisibleAnswerPath,
		}, metadata),
		Detail: map[string]any{
			"request":                   request,
			"metadata":                  meta,
			"model_visible_answer_path": modelVisibleAnswerPath,
			"source_refs":               refs,
			"answer":                    answer,
			"model_metadata":            metadata,
		},
	})
	return r.result(false, cacheDir, cacheKey, modelVisibleAnswerPath, answer, refs), nil
}

func (r *ToolRunner) validateRequest(request ToolRequest) error {
	if strings.TrimSpace(request.Question) == "" {
		return errors.New("question is required")
	}
	if request.PackID != "" && r.PackID != "" && request.PackID != r.PackID {
		return fmt.Errorf("ask_lore_expert is scoped to lore pack %q, got %q", r.PackID, request.PackID)
	}
	if !allowedPurpose(request.Purpose) {
		return fmt.Errorf("unsupported purpose %q", request.Purpose)
	}
	lower := strings.ToLower(request.Question)
	for _, term := range []string{
		"target_node_id",
		"storypacket",
		"story packet",
		"storybible",
		"story bible",
		"return json",
		"write the final",
		"write final",
		"player-facing prose",
		"narrator prose",
	} {
		if strings.Contains(lower, term) {
			return fmt.Errorf("ask_lore_expert is reference-only; request appears to ask for final story material via %q", term)
		}
	}
	return nil
}

func allowedPurpose(value string) bool {
	switch value {
	case "", "source_pattern", "source_ref_check", "source_mechanism", "law_mechanism", "character_pressure", "relationship_matrix", "plausibility_check", "tone_pattern", "fidelity_check", "scenario_contract", "lore_reference", "general_lore_reference", "general_influence":
		return true
	default:
		return false
	}
}

func (r *ToolRunner) searchSources(ctx context.Context, request ToolRequest) ([]lore.Snippet, error) {
	if r.Retriever == nil {
		return nil, nil
	}
	k := r.MaxSources
	if k <= 0 {
		k = 6
	}
	query := request.Question
	if request.Purpose != "" && request.Purpose != "general_lore_reference" && request.Purpose != "general_influence" {
		query = request.Purpose + " " + query
	}
	return r.Retriever.Search(ctx, query, k)
}

func (r *ToolRunner) promptText(request ToolRequest, snippets []lore.Snippet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the Nerd Lore Expert for Sans-Nom.\n\n")
	fmt.Fprintf(&b, "Your job is to answer reference questions about the selected lorepack so Writers and Narrators can preserve scenario fidelity without forcing generic dramatic rails.\n")
	fmt.Fprintf(&b, "You are not writing final prose, StoryBible guidance, StoryPacket nodes, or character cards.\n")
	fmt.Fprintf(&b, "Answer as reference support: explain what the source references support, what they do not support, and what would drift from the active lorepack notes.\n\n")
	fmt.Fprintf(&b, "When the source material includes rules, customs, ceremonies, family structures, or social constraints, describe only what the cited material supports and how it affects scene pressure.\n")
	fmt.Fprintf(&b, "Focus on mechanisms, relationships, and consequences rather than ornamental scenery or invented requirements.\n\n")
	fmt.Fprintf(&b, "Ground-lore pack:\n- id: %s\n- label: %s\n- collection: %s\n", request.PackID, r.packLabel(), r.Collection)
	if strings.TrimSpace(r.PackDescription) != "" {
		fmt.Fprintf(&b, "- description: %s\n", strings.TrimSpace(r.PackDescription))
	}
	fmt.Fprintf(&b, "\nQuestion purpose: %s\nQuestion:\n%s\n", request.Purpose, strings.TrimSpace(request.Question))
	if len(request.RelevantPaths) > 0 {
		fmt.Fprintf(&b, "\nRelevant path hints:\n")
		for _, path := range request.RelevantPaths {
			if strings.TrimSpace(path) != "" {
				fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(path))
			}
		}
	}
	if len(request.AvoidTerms) > 0 {
		fmt.Fprintf(&b, "\nAvoid terms or direct equivalents:\n")
		for _, term := range request.AvoidTerms {
			if strings.TrimSpace(term) != "" {
				fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(term))
			}
		}
	}
	fmt.Fprintf(&b, "\nRetrieved source references for analysis. Distill the fidelity implications; do not dump long source text:\n")
	if len(snippets) == 0 {
		fmt.Fprintf(&b, "- No source snippets were available. Answer from pack-level context only and say confidence is limited.\n")
	} else {
		for _, snippet := range snippets {
			label := snippet.ID
			if snippet.Kind != "" {
				label = snippet.Kind + " " + label
			}
			if snippet.Source != "" {
				label += " " + snippet.Source
			}
			fmt.Fprintf(&b, "\n### %s\n%s\n", label, preview(snippet.Text, 1400))
		}
	}
	fmt.Fprintf(&b, `
Return Markdown with exactly these sections:
## Lore reference answer
## Source basis
## Fidelity implications
## Pressure and consequence pattern
## Risks or contradictions
## Useful source refs

Keep the answer compact, concrete, and useful to a Creative Writer or Narrator. Do not invent unsupported source facts. Do not propose final plot beats as requirements.`)
	return strings.TrimSpace(b.String())
}

func (r *ToolRunner) result(cached bool, cacheDir string, cacheKey string, modelVisibleAnswerPath string, answer string, refs []SourceRef) map[string]any {
	cacheAnswerPath := filepath.ToSlash(filepath.Join(cacheDir, "answer.md"))
	answerPath := firstNonEmpty(modelVisibleAnswerPath, cacheAnswerPath)
	return map[string]any{
		"ok":                 true,
		"cached":             cached,
		"cache_key":          cacheKey,
		"answer_path":        answerPath,
		"cache_answer_path":  cacheAnswerPath,
		"model_visible_path": modelVisibleAnswerPath,
		"model_visible_root": "lore-expert",
		"source_refs":        refs,
		"preview":            preview(answer, 700),
		"note":               "Lore expert material is reference support for scenario fidelity. It is not final prose, StoryBible guidance, or StoryPacket structure. Read answer_path from the current agent workspace when more detail is needed.",
	}
}

func (r *ToolRunner) materializeAnswer(workspaceRoot string, packID string, cacheKey string, cacheDir string, answer string, refs []SourceRef, cached bool) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	relDir := filepath.Join("lore-expert", safeSegment(packID), safeSegment(cacheKey))
	absDir := filepath.Join(absRoot, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", err
	}
	if err := writeTextAtomic(filepath.Join(absDir, "answer.md"), strings.TrimSpace(answer)+"\n"); err != nil {
		return "", err
	}
	if err := writeJSONAtomic(filepath.Join(absDir, "sources.json"), refs); err != nil {
		return "", err
	}
	metadata := map[string]any{
		"cached":            cached,
		"cache_answer_path": filepath.ToSlash(filepath.Join(cacheDir, "answer.md")),
		"cache_key":         cacheKey,
		"pack_id":           packID,
		"created_at":        time.Now().UTC().Format(time.RFC3339Nano),
		"planning_only":     true,
	}
	if err := writeJSONAtomic(filepath.Join(absDir, "metadata.json"), metadata); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(relDir, "answer.md")), nil
}

func (r *ToolRunner) cacheKey(request ToolRequest) (string, error) {
	payload := map[string]any{
		"prompt_version": PromptVersion,
		"pack_id":        request.PackID,
		"collection":     r.Collection,
		"request":        request,
		"pack_manifest":  fileSHA256(r.PackManifestPath),
		"index_manifest": fileSHA256(r.IndexManifestPath),
		"master_lore":    fileSHA256(r.MasterLorePath),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func sourceRefs(snippets []lore.Snippet) []SourceRef {
	refs := make([]SourceRef, 0, len(snippets))
	for _, snippet := range snippets {
		refs = append(refs, SourceRef{
			ID:     snippet.ID,
			Kind:   snippet.Kind,
			Source: snippet.Source,
			Score:  snippet.Score,
		})
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Score == refs[j].Score {
			return refs[i].ID < refs[j].ID
		}
		return refs[i].Score > refs[j].Score
	})
	return refs
}

func readSourceRefs(path string) []SourceRef {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var refs []SourceRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil
	}
	return refs
}

func (r *ToolRunner) cacheRoot() string {
	if strings.TrimSpace(r.CacheRoot) != "" {
		return filepath.Clean(r.CacheRoot)
	}
	root := r.RepoRoot
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, ".cache", "sans-nom", "lore-expert")
}

func (r *ToolRunner) packLabel() string {
	return firstNonEmpty(r.PackLabel, r.PackID, "selected lore pack")
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
	speaker := firstNonEmpty(r.CallerSpeaker, role)
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

func (r *ToolRunner) logFailed(arguments string, request *ToolRequest, cacheKey string, err error) {
	if err == nil {
		return
	}
	fields := map[string]any{"argument_chars": len(arguments)}
	if request != nil {
		fields["pack_id"] = request.PackID
		fields["purpose"] = request.Purpose
	}
	if cacheKey != "" {
		fields["cache_key"] = cacheKey
	}
	metadata := r.modelMetadata()
	r.log(gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "lore",
		Event:    "lore.expert.failed",
		Message:  err.Error(),
		Fields:   modelmeta.MergeFields(fields, metadata),
		Detail: map[string]any{
			"arguments":      arguments,
			"request":        request,
			"cache_key":      cacheKey,
			"error":          err.Error(),
			"model_metadata": metadata,
		},
	})
}

func (r *ToolRunner) modelMetadata() modelmeta.Metadata {
	if r == nil || r.Runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := r.Runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

func toolSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"pack_id": map[string]any{
				"type":        "string",
				"description": "Optional lore pack id. Defaults to the selected ground-lore pack for this story.",
			},
			"question": map[string]any{
				"type":        "string",
				"description": "Reference question. Ask for lorepack patterns, source refs, tone, relationship pressure, contradiction risks, or scenario-fidelity checks.",
			},
			"purpose": map[string]any{
				"type": "string",
				"enum": []string{
					"source_pattern",
					"source_ref_check",
					"source_mechanism",
					"character_pressure",
					"relationship_matrix",
					"plausibility_check",
					"tone_pattern",
					"fidelity_check",
					"scenario_contract",
					"lore_reference",
					"general_lore_reference",
					"general_influence",
				},
				"description": "The kind of lore reference analysis requested.",
			},
			"relevant_paths": map[string]any{
				"type":        "array",
				"description": "Optional repo paths the expert should consider as source hints.",
				"items":       map[string]any{"type": "string"},
			},
			"avoid_terms": map[string]any{
				"type":        "array",
				"description": "Optional source terms the answer should treat carefully when explaining fidelity risks.",
				"items":       map[string]any{"type": "string"},
			},
		},
		"required": []string{"question"},
	}
}

func fileSHA256(path string) string {
	if strings.TrimSpace(path) == "" {
		return "none"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "missing:" + filepath.ToSlash(path)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeTextAtomic(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeTextAtomic(path, string(data))
}

func preview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func safeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
