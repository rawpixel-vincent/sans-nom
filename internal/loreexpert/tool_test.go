package loreexpert

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
)

type fakeRunner struct {
	calls  int
	prompt string
	answer string
}

func (r *fakeRunner) Run(_ context.Context, prompt string) (string, error) {
	r.calls++
	r.prompt = prompt
	if r.answer != "" {
		return r.answer, nil
	}
	return `## Lore reference answer
This source pattern supports consequence-heavy dynasty pressure.

## Source basis
Law works through public consequence.

## Fidelity implications
The world reacts before the plot explains.

## Pressure and consequence pattern
The cost must be visible in-scene.

## Risks or contradictions
Do not make the law arbitrary.

## Useful source refs
- source-a`, nil
}

type fakeRetriever struct {
	calls int
	query string
	k     int
	hits  []lore.Snippet
}

func (r *fakeRetriever) Search(_ context.Context, query string, k int) ([]lore.Snippet, error) {
	r.calls++
	r.query = query
	r.k = k
	return r.hits, nil
}

func TestToolCachesLoreExpertAnswer(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "lore-pack.json")
	index := filepath.Join(root, "index-manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"id":"metabarons"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte(`{"documents":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	retriever := &fakeRetriever{hits: []lore.Snippet{{
		ID:     "doc-a",
		Kind:   "wiki_lore",
		Source: "lore/packs/metabarons/wiki/lore.md",
		Score:  0.9,
		Text:   "A source pattern becomes credible because the social world reacts through cost, pressure, and consequence.",
	}}}
	tool := &ToolRunner{
		Runner:            runner,
		Retriever:         retriever,
		CacheRoot:         filepath.Join(root, ".cache"),
		PackID:            "metabarons",
		PackLabel:         "Metabarons",
		PackDescription:   "Body pressure and difficult custom.",
		Collection:        "sans_nom_lore_metabarons",
		PackManifestPath:  manifest,
		IndexManifestPath: index,
	}
	args := `{"question":"How does a source rule stay credible?","purpose":"source_mechanism"}`
	first, err := tool.Handle(context.Background(), root, args)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tool.Handle(context.Background(), root, args)
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
	if retriever.calls != 1 {
		t.Fatalf("retriever calls = %d", retriever.calls)
	}
	firstMap := first.(map[string]any)
	secondMap := second.(map[string]any)
	if firstMap["cached"].(bool) {
		t.Fatal("first call should miss cache")
	}
	if !secondMap["cached"].(bool) {
		t.Fatal("second call should hit cache")
	}
	answerPath := secondMap["answer_path"].(string)
	if filepath.IsAbs(answerPath) {
		t.Fatalf("model-visible answer path should be workspace-relative, got %q", answerPath)
	}
	if _, err := os.Stat(filepath.Join(root, answerPath)); err != nil {
		t.Fatalf("answer path missing: %v", err)
	}
	cacheAnswerPath := secondMap["cache_answer_path"].(string)
	if !filepath.IsAbs(cacheAnswerPath) {
		t.Fatalf("cache answer path should stay durable and absolute, got %q", cacheAnswerPath)
	}
	if _, err := os.Stat(cacheAnswerPath); err != nil {
		t.Fatalf("cache answer path missing: %v", err)
	}
	if !strings.Contains(runner.prompt, "## Lore reference answer") ||
		!strings.Contains(runner.prompt, "Fidelity implications") ||
		!strings.Contains(runner.prompt, "rules, customs, ceremonies") ||
		!strings.Contains(runner.prompt, "scenario fidelity") {
		t.Fatalf("prompt missing required answer contract:\n%s", runner.prompt)
	}
	refs := secondMap["source_refs"].([]SourceRef)
	if len(refs) != 1 || refs[0].ID != "doc-a" {
		t.Fatalf("unexpected source refs: %#v", refs)
	}
}

func TestToolLogsCallerRole(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "lore-pack.json")
	index := filepath.Join(root, "index-manifest.json")
	if err := os.WriteFile(manifest, []byte(`{"id":"metabarons"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, []byte(`{"documents":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := gamelog.NewGlobalWriter(root)
	tool := &ToolRunner{
		Runner:            &fakeRunner{},
		CacheRoot:         filepath.Join(root, ".cache"),
		PackID:            "metabarons",
		PackManifestPath:  manifest,
		IndexManifestPath: index,
		Logger:            logger,
		CallerRole:        "creative-writer",
		CallerSpeaker:     "Creative Writer",
	}
	if _, err := tool.Handle(context.Background(), root, `{"question":"What makes a source custom credible?","purpose":"general_lore_reference"}`); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "lore.expert.request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("request events = %#v", events)
	}
	if events[0].Fields["caller_role"] != "creative-writer" || events[0].Fields["caller_speaker"] != "Creative Writer" {
		t.Fatalf("missing caller fields: %#v", events[0].Fields)
	}
	detail, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"caller_role":"creative-writer"`) {
		t.Fatalf("missing caller detail: %s", text)
	}
}

func TestToolRejectsFinalStoryRequests(t *testing.T) {
	tool := &ToolRunner{PackID: "metabarons", Runner: &fakeRunner{}}
	_, err := tool.Handle(context.Background(), t.TempDir(), `{"question":"Write the final StoryPacket with target_node_id values","purpose":"general_lore_reference"}`)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(err.Error(), "reference-only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalToolSchemaIsLoreAuthority(t *testing.T) {
	tool := (&ToolRunner{PackID: "metabarons"}).LocalTool()
	if tool.Name != "ask_lore_expert" {
		t.Fatalf("tool name = %q", tool.Name)
	}
	if !strings.Contains(tool.Description, "Lore reference support") || !strings.Contains(tool.Description, "scenario-fidelity") || !strings.Contains(tool.Description, "contradiction checks") {
		t.Fatalf("description should be lore-reference scoped: %s", tool.Description)
	}
	data, err := json.Marshal(tool.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "source_pattern") || !strings.Contains(string(data), "source_mechanism") || !strings.Contains(string(data), "fidelity_check") {
		t.Fatalf("schema missing purpose enum: %s", data)
	}
	if !strings.Contains(string(data), "scenario-fidelity") {
		t.Fatalf("schema missing fidelity guidance: %s", data)
	}
}
