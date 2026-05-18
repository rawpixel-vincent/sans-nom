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
)

const PromptVersion = "lore-expert-v1"

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
	MaxSources        int
	Logger            *gamelog.Writer
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
		Description: "Ask a nerd lore expert about the selected ground-lore pack. Answers are influence-only craft analysis for inspiration, never story canon, final prose, StoryBible doctrine, or StoryPacket structure. Results are cached per lore pack and returned as file paths plus a short preview.",
		Parameters:  toolSchema(),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, _ string, arguments string) (any, error) {
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
	request.Purpose = firstNonEmpty(request.Purpose, "general_influence")
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
	if answer, err := os.ReadFile(answerPath); err == nil && strings.TrimSpace(string(answer)) != "" {
		refs := readSourceRefs(filepath.Join(cacheDir, "sources.json"))
		r.log(gamelog.Event{
			Category: "lore",
			Event:    "lore.expert.cache_hit",
			Message:  "lore expert cache hit",
			Fields: map[string]any{
				"pack_id":     request.PackID,
				"purpose":     request.Purpose,
				"cache_key":   cacheKey,
				"answer_path": filepath.ToSlash(answerPath),
			},
			Detail: map[string]any{
				"request":     request,
				"cache_key":   cacheKey,
				"cache_dir":   filepath.ToSlash(cacheDir),
				"answer_path": filepath.ToSlash(answerPath),
				"source_refs": refs,
			},
		})
		return r.result(true, cacheDir, cacheKey, string(answer), refs), nil
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
		Fields: map[string]any{
			"pack_id":      request.PackID,
			"purpose":      request.Purpose,
			"cache_key":    cacheKey,
			"source_count": len(refs),
			"prompt_chars": len(promptText),
			"prompt_path":  filepath.ToSlash(filepath.Join(cacheDir, "prompt.txt")),
		},
		Detail: map[string]any{
			"request":     request,
			"cache_key":   cacheKey,
			"cache_dir":   filepath.ToSlash(cacheDir),
			"prompt":      promptText,
			"source_refs": refs,
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
		"created_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"planning_only":  true,
	}
	if err := writeJSONAtomic(filepath.Join(cacheDir, "metadata.json"), meta); err != nil {
		r.logFailed(arguments, &request, cacheKey, err)
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "lore",
		Event:    "lore.expert.response",
		Message:  "lore expert answer written",
		Fields: map[string]any{
			"pack_id":      request.PackID,
			"purpose":      request.Purpose,
			"cache_key":    cacheKey,
			"answer_chars": len(answer),
			"answer_path":  filepath.ToSlash(answerPath),
		},
		Detail: map[string]any{
			"request":     request,
			"metadata":    meta,
			"source_refs": refs,
			"answer":      answer,
		},
	})
	return r.result(false, cacheDir, cacheKey, answer, refs), nil
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
			return fmt.Errorf("ask_lore_expert is influence-only; request appears to ask for final story material via %q", term)
		}
	}
	return nil
}

func allowedPurpose(value string) bool {
	switch value {
	case "", "law_mechanism", "character_pressure", "relationship_matrix", "suspension_check", "tone_pattern", "anti_copy_check", "general_influence":
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
	if request.Purpose != "" && request.Purpose != "general_influence" {
		query = request.Purpose + " " + query
	}
	return r.Retriever.Search(ctx, query, k)
}

func (r *ToolRunner) promptText(request ToolRequest, snippets []lore.Snippet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are the Nerd Lore Expert for Sans-Nom.\n\n")
	fmt.Fprintf(&b, "Your job is to answer questions about a ground-lore pack as craft influence only.\n")
	fmt.Fprintf(&b, "You are not writing generated story canon, final prose, StoryBible doctrine, StoryPacket nodes, character cards, or direct equivalents.\n")
	fmt.Fprintf(&b, "Every answer must clearly say it is influence-only inspiration, not canon for the generated story.\n\n")
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
	fmt.Fprintf(&b, "\nRetrieved source references for analysis. Distill patterns; do not dump or copy them:\n")
	if len(snippets) == 0 {
		fmt.Fprintf(&b, "- No source snippets were available. Answer from pack-level craft doctrine only and say confidence is limited.\n")
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
## Influence-only answer
## Source pattern
## Why it works
## How to transform it originally
## Suspension-of-disbelief risks
## Do not copy
## Useful source refs

Keep the answer compact, concrete, and useful to a Creative Writer. Do not invent unsupported source facts. Do not propose final plot beats as requirements.`)
	return strings.TrimSpace(b.String())
}

func (r *ToolRunner) result(cached bool, cacheDir string, cacheKey string, answer string, refs []SourceRef) map[string]any {
	return map[string]any{
		"ok":          true,
		"cached":      cached,
		"cache_key":   cacheKey,
		"answer_path": filepath.ToSlash(filepath.Join(cacheDir, "answer.md")),
		"source_refs": refs,
		"preview":     preview(answer, 700),
		"note":        "Lore expert material is influence-only planning support. It is not story canon, not StoryBible doctrine, and not final StoryPacket structure.",
	}
}

func (r *ToolRunner) cacheKey(request ToolRequest) (string, error) {
	payload := map[string]any{
		"prompt_version": PromptVersion,
		"pack_id":        request.PackID,
		"collection":     r.Collection,
		"request":        request,
		"pack_manifest":  fileSHA256(r.PackManifestPath),
		"index_manifest": fileSHA256(r.IndexManifestPath),
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
	_ = r.Logger.Append(event)
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
	r.log(gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "lore",
		Event:    "lore.expert.failed",
		Message:  err.Error(),
		Fields:   fields,
		Detail: map[string]any{
			"arguments": arguments,
			"request":   request,
			"cache_key": cacheKey,
			"error":     err.Error(),
		},
	})
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
				"description": "Influence-only lore/craft question. Ask about patterns, mechanisms, risks, relationships, tone, or suspension of disbelief, not final story material.",
			},
			"purpose": map[string]any{
				"type": "string",
				"enum": []string{
					"law_mechanism",
					"character_pressure",
					"relationship_matrix",
					"suspension_check",
					"tone_pattern",
					"anti_copy_check",
					"general_influence",
				},
				"description": "The kind of influence analysis requested.",
			},
			"relevant_paths": map[string]any{
				"type":        "array",
				"description": "Optional repo paths the expert should consider as source hints.",
				"items":       map[string]any{"type": "string"},
			},
			"avoid_terms": map[string]any{
				"type":        "array",
				"description": "Optional source terms or direct equivalents to explicitly avoid.",
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
