package gamelog

import (
	"path/filepath"
	"testing"
)

func TestWriterAppendReadTailAndFilter(t *testing.T) {
	writer := NewWriter(t.TempDir(), "story-one")
	for _, event := range []Event{
		{Category: "story", Event: "story.created", Message: "created"},
		{Category: "lore", Event: "lore.search", Fields: map[string]any{"authorization": "Bearer abc", "query_sha256": "hash"}},
		{Category: "writer", Event: "writer.refresh"},
	} {
		if err := writer.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	events, err := Read(writer.Path, Filter{Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Category != "lore" || events[1].Category != "writer" {
		t.Fatalf("unexpected tail: %#v", events)
	}
	if events[0].StoryID != "story-one" {
		t.Fatalf("missing story id: %#v", events[0])
	}
	if events[0].Fields["authorization"] != "[redacted]" || events[0].Fields["query_sha256"] != "hash" {
		t.Fatalf("unexpected redaction: %#v", events[0].Fields)
	}
	filtered, err := Read(writer.Path, Filter{Category: "story"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Event != "story.created" {
		t.Fatalf("unexpected filtered events: %#v", filtered)
	}
}

func TestWriterRejectsBadCategory(t *testing.T) {
	writer := &Writer{Path: filepath.Join(t.TempDir(), "events.jsonl")}
	if err := writer.Append(Event{Category: "unknown", Event: "bad"}); err == nil {
		t.Fatal("expected bad category to fail")
	}
}

func TestRedactMapRecurses(t *testing.T) {
	got := RedactMap(map[string]any{
		"nested": map[string]any{"api_key": "secret"},
		"items":  []any{"ok", "Bearer token"},
	})
	nested := got["nested"].(map[string]any)
	if nested["api_key"] != "[redacted]" {
		t.Fatalf("nested secret leaked: %#v", got)
	}
	items := got["items"].([]any)
	if items[0] != "ok" || items[1] != "[redacted]" {
		t.Fatalf("array secret leaked: %#v", got)
	}
}
