package agentfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopeNormalizeRejectsRepoSurfaces(t *testing.T) {
	scope := New(filepath.Join(t.TempDir(), "agent"), "writer")
	for _, path := range []string{
		"../outside.md",
		"/Users/vincent/dev/sans-nom/README.md",
		"cmd/cli/main.go",
		"internal/seed/seed.go",
		"testdata/sample.json",
		"lore/packs/metabarons/lore-pack.json",
		"lorepack/comics/peer-cards/runs/batch/old-run.jsonl",
		"lorepack/comics/peer-cards/provenance/inputs/tonto.json",
		"lorepack/index/documents.jsonl",
		"lorepack/testdata/fixture.json",
		"data/stories/story-1/bible.json",
		"data/seed-runs/seed-1/workshop/output.md",
	} {
		if _, err := scope.Normalize(path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestScopeStagesVisibleFilesInsideRoot(t *testing.T) {
	scope := New(filepath.Join(t.TempDir(), "agent"), "writer")
	rel, err := scope.StageText("story/context.txt", "context")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "story/context.txt" {
		t.Fatalf("rel = %q", rel)
	}
	abs, err := scope.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := scope.Normalize(abs); err != nil || got != rel {
		t.Fatalf("normalize staged abs = %q, %v", got, err)
	}
}

func TestScopeStagesLorepackDirectoryInsideRoot(t *testing.T) {
	source := filepath.Join(t.TempDir(), "pack")
	if err := os.MkdirAll(filepath.Join(source, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "lore-pack.json"), []byte(`{"id":"pack"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "wiki", "lore.md"), []byte("lore"), 0o644); err != nil {
		t.Fatal(err)
	}

	scope := New(filepath.Join(t.TempDir(), "agent"), "writer")
	staged, err := scope.StageDir(source, "lorepack")
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 2 {
		t.Fatalf("staged = %#v", staged)
	}
	if _, err := scope.Normalize("lorepack/lore-pack.json"); err != nil {
		t.Fatalf("staged lorepack path should be accepted: %v", err)
	}
	if _, err := scope.Normalize("lore/packs/metabarons/lore-pack.json"); err == nil {
		t.Fatal("raw repo lore path should still be rejected")
	}
}
