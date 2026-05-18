package loreexpert

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	return `## Influence-only answer
This is influence-only inspiration, not canon for the generated story.

## Source pattern
Law works through public consequence.

## Why it works
The world reacts before the plot explains.

## How to transform it originally
Invent a new practice with a new cost.

## Suspension-of-disbelief risks
Do not make the law arbitrary.

## Do not copy
No canon names or direct equivalents.

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
		Text:   "A source law becomes credible because the social world reacts through cost, witness, and consequence.",
	}}}
	tool := &ToolRunner{
		Runner:            runner,
		Retriever:         retriever,
		CacheRoot:         filepath.Join(root, ".cache"),
		PackID:            "metabarons",
		PackLabel:         "Metabarons",
		PackDescription:   "Body law and taboo pressure.",
		Collection:        "sans_nom_lore_metabarons",
		PackManifestPath:  manifest,
		IndexManifestPath: index,
	}
	args := `{"question":"How does a binding law stay credible?","purpose":"law_mechanism"}`
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
	if _, err := os.Stat(answerPath); err != nil {
		t.Fatalf("answer path missing: %v", err)
	}
	if !strings.Contains(runner.prompt, "## Influence-only answer") ||
		!strings.Contains(runner.prompt, "Suspension-of-disbelief risks") {
		t.Fatalf("prompt missing required answer contract:\n%s", runner.prompt)
	}
	refs := secondMap["source_refs"].([]SourceRef)
	if len(refs) != 1 || refs[0].ID != "doc-a" {
		t.Fatalf("unexpected source refs: %#v", refs)
	}
}

func TestToolRejectsFinalStoryRequests(t *testing.T) {
	tool := &ToolRunner{PackID: "metabarons", Runner: &fakeRunner{}}
	_, err := tool.Handle(context.Background(), t.TempDir(), `{"question":"Write the final StoryPacket with target_node_id values","purpose":"general_influence"}`)
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(err.Error(), "influence-only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalToolSchemaIsInfluenceOnly(t *testing.T) {
	tool := (&ToolRunner{PackID: "metabarons"}).LocalTool()
	if tool.Name != "ask_lore_expert" {
		t.Fatalf("tool name = %q", tool.Name)
	}
	if !strings.Contains(tool.Description, "influence-only") {
		t.Fatalf("description should be influence-only: %s", tool.Description)
	}
	data, err := json.Marshal(tool.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "suspension_check") {
		t.Fatalf("schema missing purpose enum: %s", data)
	}
}
