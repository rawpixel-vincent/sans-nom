package gamelog

import (
	"os"
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
	if events[0].EventID == "" {
		t.Fatalf("missing event id: %#v", events[0])
	}
	filtered, err := Read(writer.Path, Filter{Category: "story"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Event != "story.created" {
		t.Fatalf("unexpected filtered events: %#v", filtered)
	}
}

func TestWriterStoresDetailPayload(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root, "story-one")
	if err := writer.Append(Event{
		Category: "model",
		Event:    "model.response",
		Detail: map[string]any{
			"prompt": "hello",
			"token":  "secret",
		},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := Read(writer.Path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("missing detail path: %#v", events)
	}
	detail, err := writer.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	data := detail.(map[string]any)
	if data["prompt"] != "hello" || data["token"] != "[redacted]" {
		t.Fatalf("unexpected detail payload: %#v", data)
	}
}

func TestWriterDeletesEventAndDetailPayload(t *testing.T) {
	root := t.TempDir()
	writer := NewWriter(root, "story-one")
	if err := writer.Append(Event{
		Category: "model",
		Event:    "model.response",
		Detail:   map[string]any{"response": "delete me"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(Event{Category: "story", Event: "story.kept"}); err != nil {
		t.Fatal(err)
	}
	events, err := Read(writer.Path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("unexpected events: %#v", events)
	}
	detailPath, err := writer.safeDetailPath(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok, err := writer.Delete(events[0].EventID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || deleted.EventID != events[0].EventID {
		t.Fatalf("unexpected delete result: ok=%v event=%#v", ok, deleted)
	}
	if _, err := os.Stat(detailPath); !os.IsNotExist(err) {
		t.Fatalf("detail file should be removed, stat err=%v", err)
	}
	remaining, err := Read(writer.Path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Event != "story.kept" {
		t.Fatalf("unexpected remaining events: %#v", remaining)
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
