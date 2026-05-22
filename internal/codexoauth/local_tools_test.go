package codexoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

func TestCreateWithLocalToolsLogsReadToolUseAndResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("workshop material\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls {
		case 1:
			writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"cat notes.md\"}"}}`))
			writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
		case 2:
			writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":"done"}]}}`))
			writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
		default:
			t.Fatalf("unexpected call %d", calls)
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthPath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	logger := gamelog.NewWriter(root, "story-1")
	resp, err := client.CreateWithLocalTools(context.Background(), map[string]any{
		"model": "gpt-5.5",
		"input": "read notes",
	}, LocalToolOptions{
		Root:          root,
		Logger:        logger,
		ToolCategory:  "writer",
		CallerRole:    "writer",
		CallerSpeaker: "Writer",
		RunID:         "run-1",
		ModelMetadata: modelmeta.Metadata{ProviderType: "codex_oauth", ModelProvider: "openai", Model: "gpt-5.5", ReasoningLevel: "medium"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp["output_text"] != "done" {
		t.Fatalf("response = %#v", resp)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Event != "writer.provider.tool_use" || events[0].Fields["tool_name"] != "Read" || events[0].Fields["target"] != "cat notes.md" || events[0].RunID != "run-1" {
		t.Fatalf("unexpected tool use event: %#v", events[0])
	}
	if events[1].Event != "writer.provider.tool_result" || !strings.Contains(events[1].Message, "workshop material") {
		t.Fatalf("unexpected tool result event: %#v", events[1])
	}
	detail, err := logger.ReadDetail(events[1].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(detail), "workshop material") {
		t.Fatalf("tool result detail missing stdout: %#v", detail)
	}
}

func TestCreateWithLocalToolsAdvertisesOnlyExistingVisiblePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lorepack"), 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Fatalf("missing tools: %#v", req)
		}
		first, ok := tools[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected tool: %#v", tools[0])
		}
		description, _ := first["description"].(string)
		if !strings.Contains(description, "lorepack/") {
			t.Fatalf("description missing staged lorepack path: %s", description)
		}
		for _, hidden := range []string{"story/", "scenarists/", "workshop/"} {
			if strings.Contains(description, hidden) {
				t.Fatalf("description advertised nonexistent %s: %s", hidden, description)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":"ok"}]}}`))
		writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthPath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateWithLocalTools(context.Background(), map[string]any{"model": "gpt-5.5", "input": "inspect"}, LocalToolOptions{
		Root:         root,
		VisiblePaths: []string{"story", "scenarists", "lorepack"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestToolExecCommandRejectsPathsOutsideScopedRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lorepack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lorepack", "lore-pack.json"), []byte("active pack"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lorepack", "comics", "peer-cards", "runs", "batch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lorepack", "comics", "peer-cards", "runs", "batch", "old-run.jsonl"), []byte("fixture leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := toolExecCommand(context.Background(), root, `{"cmd":"cat notes.md"}`); err != nil {
		t.Fatalf("scoped file should be readable: %v", err)
	}
	if _, err := toolExecCommand(context.Background(), root, `{"cmd":"cat lorepack/lore-pack.json"}`); err != nil {
		t.Fatalf("staged lorepack file should be readable: %v", err)
	}
	for _, cmd := range []string{
		`cat /Users/vincent/dev/sans-nom/README.md`,
		`cat ../README.md`,
		`rg Bell lore/packs`,
		`find data/stories`,
		`cat internal/seed/seed.go`,
		`cat lorepack/comics/peer-cards/runs/batch/old-run.jsonl`,
	} {
		args := fmt.Sprintf(`{"cmd":%q}`, cmd)
		if _, err := toolExecCommand(context.Background(), root, args); err == nil {
			t.Fatalf("expected command %q to be rejected", cmd)
		}
	}
}

func TestToolExecCommandAllowsQuotedRegexAlternation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lorepack", "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lorepack", "wiki", "lore.md"), []byte("succession\nhonor\nwound\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := toolExecCommand(context.Background(), root, `{"cmd":"rg -n \"succession|honor|wound|ritual|warrior|dynasty\" lorepack/wiki/lore.md"}`)
	if err != nil {
		t.Fatalf("quoted regex alternation should validate: %v", err)
	}
	stdout, _ := result["stdout"].(string)
	for _, want := range []string{"succession", "honor", "wound"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestToolExecCommandAllowsClassicGrep(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lorepack", "comics", "volumes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lorepack", "comics", "pages"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "lorepack", "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lorepack", "comics", "volumes", "volume-17.json"), []byte("Karbit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lorepack", "wiki", "lore.md"), []byte("Okhar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := toolExecCommand(context.Background(), root, `{"cmd":"grep -R \"Karbit\\|Okhar\\|Okkhar\\|leodact\" -n lorepack/comics/volumes lorepack/comics/pages lorepack/wiki/lore.md"}`)
	if err != nil {
		t.Fatalf("classic grep should validate: %v", err)
	}
	stdout, _ := result["stdout"].(string)
	for _, want := range []string{"Karbit", "Okhar"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestToolExecCommandShellParserKeepsGuards(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	allowed := []string{
		`cat notes.md | head -1`,
		`jq ".items[] | .name" notes.md`,
	}
	for _, cmd := range allowed {
		args := fmt.Sprintf(`{"cmd":%q}`, cmd)
		if _, err := toolExecCommand(context.Background(), root, args); err != nil {
			t.Fatalf("expected command %q to validate: %v", cmd, err)
		}
	}
	rejected := []string{
		`cat notes.md | sort | uniq | head -1 | wc -l`,
		`cat notes.md && head notes.md`,
		`cat notes.md > out.md`,
		`cat $(pwd)`,
		`FOO=bar cat notes.md`,
	}
	for _, cmd := range rejected {
		args := fmt.Sprintf(`{"cmd":%q}`, cmd)
		if _, err := toolExecCommand(context.Background(), root, args); err == nil {
			t.Fatalf("expected command %q to be rejected", cmd)
		}
	}
}

func writeSSE(t *testing.T, w http.ResponseWriter, raw map[string]any) {
	t.Helper()
	data := mustMarshalJSON(t, raw)
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		t.Fatal(err)
	}
}

func mapStringAny(raw string) map[string]any {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		panic(err)
	}
	return value
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
