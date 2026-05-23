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
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

func TestCreateWithLocalToolsLogsReadToolUseAndResult(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "0")

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
	if len(events) != 6 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Event != "writer.provider.request" || events[0].RunID != "run-1" {
		t.Fatalf("unexpected provider request event: %#v", events[0])
	}
	if events[1].Event != "writer.provider.response" || fmt.Sprint(events[1].Fields["tool_call_count"]) != "1" {
		t.Fatalf("unexpected provider response event: %#v", events[1])
	}
	toolEvents := eventsNamed(events, "writer.provider.tool_use", "writer.provider.tool_result")
	if len(toolEvents) != 2 {
		t.Fatalf("tool events = %#v", toolEvents)
	}
	if toolEvents[0].Fields["tool_name"] != "Read" || toolEvents[0].Fields["target"] != "cat notes.md" || toolEvents[0].RunID != "run-1" {
		t.Fatalf("unexpected tool use event: %#v", toolEvents[0])
	}
	if !strings.Contains(toolEvents[1].Message, "workshop material") {
		t.Fatalf("unexpected tool result event: %#v", toolEvents[1])
	}
	detail, err := logger.ReadDetail(toolEvents[1].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(detail), "workshop material") {
		t.Fatalf("tool result detail missing stdout: %#v", detail)
	}
}

