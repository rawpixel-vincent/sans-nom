package lore

import (
	"context"
	"testing"
)

func TestMemoryIndexSearch(t *testing.T) {
	idx, err := NewMemoryIndex(context.Background(), FakeEmbedder{}, []Snippet{{
		ID:     "eastern",
		Text:   "The eastern hall contains Steelhead's stasis pod.",
		Source: "test",
	}, {
		ID:     "food",
		Text:   "Sans-Nom ate breakfast quietly.",
		Source: "test",
	}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search(context.Background(), "stasis pod eastern hall", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "eastern" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
