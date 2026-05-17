package lore

import (
	"context"
	"testing"
)

type staticRetriever []Snippet

func (r staticRetriever) Search(context.Context, string, int) ([]Snippet, error) {
	return []Snippet(r), nil
}

func TestMultiRetrieverMergesDedupesAndSorts(t *testing.T) {
	results, err := MultiRetriever{Retrievers: []Retriever{
		staticRetriever{{ID: "global", Source: "global", Score: 0.5}, {ID: "shared", Source: "x", Score: 0.2}},
		staticRetriever{{ID: "story", Source: "story", Score: 0.9}, {ID: "shared", Source: "x", Score: 0.7}},
	}}.Search(context.Background(), "query", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %#v", results)
	}
	if results[0].ID != "story" || results[1].ID != "shared" || results[1].Score != 0.7 {
		t.Fatalf("unexpected merged order: %#v", results)
	}
}