func TestCreateWithLocalToolsRetriesWhenRequiredToolUseMissing(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "0")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "story"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "story", "current-node.json"), []byte(`{"id":"turn_3"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	var retryInput string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if calls == 2 {
			raw, err := json.Marshal(req["input"])
			if err != nil {
				t.Fatal(err)
			}
			retryInput = string(raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls {
		case 1:
			writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":"premature"}]}}`))
			writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
		case 2:
			writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"cat story/current-node.json\"}"}}`))
			writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
		case 3:
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
		"input": "draft packet",
	}, LocalToolOptions{
		Root:                  root,
		Logger:                logger,
		ToolCategory:          "writer",
		CallerRole:            "writer",
		CallerSpeaker:         "Writer",
		RunID:                 "run-required",
		VisiblePaths:          []string{"story"},
		RequireToolUse:        true,
		ToolUseRequiredReason: "Writer refreshes must perform one narrow grounding read before drafting.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp["output_text"] != "done" {
		t.Fatalf("response = %#v", resp)
	}
	if !strings.Contains(retryInput, "Writer refreshes must perform one narrow grounding read") || !strings.Contains(retryInput, "story/current-node.json") {
		t.Fatalf("retry input missing grounding reminder: %s", retryInput)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if retries := eventsNamed(events, "writer.provider.tool_required_retry"); len(retries) != 1 {
		t.Fatalf("required retry events = %#v", retries)
	}
	if toolEvents := eventsNamed(events, "writer.provider.tool_use", "writer.provider.tool_result"); len(toolEvents) != 2 {
		t.Fatalf("tool events = %#v", toolEvents)
	}
	if calls != 3 {
		t.Fatalf("provider calls = %d", calls)
	}
}

func TestCreateWithLocalToolsFailsWhenRequiredToolUseStillMissing(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "0")

	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","content":[{"type":"output_text","text":"still no tools"}]}}`))
		writeSSE(t, w, mapStringAny(`{"type":"response.completed","response":{}}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthPath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	logger := gamelog.NewWriter(root, "story-1")
	_, err = client.CreateWithLocalTools(context.Background(), map[string]any{
		"model": "gpt-5.5",
		"input": "draft packet",
	}, LocalToolOptions{
		Root:           root,
		MaxRounds:      2,
		Logger:         logger,
		ToolCategory:   "writer",
		RunID:          "run-required-fail",
		RequireToolUse: true,
	})
	if err == nil || !strings.Contains(err.Error(), "without required local tool call") {
		t.Fatalf("expected required tool error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d", calls)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	if failed := eventsNamed(events, "writer.provider.failed"); len(failed) != 1 {
		t.Fatalf("failed events = %#v", failed)
	}
}

func TestCreateWithLocalToolsLogsExtraToolUseAndResult(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "0")

	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	toolArgs, err := json.Marshal(map[string]any{
		"actor_id":    "tonto",
		"mode":        "present",
		"scene_setup": "The hatch opens.",
		"question":    "What do you risk saying?",
		"targets":     []string{"sans-nom"},
		"node_id":     "",
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls {
		case 1:
			writeSSE(t, w, map[string]any{
				"type":         "response.output_item.done",
				"output_index": 0,
				"item": map[string]any{
					"type":      "function_call",
					"call_id":   "call_actor",
					"name":      "ask_actor",
					"arguments": string(toolArgs),
				},
			})
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
		"input": "ask actor",
	}, LocalToolOptions{
		Root:          root,
		Logger:        logger,
		ToolCategory:  "writer",
		CallerRole:    "writer",
		CallerSpeaker: "Writer",
		RunID:         "run-actor",
		ExtraTools: []LocalTool{{
			Name:        "ask_actor",
			Description: "ask actor",
			Parameters:  map[string]any{"type": "object"},
			Handler: func(context.Context, string, string) (any, error) {
				return map[string]any{
					"ok": true,
					"material": map[string]any{
						"actor_id":      "tonto",
						"dialogue":      "I remember the door.",
						"artifact_path": "actors/actor-call-001/material.json",
					},
				}, nil
			},
		}},
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
	if len(events) != 6 {
		t.Fatalf("events = %#v", events)
	}
	toolEvents := eventsNamed(events, "writer.provider.tool_use", "writer.provider.tool_result")
	if len(toolEvents) != 2 {
		t.Fatalf("tool events = %#v", toolEvents)
	}
	for _, event := range toolEvents {
		if event.Fields["tool_name"] != "ask_actor" || event.Fields["raw_tool_name"] != "ask_actor" {
			t.Fatalf("extra tool event missing tool name: %#v", event)
		}
		if !strings.Contains(fmt.Sprint(event.Fields["target"]), "tonto") || event.RunID != "run-actor" {
			t.Fatalf("extra tool event missing target/run: %#v", event)
		}
	}
	if toolEvents[0].Event != "writer.provider.tool_use" || toolEvents[1].Event != "writer.provider.tool_result" {
		t.Fatalf("unexpected extra tool events: %#v", toolEvents)
	}
	detail, err := logger.ReadDetail(toolEvents[1].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fmt.Sprint(detail), "actor-call-001/material.json") {
		t.Fatalf("tool result detail missing actor artifact: %#v", detail)
	}
}

func TestCreateWithLocalToolsWaitsBetweenProviderRounds(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("workshop material\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(root, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"tokens":{"access_token":"token","account_id":"acct"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	var callTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		callTimes = append(callTimes, time.Now())
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls {
		case 1:
			writeSSE(t, w, mapStringAny(`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"cat notes.md\"}"}}`))
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
		Root:            root,
		MinCallInterval: 30 * time.Millisecond,
		Logger:          logger,
		ToolCategory:    "writer",
		RunID:           "run-wait",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp["output_text"] != "done" {
		t.Fatalf("response = %#v", resp)
	}
	if len(callTimes) != 2 {
		t.Fatalf("callTimes = %#v", callTimes)
	}
	if elapsed := callTimes[1].Sub(callTimes[0]); elapsed < 30*time.Millisecond {
		t.Fatalf("provider calls were not spaced out: %s", elapsed)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	waitEvents := eventsNamed(events, "writer.provider.wait")
	if len(waitEvents) != 1 {
		t.Fatalf("wait events = %#v", waitEvents)
	}
	if fmt.Sprint(waitEvents[0].Fields["min_call_interval_ms"]) != "30" {
		t.Fatalf("unexpected wait event: %#v", waitEvents[0])
	}
}

func TestLocalToolMinCallIntervalUsesEnv(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "25")

	if got := localToolMinCallInterval(LocalToolOptions{}); got != 25*time.Millisecond {
		t.Fatalf("interval = %s", got)
	}
}

func TestLocalToolMinCallIntervalDefaultsToTenSeconds(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "")

	if got := localToolMinCallInterval(LocalToolOptions{}); got != defaultMinCodexCallInterval {
		t.Fatalf("blank interval = %s", got)
	}

	t.Setenv(codexMinCallIntervalEnv, "not-a-number")
	if got := localToolMinCallInterval(LocalToolOptions{}); got != defaultMinCodexCallInterval {
		t.Fatalf("invalid interval = %s", got)
	}
}

func TestLocalToolMinCallIntervalCanBeDisabled(t *testing.T) {
	t.Setenv(codexMinCallIntervalEnv, "0")

	if got := localToolMinCallInterval(LocalToolOptions{}); got != 0 {
		t.Fatalf("interval = %s", got)
	}
}

func eventsNamed(events []gamelog.Event, names ...string) []gamelog.Event {
	allowed := map[string]struct{}{}
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	var out []gamelog.Event
	for _, event := range events {
		if _, ok := allowed[event.Event]; ok {
			out = append(out, event)
		}
	}
	return out
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
