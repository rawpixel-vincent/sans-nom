package agentfs

import (
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
