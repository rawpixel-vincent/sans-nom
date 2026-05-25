package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

func TestAdminListsStoriesAndReadsDetail(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	if err := gamelog.NewWriter(ws.Dir, state.StoryID).Append(gamelog.Event{Category: "story", Event: "story.created"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var stories struct {
		Stories []map[string]any `json:"stories"`
	}
	getJSON(t, server.URL+"/admin/api/stories", &stories)
	if len(stories.Stories) != 1 || stories.Stories[0]["story_id"] != state.StoryID {
		t.Fatalf("unexpected stories: %#v", stories)
	}

	var detail storyDetail
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID, &detail)
	if detail.State.StoryID != state.StoryID || detail.Bible.Title == "" || detail.Packet == nil || detail.Turns == nil || detail.Narration == nil {
		t.Fatalf("unexpected detail: %#v", detail)
	}
	if detail.Summary.GroundLorePackID != "metabarons" || detail.GroundLorePack == nil {
		t.Fatalf("missing ground lore pack in detail: %#v", detail.Summary)
	}
}

func TestAdminListsLorePacks(t *testing.T) {
	server := httptest.NewServer(NewServer(t.TempDir(), bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Packs []map[string]any `json:"packs"`
	}
	getJSON(t, server.URL+"/admin/api/lore-packs", &resp)
	if len(resp.Packs) == 0 || resp.Packs[0]["id"] != "metabarons" {
		t.Fatalf("unexpected lore packs: %#v", resp)
	}
}

func TestAdminExposesGlobalLogsAndDetails(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "story",
		Event:    "story.seed.started",
		Detail:   map[string]any{"prompt": "seed request"},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var logs struct {
		Events []gamelog.Event `json:"events"`
	}
	getJSON(t, server.URL+"/admin/api/logs", &logs)
	if len(logs.Events) != 1 || logs.Events[0].DetailPath == "" {
		t.Fatalf("unexpected global logs: %#v", logs.Events)
	}
	var detail struct {
		Detail map[string]any `json:"detail"`
	}
	getJSON(t, server.URL+"/admin/api/logs/detail?path="+logs.Events[0].DetailPath, &detail)
	if detail.Detail["prompt"] != "seed request" {
		t.Fatalf("unexpected global detail: %#v", detail.Detail)
	}
}

func TestAdminExposesGlobalBackstage(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "model",
		Event:    "seed.writer.request",
		Detail: map[string]any{
			"prompt": "make a seed",
			"model_metadata": map[string]any{
				"provider_type":   "codex_oauth",
				"model_provider":  "openai",
				"model":           "gpt-5.5",
				"reasoning_level": "medium",
				"service_tier":    "priority",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{
		Category: "model",
		Event:    "seed.writer.response",
		Detail:   map[string]any{"response": `{"bible":{},"packet":{}}`},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected backstage messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "System" || resp.Messages[0].Role != "creative-writer" || !strings.Contains(resp.Messages[0].Text, "make a seed") {
		t.Fatalf("unexpected request message: %#v", resp.Messages[0])
	}
	if resp.Messages[0].Model == nil || resp.Messages[0].Model.Model != "gpt-5.5" || resp.Messages[0].Model.ReasoningLevel != "medium" {
		t.Fatalf("missing model metadata: %#v", resp.Messages[0].Model)
	}
	if resp.Messages[1].Speaker != "Creative Writer" || resp.Messages[1].Status != "completed" {
		t.Fatalf("unexpected response message: %#v", resp.Messages[1])
	}
}

func TestAdminGlobalBackstageIncludesStoryNarratorMessages(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "narrator",
		Event:    "narrator.rollout.request",
		Turn:     1,
		PacketID: "packet-1",
		NodeID:   "node-1",
		Detail:   map[string]any{"prompt": "narrator prompt"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{
		Category: "narrator",
		Event:    "narrator.rollout.completed",
		Turn:     1,
		PacketID: "packet-1",
		NodeID:   "node-1",
		Detail:   map[string]any{"draft": map[string]any{"prose": "narrator prose"}},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected backstage messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "System" || resp.Messages[0].Role != "narrator" || !strings.Contains(resp.Messages[0].Text, "@Narrator narrator prompt") {
		t.Fatalf("unexpected narrator request message: %#v", resp.Messages[0])
	}
	if resp.Messages[1].Speaker != "Narrator" || resp.Messages[1].Status != "completed" || !strings.Contains(resp.Messages[1].Text, "narrator prose") {
		t.Fatalf("unexpected narrator response message: %#v", resp.Messages[1])
	}
	var detail struct {
		Detail map[string]any `json:"detail"`
	}
	getJSON(t, server.URL+"/admin/api/logs/detail?path="+resp.Messages[0].DetailPath, &detail)
	if detail.Detail["prompt"] != "narrator prompt" {
		t.Fatalf("global backstage story detail did not resolve: %#v", detail.Detail)
	}
}

func TestAdminBackstageAttributesToolRequestsToActiveWriter(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "model",
		Event:    "seed.workshop.request",
		Detail:   map[string]any{"prompt": "imagine"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{
		Category: "lore",
		Event:    "lore.expert.request",
		Detail: map[string]any{
			"request": map[string]any{"question": "What makes doctrine credible?"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	toolCall := resp.Messages[1]
	if toolCall.Speaker != "Creative Writer" || toolCall.Role != "creative-writer" {
		t.Fatalf("tool call should be attributed to Creative Writer, got %#v", toolCall)
	}
	if !strings.Contains(toolCall.Text, "@LoreExpert") {
		t.Fatalf("tool call text should address LoreExpert, got %q", toolCall.Text)
	}
}

func TestAdminBackstageShowsLoreFidelityContext(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "lore",
		Event:    "lore.fidelity.context",
		Turn:     2,
		PacketID: "packet-1",
		NodeID:   "node-2",
		Message:  "lore fidelity contract attached to narrator context",
		Detail: map[string]any{
			"role":              "narrator",
			"scenario_contract": "Lorepack Scenario Notes\n- story_lines:\n  - dynasty pressure matters",
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 1 {
		t.Fatalf("unexpected backstage messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "Lore Authority" || resp.Messages[0].Role != "lore-authority" || !strings.Contains(resp.Messages[0].Text, "dynasty pressure matters") {
		t.Fatalf("unexpected lore fidelity message: %#v", resp.Messages[0])
	}
}

func TestAdminBackstageShowsScenaristProviderToolCalls(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.provider.tool_use",
		Message:  "@Read data/seed-runs/example/output.md",
		Fields: map[string]any{
			"tool_name": "Read",
			"target":    "data/seed-runs/example/output.md",
		},
		Detail: map[string]any{
			"tool_name":   "Read",
			"tool_use_id": "toolu_1",
			"input":       map[string]any{"file_path": "data/seed-runs/example/output.md"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.provider.tool_result",
		Message:  "Read result for data/seed-runs/example/output.md\n\nworkshop material",
		Fields: map[string]any{
			"tool_name": "Read",
			"target":    "data/seed-runs/example/output.md",
		},
		Detail: map[string]any{
			"tool_name": "Read",
			"input":     map[string]any{"file_path": "data/seed-runs/example/output.md"},
			"result":    map[string]any{"content": "workshop material"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "Scenarist" || resp.Messages[0].Kind != "tool_request" || !strings.Contains(resp.Messages[0].Text, "@Read data/seed-runs/example/output.md") {
		t.Fatalf("unexpected tool request message: %#v", resp.Messages[0])
	}
	if resp.Messages[1].Speaker != "Read" || resp.Messages[1].Kind != "tool_response" || !strings.Contains(resp.Messages[1].Text, "workshop material") {
		t.Fatalf("unexpected tool result message: %#v", resp.Messages[1])
	}
}

func TestAdminBackstageShowsWriterAndNarratorReadToolCalls(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	longMaterial := strings.Repeat("workshop material ", 40) + "done"
	for _, event := range []gamelog.Event{
		{
			Category: "writer",
			Event:    "writer.provider.tool_use",
			Message:  "@Read cat data/seed-runs/example/output.md",
			Fields: map[string]any{
				"tool_name":      "Read",
				"target":         "cat data/seed-runs/example/output.md",
				"caller_role":    "writer",
				"caller_speaker": "Writer",
			},
			Detail: map[string]any{
				"tool_name":      "Read",
				"tool_use_id":    "call_1",
				"input":          map[string]any{"cmd": "cat data/seed-runs/example/output.md"},
				"caller_role":    "writer",
				"caller_speaker": "Writer",
			},
		},
		{
			Category: "writer",
			Event:    "writer.provider.tool_result",
			Message:  "Read result for cat data/seed-runs/example/output.md\n\nworkshop material",
			Fields: map[string]any{
				"tool_name": "Read",
				"target":    "cat data/seed-runs/example/output.md",
			},
			Detail: map[string]any{
				"tool_name": "Read",
				"input":     map[string]any{"cmd": "cat data/seed-runs/example/output.md"},
				"result": map[string]any{
					"stdout":           longMaterial,
					"stdout_truncated": true,
				},
			},
		},
		{
			Category: "narrator",
			Event:    "narrator.provider.tool_use",
			Message:  "@Grep rg turn-000001",
			Fields: map[string]any{
				"tool_name":      "Grep",
				"target":         "rg turn-000001",
				"caller_role":    "narrator",
				"caller_speaker": "Narrator",
			},
			Detail: map[string]any{
				"tool_name":      "Grep",
				"tool_use_id":    "call_2",
				"input":          map[string]any{"cmd": "rg turn-000001"},
				"caller_role":    "narrator",
				"caller_speaker": "Narrator",
			},
		},
		{
			Category: "narrator",
			Event:    "narrator.provider.tool_result",
			Message:  "Grep result for rg turn-000001\n\nturn found",
			Fields: map[string]any{
				"tool_name": "Grep",
				"target":    "rg turn-000001",
			},
			Detail: map[string]any{
				"tool_name": "Grep",
				"input":     map[string]any{"cmd": "rg turn-000001"},
				"result":    map[string]any{"stdout": "turn found"},
			},
		},
	} {
		if err := global.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 4 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "Writer" || resp.Messages[0].Role != "writer" || resp.Messages[0].Kind != "tool_request" || !strings.Contains(resp.Messages[0].Text, "@Read cat") {
		t.Fatalf("unexpected writer tool request: %#v", resp.Messages[0])
	}
	if resp.Messages[1].Speaker != "Read" || resp.Messages[1].Kind != "tool_response" || !strings.Contains(resp.Messages[1].Text, longMaterial) {
		t.Fatalf("unexpected writer tool response: %#v", resp.Messages[1])
	}
	if strings.Count(resp.Messages[1].Text, "Read result for cat data/seed-runs/example/output.md") != 1 {
		t.Fatalf("writer tool response double-prefixed result text: %q", resp.Messages[1].Text)
	}
	if !resp.Messages[1].PreviewTruncated {
		t.Fatalf("expected writer tool response preview to be marked truncated: %#v", resp.Messages[1])
	}
	if !resp.Messages[1].ToolOutputTruncated {
		t.Fatalf("expected writer tool response output to be marked truncated: %#v", resp.Messages[1])
	}
	if resp.Messages[2].Speaker != "Narrator" || resp.Messages[2].Role != "narrator" || resp.Messages[2].Kind != "tool_request" || !strings.Contains(resp.Messages[2].Text, "@Grep rg") {
		t.Fatalf("unexpected narrator tool request: %#v", resp.Messages[2])
	}
	if resp.Messages[3].Speaker != "Grep" || resp.Messages[3].Kind != "tool_response" || !strings.Contains(resp.Messages[3].Text, "turn found") {
		t.Fatalf("unexpected narrator tool response: %#v", resp.Messages[3])
	}
}

func TestAdminBackstageScenaristResponsesUseMaterialModelMetadata(t *testing.T) {
	root := t.TempDir()
	materialPath := filepath.Join("data", "seed-runs", "example", "scenarists", "tool-call-001", "01-germ.md")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(materialPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, materialPath), []byte("scenarist material"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{
		Category: "scenarist",
		Event:    "scenarist.ask.completed",
		Message:  "scenarist material written",
		Fields: map[string]any{
			"provider_type":  "claude_compatible_pool",
			"model_provider": "MINIMAX,XIAOMI,ZAI",
		},
		Detail: map[string]any{
			"manifest": map[string]any{
				"materials": []any{
					map[string]any{
						"topic_id": "germ",
						"topic":    "Germ",
						"path":     filepath.ToSlash(materialPath),
						"model_metadata": map[string]any{
							"provider_type":  "claude_compatible",
							"model_provider": "ZAI",
							"model":          "glm-4.7",
						},
					},
				},
			},
			"model_metadata": map[string]any{
				"provider_type":  "claude_compatible_pool",
				"model_provider": "MINIMAX,XIAOMI,ZAI",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/backstage?tail=10", &resp)
	if len(resp.Messages) != 1 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Model == nil || resp.Messages[0].Model.ModelProvider != "ZAI" || resp.Messages[0].Model.Model != "glm-4.7" {
		t.Fatalf("scenarist response used aggregate metadata instead of material metadata: %#v", resp.Messages[0].Model)
	}
}

func TestAdminBackstageShowsActorInvocationPrompt(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "actor",
		Event:    "actor.invoke.request",
		Turn:     2,
		PacketID: "packet-1",
		NodeID:   "node-1",
		Fields:   map[string]any{"actor_id": "tonto"},
		Detail:   map[string]any{"prompt": "actor prompt text"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{
		Category: "actor",
		Event:    "actor.invoke.completed",
		Turn:     2,
		PacketID: "packet-1",
		NodeID:   "node-1",
		Fields:   map[string]any{"actor_id": "tonto"},
		Detail:   map[string]any{"output": map[string]any{"dialogue": "actor answer"}},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID+"/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "Narrator" || !strings.Contains(resp.Messages[0].Text, "@tonto actor prompt text") {
		t.Fatalf("unexpected actor request bubble: %#v", resp.Messages[0])
	}
	if resp.Messages[1].Speaker != "tonto" || !strings.Contains(resp.Messages[1].Text, "actor answer") {
		t.Fatalf("unexpected actor response bubble: %#v", resp.Messages[1])
	}
}

func TestAdminBackstageShowsActorToolCalls(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "actor",
		Event:    "actor.tool.request",
		Turn:     3,
		RunID:    "run-actor",
		PacketID: "packet-1",
		NodeID:   "node-1",
		Fields: map[string]any{
			"actor_id":       "tonto",
			"caller_role":    "writer",
			"caller_speaker": "Writer",
			"call_dir":       filepath.Join(ws.Dir, "actors", "actor-call-001"),
		},
		Detail: map[string]any{
			"caller_role":    "writer",
			"caller_speaker": "Writer",
			"request": map[string]any{
				"actor_id": "tonto",
				"question": "What does Tonto risk saying?",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{
		Category: "actor",
		Event:    "actor.tool.completed",
		Turn:     3,
		RunID:    "run-actor",
		PacketID: "packet-1",
		NodeID:   "node-1",
		Fields: map[string]any{
			"actor_id":      "tonto",
			"artifact_path": filepath.Join(ws.Dir, "actors", "actor-call-001", "material.json"),
		},
		Detail: map[string]any{
			"material": map[string]any{
				"actor_id": "tonto",
				"dialogue": "I remember the door.",
				"action":   "Tonto touches the lintel.",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID+"/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected messages: %#v", resp.Messages)
	}
	if resp.Messages[0].Speaker != "Writer" || resp.Messages[0].Role != "writer" || resp.Messages[0].Kind != "tool_request" || !strings.Contains(resp.Messages[0].Text, "@tonto What does Tonto risk saying?") {
		t.Fatalf("unexpected actor tool request bubble: %#v", resp.Messages[0])
	}
	if resp.Messages[1].Speaker != "tonto" || resp.Messages[1].Role != "actor" || resp.Messages[1].Kind != "tool_response" || !strings.Contains(resp.Messages[1].Text, "I remember the door.") {
		t.Fatalf("unexpected actor tool response bubble: %#v", resp.Messages[1])
	}
	if len(resp.Messages[1].ArtifactPaths) != 1 || resp.Messages[1].ArtifactPaths[0] != "actors/actor-call-001/material.json" {
		t.Fatalf("unexpected actor tool artifact paths: %#v", resp.Messages[1].ArtifactPaths)
	}
}

func TestAdminExposesStoryBackstageArtifactsSafely(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	runDir := filepath.Join(ws.Dir, "model-runs", "writer", "turn-000001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "input.txt"), []byte("writer prompt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "accepted-packet.json"), []byte(`{"id":"packet"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "writer",
		Event:    "writer.refresh.started",
		Fields:   map[string]any{"artifact_dir": runDir},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{
		Category: "writer",
		Event:    "writer.refresh.completed",
		Fields:   map[string]any{"artifact_dir": runDir, "leaked": "../state.json"},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID+"/backstage?tail=10", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected backstage messages: %#v", resp.Messages)
	}
	if !strings.Contains(resp.Messages[0].Text, "writer prompt") {
		t.Fatalf("writer input was not loaded: %#v", resp.Messages[0])
	}
	if !strings.Contains(resp.Messages[1].Text, `"id":"packet"`) {
		t.Fatalf("accepted packet was not loaded: %#v", resp.Messages[1])
	}
	for _, path := range resp.Messages[1].ArtifactPaths {
		if strings.Contains(path, "..") {
			t.Fatalf("unsafe artifact path exposed: %q", path)
		}
	}
}

func TestAdminStoryBackstageTailsRenderedMessagesNotRawEvents(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{
		Category: "writer",
		Event:    "writer.provider.tool_use",
		Message:  "@Read cat story/context.txt",
		Fields: map[string]any{
			"tool_name":      "Read",
			"target":         "cat story/context.txt",
			"caller_role":    "writer",
			"caller_speaker": "Writer",
		},
		Detail: map[string]any{
			"tool_name":      "Read",
			"input":          map[string]any{"cmd": "cat story/context.txt"},
			"caller_role":    "writer",
			"caller_speaker": "Writer",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{
		Category: "writer",
		Event:    "writer.provider.tool_result",
		Message:  "Read result for cat story/context.txt\n\ncontext body",
		Fields: map[string]any{
			"tool_name": "Read",
			"target":    "cat story/context.txt",
		},
		Detail: map[string]any{
			"tool_name": "Read",
			"input":     map[string]any{"cmd": "cat story/context.txt"},
			"result":    map[string]any{"stdout": "context body"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := logger.Append(gamelog.Event{Category: "story", Event: "story.noise", Message: "raw log noise"}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	var resp struct {
		Messages []backstageMessage `json:"messages"`
	}
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID+"/backstage?tail=2", &resp)
	if len(resp.Messages) != 2 {
		t.Fatalf("unexpected backstage messages: %#v", resp.Messages)
	}
	if resp.Messages[0].SourceEvent != "writer.provider.tool_use" || resp.Messages[1].SourceEvent != "writer.provider.tool_result" {
		t.Fatalf("writer tool calls were hidden by raw log tailing: %#v", resp.Messages)
	}
}

func TestAdminBackstageStreamEmitsSnapshot(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{Category: "narrator", Event: "narrator.rollout.completed", Detail: map[string]any{"draft": map[string]any{"prose": "hello"}}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/admin/api/backstage/stream?tail=10", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("unexpected content type %q", got)
	}
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	body := string(buf[:n])
	if !strings.Contains(body, "event: backstage") || !strings.Contains(body, "hello") {
		t.Fatalf("unexpected stream body: %s", body)
	}
}

func TestAdminDeletesGlobalLog(t *testing.T) {
	root := t.TempDir()
	global := gamelog.NewGlobalWriter(root)
	if err := global.Append(gamelog.Event{Category: "story", Event: "delete.me", Detail: map[string]any{"payload": "gone"}}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{Category: "story", Event: "delete.me.too"}); err != nil {
		t.Fatal(err)
	}
	if err := global.Append(gamelog.Event{Category: "story", Event: "keep.me"}); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(global.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/api/logs?event_id="+events[0].EventID+"&event_id="+events[1].EventID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	remaining, err := gamelog.Read(global.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Event != "keep.me" {
		t.Fatalf("unexpected remaining global logs: %#v", remaining)
	}
}

func TestAdminDeletesStoryLog(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	logger := gamelog.NewWriter(ws.Dir, state.StoryID)
	if err := logger.Append(gamelog.Event{Category: "story", Event: "delete.me"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(gamelog.Event{Category: "story", Event: "keep.me"}); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/admin/api/stories/"+state.StoryID+"/logs?event_id="+events[0].EventID, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	remaining, err := gamelog.Read(logger.Path, gamelog.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Event != "keep.me" {
		t.Fatalf("unexpected remaining story logs: %#v", remaining)
	}
}

func TestAdminCreateStoryUsesSelectedPack(t *testing.T) {
	root := t.TempDir()
	var gotPackID string
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{
		NewStory: func(_ context.Context, packID string) (storyapp.StorySession, error) {
			gotPackID = packID
			ws, state, err := story.InitWorkspace(story.InitOptions{
				StoryID:              "created-story",
				Root:                 root,
				GroundLorePackID:     packID,
				GroundLoreCollection: story.DefaultGroundLoreCollection,
				Replace:              true,
			}, seed.DefaultBundle())
			if err != nil {
				return storyapp.StorySession{}, err
			}
			return storyapp.ResumeStory(context.Background(), state.StoryID, storyapp.StoryActionOptions{StoryRoot: ws.Root})
		},
	}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories", "application/json", strings.NewReader(`{"ground_lore_pack_id":"metabarons"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	if gotPackID != "metabarons" {
		t.Fatalf("pack id = %q", gotPackID)
	}
}

func TestAdminListsStoryActors(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	var gotStoryID string
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{
		ListActors: func(_ context.Context, storyID string) ([]ActorSummary, error) {
			gotStoryID = storyID
			return []ActorSummary{{
				ActorID:     "tonto",
				DisplayName: "Tonto",
				CardSource:  "cards/tonto.md",
				CardPreview: "Tonto tells witness-bound stories.",
			}}, nil
		},
	}))
	defer server.Close()

	var resp struct {
		Actors []ActorSummary `json:"actors"`
	}
	getJSON(t, server.URL+"/admin/api/stories/"+state.StoryID+"/actors", &resp)
	if gotStoryID != state.StoryID {
		t.Fatalf("story id = %q", gotStoryID)
	}
	if len(resp.Actors) != 1 || resp.Actors[0].ActorID != "tonto" || resp.Actors[0].CardSource == "" {
		t.Fatalf("unexpected actors: %#v", resp.Actors)
	}

	missing, err := http.Get(server.URL + "/admin/api/stories/missing/actors")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected missing story to reject actor list, got %s", missing.Status)
	}
}

func TestAdminActorChatValidatesRequest(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	called := false
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{
		ActorChat: func(context.Context, string, ActorChatRequest) (ActorChatResponse, error) {
			called = true
			return ActorChatResponse{}, nil
		},
	}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/actor-chat", "application/json", strings.NewReader(`{"message":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %s: %s", resp.Status, body)
	}
	if called {
		t.Fatal("actor chat callback should not run for invalid request")
	}
}

func TestAdminActorChatInvokesCallback(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	var gotStoryID string
	var gotReq ActorChatRequest
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{
		ActorChat: func(_ context.Context, storyID string, req ActorChatRequest) (ActorChatResponse, error) {
			gotStoryID = storyID
			gotReq = req
			return ActorChatResponse{
				ActorID:  req.ActorID,
				Dialogue: "I remember the hall.",
				Action:   "Tonto lowers his voice.",
				Warnings: []string{"read-only"},
				Model:    modelmeta.Metadata{ProviderType: "static", Model: "test"},
			}, nil
		},
	}))
	defer server.Close()

	resp, err := http.Post(
		server.URL+"/admin/api/stories/"+state.StoryID+"/actor-chat",
		"application/json",
		strings.NewReader(`{"actor_id":"tonto","message":"what do you see?","history":[{"role":"user","text":"hello"}]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	var out ActorChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if gotStoryID != state.StoryID || gotReq.ActorID != "tonto" || gotReq.Message == "" || len(gotReq.History) != 1 {
		t.Fatalf("callback did not receive request: story=%q req=%#v", gotStoryID, gotReq)
	}
	if out.Dialogue == "" || out.Model.Model != "test" {
		t.Fatalf("unexpected actor chat response: %#v", out)
	}
}

func TestAdminRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/admin/api/stories/" + state.StoryID + "/file?path=../state.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %s", resp.Status)
	}
}

func TestAdminControlRequiresConfirmation(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/rebuild-index", "application/json", strings.NewReader(`{"confirm":false}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected confirmation rejection, got %s", resp.Status)
	}
}

func TestAdminRebuildIndexEmitsControlLog(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/rebuild-index", "application/json", strings.NewReader(`{"confirm":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %s", resp.Status)
	}
	events, err := gamelog.Read(ws.LogsDir()+"/events.jsonl", gamelog.Filter{Category: "control"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[len(events)-1].Event != "rebuild-index.completed" {
		t.Fatalf("missing control log: %#v", events)
	}
}

func TestAdminDeleteStoryRequiresTypedConfirmation(t *testing.T) {
	root := t.TempDir()
	_, state := createAdminStory(t, root)
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient("", ""), "", Callbacks{}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/delete-story", "application/json", strings.NewReader(`{"confirm":true,"story_id_confirmation":"wrong"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected typed confirmation rejection, got %s", resp.Status)
	}
	if _, _, err := story.LoadWorkspace(root, state.StoryID); err != nil {
		t.Fatalf("story should still exist: %v", err)
	}
}

func TestAdminDeleteStoryRemovesLocalAndExternal(t *testing.T) {
	root := t.TempDir()
	ws, state := createAdminStory(t, root)
	var got bridgeclient.StoryDeleteRequest
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/story/delete" || r.Method != http.MethodPost {
			t.Fatalf("unexpected bridge request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(bridgeclient.StoryDeleteResponse{
			WorkspaceID:             got.WorkspaceID,
			Collection:              got.Collection,
			QdrantCollectionDeleted: true,
			HonchoWorkspaceDeleted:  true,
			HonchoSessionsDeleted:   1,
			HonchoLive:              true,
		})
	}))
	defer bridgeServer.Close()
	server := httptest.NewServer(NewServer(root, bridgeclient.NewClient(bridgeServer.URL, ""), "", Callbacks{}))
	defer server.Close()

	resp, err := http.Post(server.URL+"/admin/api/stories/"+state.StoryID+"/control/delete-story", "application/json", strings.NewReader(`{"confirm":true,"story_id_confirmation":"admin-story"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %s: %s", resp.Status, body)
	}
	if got.WorkspaceID != state.WorkspaceID || got.Collection != state.StoryCollection {
		t.Fatalf("unexpected bridge delete request: %#v", got)
	}
	if _, err := os.Stat(ws.Dir); !os.IsNotExist(err) {
		t.Fatalf("story dir should be removed, stat err=%v", err)
	}
}

func createAdminStory(t *testing.T, root string) (story.Workspace, story.StoryState) {
	t.Helper()
	ws, state, err := story.InitWorkspace(story.InitOptions{
		StoryID:              "admin-story",
		Root:                 root,
		GroundLorePackID:     "metabarons",
		GroundLoreCollection: story.DefaultGroundLoreCollection,
		Replace:              true,
	}, seed.DefaultBundle())
	if err != nil {
		t.Fatal(err)
	}
	return ws, state
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}
