package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

func TestParseDecisionAcceptsNumericChoice(t *testing.T) {
	node := scene.SceneNode{Choices: []scene.Choice{{
		ID:           "first",
		TargetNodeID: "first-node",
	}, {
		ID:           "second",
		TargetNodeID: "second-node",
	}}}
	decision := parseDecision("2", node)
	if decision.ChoiceID != "second" || decision.FreeText != "" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestResolveWriterScenaristCount(t *testing.T) {
	if got := resolveWriterScenaristCount(-1, "codex", true); got != 24 {
		t.Fatalf("default live refresh scenarists = %d", got)
	}
	if got := resolveWriterScenaristCount(-1, "static", true); got != 0 {
		t.Fatalf("static scenarists = %d", got)
	}
	if got := resolveWriterScenaristCount(99, "codex", true); got != 24 {
		t.Fatalf("clamped scenarists = %d", got)
	}
	if got := resolveWriterScenaristCount(0, "codex", true); got != 0 {
		t.Fatalf("explicit zero scenarists = %d", got)
	}
}

func TestSeedWriterRunnerGetsScenaristTool(t *testing.T) {
	t.Setenv("ZAI__ANTHROPIC_BASE_URL", "https://example.invalid")
	t.Setenv("ZAI__ANTHROPIC_AUTH_TOKEN", "token")
	t.Setenv("ZAI__ANTHROPIC_MODEL", "model")
	cfg := seedRuntimeConfig{
		Backend:           "codex",
		WriterScenarists:  1,
		ScenaristProvider: "ZAI",
		ScenaristRounds:   7,
	}
	outputDir := t.TempDir()
	toolRunner, err := seedScenaristToolRunner(cfg, "writer context", outputDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := seedWriterRunner(cfg, outputDir, toolRunner)
	if err != nil {
		t.Fatal(err)
	}
	codex, ok := runner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", runner)
	}
	if !codex.LocalTools || codex.MaxToolRounds != 7 {
		t.Fatalf("seed writer runner missing local tools: %#v", codex)
	}
	if len(codex.ExtraTools) != 1 || codex.ExtraTools[0].Name != "call_scenarists" {
		t.Fatalf("seed writer missing scenarist tool: %#v", codex.ExtraTools)
	}
	workshop, err := seedWorkshopRunner(cfg, outputDir, toolRunner)
	if err != nil {
		t.Fatal(err)
	}
	workshopCodex, ok := workshop.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", workshop)
	}
	if workshopCodex.SchemaName != "" || workshopCodex.Schema != nil {
		t.Fatalf("workshop runner should be free-form, got schema %q %#v", workshopCodex.SchemaName, workshopCodex.Schema)
	}
}

func TestRefreshWriterPlannerGetsScenaristTool(t *testing.T) {
	planner := writerPlannerWithMaterialAccess("codex", prompt.ClaudeRunner{}, t.TempDir(), &scenarist.ToolRunner{MaxParallel: 1}, 5)
	claude, ok := planner.(writer.ClaudePlanner)
	if !ok {
		t.Fatalf("expected ClaudePlanner, got %T", planner)
	}
	codex, ok := claude.Runner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", claude.Runner)
	}
	if !codex.LocalTools || codex.MaxToolRounds != 5 {
		t.Fatalf("refresh writer runner missing local tools: %#v", codex)
	}
	if len(codex.ExtraTools) != 1 || codex.ExtraTools[0].Name != "call_scenarists" {
		t.Fatalf("refresh writer missing scenarist tool: %#v", codex.ExtraTools)
	}
}

func TestBridgeDialFailureDetectsConnectionRefused(t *testing.T) {
	err := errors.New(`seed story collection "x": Post "http://127.0.0.1:8787/v1/story/index/ingest": dial tcp 127.0.0.1:8787: connect: connection refused`)
	if !bridgeDialFailure(err) {
		t.Fatal("expected connection-refused bridge error to be startup eligible")
	}
	if bridgeDialFailure(errors.New("sidecar POST /v1/story/index/ingest: 500 qdrant schema mismatch")) {
		t.Fatal("unexpectedly treated backend error as dial failure")
	}
}

func TestSynthesizeTurnAudioWritesWAVAndLogs(t *testing.T) {
	audio := []byte("RIFF fake wav")
	longText := strings.Repeat("Read this full narrator turn.", 1000)
	var gotCreate map[string]any
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing bearer token")
		}
		switch r.URL.Path {
		case "/v1/t2a_async_v2":
			if err := json.NewDecoder(r.Body).Decode(&gotCreate); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"task_id":          123,
				"file_id":          456,
				"usage_characters": 11,
				"base_resp":        map[string]any{"status_code": 0, "status_msg": "success"},
			})
		case "/v1/query/t2a_async_query_v2":
			statusCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"task_id":   123,
				"status":    "Success",
				"file_id":   456,
				"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
			})
		case "/v1/files/retrieve_content":
			_, _ = w.Write(audio)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	logger := gamelog.NewGlobalWriter(t.TempDir())
	outDir := t.TempDir()
	path, err := synthesizeTurnAudio(context.Background(), playConfig{
		Logger: logger,
		TTS: playTTSConfig{
			Enabled:   true,
			BaseURL:   server.URL,
			APIKey:    "secret",
			Model:     "speech-2.8-turbo",
			Voice:     "English_expressive_narrator",
			Format:    "wav",
			OutputDir: outDir,
		},
	}, nil, 1, narrator.Rollout{
		Draft: narrator.Draft{Prose: longText},
		Commit: scene.Commit{
			PacketID: "packet-1",
			NodeID:   "node-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "turn-000001.wav" {
		t.Fatalf("path = %s", path)
	}
	if statusCalls != 1 {
		t.Fatalf("status calls = %d", statusCalls)
	}
	if got, _ := gotCreate["text"].(string); len(got) != len(longText) || !strings.HasSuffix(got, "full narrator turn.") {
		t.Fatalf("create text was not submitted intact: got len %d want %d", len(got), len(longText))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(audio) {
		t.Fatalf("audio = %q", data)
	}
	for _, rel := range []string{
		filepath.Join("model-runs", "tts", "turn-000001", "request.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "create-response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "status.jsonl"),
		filepath.Join("model-runs", "tts", "turn-000001", "download-response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "metadata.json"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "tts.async.downloaded"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("missing tts completed detail: %#v", events)
	}
}

func TestSynthesizeTurnAudioFailurePreservesDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"base_resp": map[string]any{"status_code": 1001, "status_msg": "bad text"},
		})
	}))
	defer server.Close()
	logger := gamelog.NewGlobalWriter(t.TempDir())
	outDir := t.TempDir()
	_, err := synthesizeTurnAudio(context.Background(), playConfig{
		Logger: logger,
		TTS: playTTSConfig{
			Enabled:   true,
			BaseURL:   server.URL,
			APIKey:    "secret",
			Model:     "speech-2.8-turbo",
			Voice:     "English_expressive_narrator",
			Format:    "wav",
			OutputDir: outDir,
		},
	}, nil, 1, narrator.Rollout{
		Draft: narrator.Draft{Prose: "Bad text."},
		Commit: scene.Commit{
			PacketID: "packet-1",
			NodeID:   "node-1",
		},
	})
	if err == nil {
		t.Fatal("expected tts failure")
	}
	for _, rel := range []string{
		filepath.Join("model-runs", "tts", "turn-000001", "request.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "create-response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "error.txt"),
	} {
		if _, statErr := os.Stat(filepath.Join(outDir, rel)); statErr != nil {
			t.Fatalf("missing artifact %s: %v", rel, statErr)
		}
	}
	events, readErr := gamelog.Read(logger.Path, gamelog.Filter{Event: "tts.async.failed"})
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("missing failed event detail: %#v", events)
	}
}

func TestTTSOutputPathUsesConfiguredFormat(t *testing.T) {
	got := ttsOutputPath(playConfig{TTS: playTTSConfig{OutputDir: "audio-out", Format: "mp3"}}, nil, 2)
	if filepath.Base(got) != "turn-000002.mp3" {
		t.Fatalf("output path = %s", got)
	}
}

func TestBridgeListenAddressOnlySupportsLocalhost(t *testing.T) {
	host, port, err := bridgeListenAddress("http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port != "8787" {
		t.Fatalf("unexpected listen address %s:%s", host, port)
	}
	if _, _, err := bridgeListenAddress("https://example.com:8787"); err == nil {
		t.Fatal("expected remote bridge URL to reject auto-start")
	}
}

func TestBuildSeedContextDoesNotInjectStaticFixture(t *testing.T) {
	retriever := &seedContextRetriever{hits: []lore.Snippet{{
		ID:     "wiki_lore:bushitaka",
		Kind:   "wiki_lore",
		Source: "lore/packs/metabarons/wiki/lore.md",
		Score:  0.9,
		Text:   "A binding law works when the world reacts through belief, consequence, and social pressure.",
	}}}

	text := buildSeedContext(context.Background(), retriever)
	for _, forbidden := range []string{
		"Ila Venn",
		"Glasswell",
		"Veyr Tidemarket",
		"Mourning Lantern",
		"Law of Living Collateral",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("live seed context leaked static fixture term %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "A binding law works") {
		t.Fatalf("expected source-lore influence snippet in seed context:\n%s", text)
	}
	if retriever.query != seedInfluenceQuery || retriever.k != 8 {
		t.Fatalf("unexpected retrieval request query=%q k=%d", retriever.query, retriever.k)
	}
}

func TestValidatePacketSourceForBackendRequiresExplicitLivePacket(t *testing.T) {
	err := validatePacketSourceForBackend("codex", false, "testdata/packets/toy.json")
	if err == nil {
		t.Fatal("expected live backend to reject implicit packet")
	}
	if !strings.Contains(err.Error(), "testdata is never selected by default") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validatePacketSourceForBackend("codex", true, "testdata/packets/toy.json"); err != nil {
		t.Fatalf("explicit packet should be allowed: %v", err)
	}
	if err := validatePacketSourceForBackend("static", false, "testdata/packets/toy.json"); err != nil {
		t.Fatalf("static backend should allow implicit test packet: %v", err)
	}
	if err := validatePacketSourceForBackend("static", true, ""); err == nil {
		t.Fatal("expected empty packet path to fail")
	}
}

func TestShouldCreateStoryDefaultsToLiveStory(t *testing.T) {
	root := t.TempDir()
	if !shouldCreateStory("", "", root, false) {
		t.Fatal("expected missing story source to create a live story")
	}
	if shouldCreateStory("", "testdata/packets/toy.json", root, false) {
		t.Fatal("explicit packet should not create a story")
	}
	if !shouldCreateStory("new-id", "", root, false) {
		t.Fatal("missing story id should create a story")
	}
	if err := os.MkdirAll(filepath.Join(root, "existing-id"), 0o755); err != nil {
		t.Fatal(err)
	}
	if shouldCreateStory("existing-id", "", root, false) {
		t.Fatal("existing story id should resume instead of creating")
	}
	if !shouldCreateStory("existing-id", "", root, true) {
		t.Fatal("replace should recreate an existing story")
	}
}

func TestShouldRunLauncherAllowsTTSConfigOnlyFlags(t *testing.T) {
	if !shouldRunLauncher(true, nil) {
		t.Fatal("no args should launch menu")
	}
	if !shouldRunLauncher(false, map[string]struct{}{"tts": {}}) {
		t.Fatal("tts-only run should still launch menu")
	}
	if !shouldRunLauncher(false, map[string]struct{}{"tts": {}, "tts-base-url": {}, "tts-format": {}, "tts-voice": {}, "story-root": {}}) {
		t.Fatal("tts config flags should still launch menu")
	}
	if shouldRunLauncher(false, map[string]struct{}{"tts": {}, "story-id": {}}) {
		t.Fatal("story-id is an explicit flow flag")
	}
	if shouldRunLauncher(false, map[string]struct{}{"packet": {}}) {
		t.Fatal("packet is an explicit flow flag")
	}
	if shouldRunLauncher(false, map[string]struct{}{"admin": {}}) {
		t.Fatal("admin is an explicit flow flag")
	}
}

func TestStaticStorySeedWorkspaceResume(t *testing.T) {
	repoRoot := repoRootForTest(t)
	storyRoot := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/cli",
		"-story-id", "test-id",
		"-story-root", storyRoot,
		"-replace-story",
		"-model-backend", "static",
		"-no-writer-refresh",
		"-no-commit",
		"-play-trace-jsonl",
	)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader("1\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("static seed play failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"event":"player_decision"`) {
		t.Fatalf("missing decision trace:\n%s", output)
	}
	storyDir := filepath.Join(storyRoot, "test-id")
	for _, rel := range []string{
		"state.json",
		"bible.json",
		"turns.jsonl",
		filepath.Join("index", "documents.jsonl"),
		filepath.Join("index", "manifest.json"),
	} {
		if _, err := os.Stat(filepath.Join(storyDir, rel)); err != nil {
			t.Fatalf("missing story file %s: %v\n%s", rel, err, output)
		}
	}

	resume := exec.CommandContext(ctx, "go", "run", "./cmd/cli",
		"-story-id", "test-id",
		"-story-root", storyRoot,
		"-model-backend", "static",
		"-no-writer-refresh",
		"-no-commit",
		"-turns", "1",
	)
	resume.Dir = repoRoot
	resume.Env = os.Environ()
	resumeOutput, err := resume.CombinedOutput()
	if err != nil {
		t.Fatalf("static resume failed: %v\n%s", err, resumeOutput)
	}
	if !strings.Contains(string(resumeOutput), "--- Turn 1 ---") {
		t.Fatalf("resume did not play a turn:\n%s", resumeOutput)
	}
}

func TestRunPlayHardFailsWriterRefreshAndBlocksStory(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("hard-fail")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "hard-fail",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bible := bundle.Bible
	originalStdin := pipeStdin(t, "1\n")
	defer func() { os.Stdin = originalStdin }()

	err = runPlay(context.Background(), playConfig{
		Packet:           bundle.Packet,
		NodeID:           bundle.Packet.EntryNodeID,
		Backend:          "static",
		Workspace:        &workspace,
		State:            &state,
		Bible:            &bible,
		RefreshWrite:     true,
		Commit:           false,
		WriterScenarists: 0,
		WriterPlanner:    invalidRefreshPlanner{},
	})
	if err == nil {
		t.Fatal("expected writer refresh failure")
	}
	if !strings.Contains(err.Error(), "missing target node inside bounded packet") {
		t.Fatalf("expected bounded packet missing target error: %v", err)
	}
	if !strings.Contains(err.Error(), "artifacts:") {
		t.Fatalf("expected artifact path in error: %v", err)
	}
	_, loaded, err := story.LoadWorkspace(root, "hard-fail")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != story.StatusBlockedWriterRefresh {
		t.Fatalf("status = %q", loaded.Status)
	}
	if loaded.PendingDecision.ChoiceID != "open" {
		t.Fatalf("pending decision = %#v", loaded.PendingDecision)
	}
	if loaded.BlockedWriterRunPath == "" {
		t.Fatalf("missing blocked run path: %#v", loaded)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("turn count = %d", len(turns))
	}
	if turns[0].NextDecision.ChoiceID != "open" {
		t.Fatalf("turn next decision = %#v", turns[0].NextDecision)
	}
	events, err := gamelog.Read(gamelog.NewWriter(workspace.Dir, state.StoryID).Path, gamelog.Filter{Event: "writer.refresh.failed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("writer failure events = %#v", events)
	}
	if events[0].DetailPath == "" {
		t.Fatalf("writer failure log missing detail: %#v", events[0])
	}
	detail, err := gamelog.NewWriter(workspace.Dir, state.StoryID).ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	detailText := string(mustJSON(t, detail))
	for _, want := range []string{`"failure_kind":"missing_choice_target"`, `"choice_id":"dangling"`, `"missing_target_node_id":"missing"`, `"known_node_ids":["start"]`} {
		if !strings.Contains(detailText, want) {
			t.Fatalf("writer failure detail missing %s:\n%s", want, detailText)
		}
	}
	for _, rel := range []string{
		filepath.Join("model-runs", "writer", "turn-000001", "input.txt"),
		filepath.Join("model-runs", "writer", "turn-000001", "raw-output.json"),
		filepath.Join("model-runs", "writer", "turn-000001", "validation-error.txt"),
	} {
		if _, err := os.Stat(filepath.Join(workspace.Dir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
}

func TestLiveSeedPlayNarratorChoiceE2E(t *testing.T) {
	if os.Getenv("SANS_NOM_E2E_LIVE") != "1" {
		t.Skip("set SANS_NOM_E2E_LIVE=1 to run live Codex OAuth e2e")
	}
	repoRoot := repoRootForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/cli",
		"-story-id", "live-e2e",
		"-story-root", t.TempDir(),
		"-replace-story",
		"-model-backend", "codex",
		"-no-writer-refresh",
		"-no-commit",
		"-play-trace-jsonl",
	)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader("1\n")

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("live e2e timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("live e2e failed: %v\n%s", err, output)
	}
	text := string(output)
	if strings.Contains(text, "\nCommitted:") {
		t.Fatalf("play should not commit when -no-commit is set:\n%s", text)
	}

	turns, decisions := parseTraceEvents(t, text)
	if len(turns) < 2 {
		t.Fatalf("expected at least two turn events, got %d:\n%s", len(turns), text)
	}
	if len(decisions) == 0 {
		t.Fatalf("expected a player_decision event:\n%s", text)
	}
	if turns[0].ProseChars <= 0 {
		t.Fatalf("turn 1 had no narrator prose: %#v", turns[0])
	}
	if turns[0].ChoiceCount <= 0 {
		t.Fatalf("turn 1 had no choices: %#v", turns[0])
	}
	decision := decisions[0]
	if decision.RawInput != "1" {
		t.Fatalf("expected raw input 1, got %#v", decision)
	}
	if decision.ChoiceID == "" || decision.TargetNodeID == "" {
		t.Fatalf("expected resolved choice and target: %#v", decision)
	}
	if turns[1].NodeID != decision.TargetNodeID {
		t.Fatalf("turn 2 did not follow selected target: got %q want %q\n%s", turns[1].NodeID, decision.TargetNodeID, text)
	}
	if turns[1].Decision.ChoiceID != decision.ChoiceID {
		t.Fatalf("turn 2 commit did not carry choice id: got %#v want %q", turns[1].Decision, decision.ChoiceID)
	}
}

type seedContextRetriever struct {
	hits  []lore.Snippet
	query string
	k     int
}

func (r *seedContextRetriever) Search(_ context.Context, query string, k int) ([]lore.Snippet, error) {
	r.query = query
	r.k = k
	return r.hits, nil
}

type traceTurn struct {
	Event            string               `json:"event"`
	Turn             int                  `json:"turn"`
	PacketID         string               `json:"packet_id"`
	NodeID           string               `json:"node_id"`
	Decision         scene.PlayerDecision `json:"decision"`
	ChoiceCount      int                  `json:"choice_count"`
	ProseChars       int                  `json:"prose_chars"`
	CommitEventCount int                  `json:"commit_event_count"`
	Witnesses        []string             `json:"witnesses"`
}

type traceDecision struct {
	Event        string `json:"event"`
	RawInput     string `json:"raw_input"`
	ChoiceID     string `json:"choice_id"`
	TargetNodeID string `json:"target_node_id"`
	FreeText     string `json:"free_text"`
}

func parseTraceEvents(t *testing.T, output string) ([]traceTurn, []traceDecision) {
	t.Helper()
	var turns []traceTurn
	var decisions []traceDecision
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") || !strings.Contains(line, `"event"`) {
			continue
		}
		var head struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &head); err != nil {
			continue
		}
		switch head.Event {
		case "turn":
			var event traceTurn
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("bad turn trace %q: %v", line, err)
			}
			turns = append(turns, event)
		case "player_decision":
			var event traceDecision
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("bad decision trace %q: %v", line, err)
			}
			decisions = append(decisions, event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return turns, decisions
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root from %s: %v", wd, err)
	}
	return root
}

func pipeStdin(t *testing.T, input string) *os.File {
	t.Helper()
	original := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	t.Cleanup(func() {
		_ = reader.Close()
		os.Stdin = original
	})
	return original
}

type invalidRefreshPlanner struct{}

func (invalidRefreshPlanner) Plan(context.Context, writer.Input) (scene.StoryPacket, error) {
	return scene.StoryPacket{
		ID:          "bad-writer-packet",
		EntryNodeID: "start",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Title: "Bad start",
			Beats: []scene.Beat{{Text: "A bad beat."}},
			Choices: []scene.Choice{{
				ID:           "dangling",
				Label:        "Dangling.",
				TargetNodeID: "missing",
			}},
		}},
	}, nil
}

func cliTestBundle(id string) story.SeedBundle {
	return story.SeedBundle{
		Bible: story.StoryBible{
			StoryID: id,
			Title:   "The Test Weight",
			Premise: "A keeper must decide whether public memory can protect the dead without owning the living.",
			PlayerCharacter: story.PlayerCharacter{
				PeerID:      "sans-nom",
				Name:        "Rin",
				Description: "A keeper with a civic graft.",
				Desire:      "Rin wants to protect a record.",
				Wound:       "Rin distrusts legal mercy.",
				Boundary:    "Rin will not become an instrument.",
			},
			PlayerWorldConnection: "Rin's graft makes the city recognize them.",
			WorldBasics: []string{
				"The city weighs public memory.",
				"Debt law is performed by machines and crowds.",
				"Witnesses change what buildings permit.",
			},
			OriginalLaws: []story.OriginalLaw{{
				ID:             "first-weight",
				Name:           "Law of First Weight",
				Description:    "A vow's first burden must stay with the vow-maker.",
				SourcePattern:  "binding taboo structure",
				Enforcement:    "Public ledgers refuse dishonest transfer.",
				WhyProfound:    "It protects the weak from inherited erasure.",
				NotMerelyCruel: "It limits debt instead of making it endless.",
			}},
			Factions: []story.Faction{{
				ID:          "ledger-choir",
				Name:        "Ledger Choir",
				Description: "Clerks who sing records into public force.",
			}},
			CoreRelationships:          []string{"Rin is bound to the witness hall."},
			ToneDoctrine:               []string{"Body law should feel sacred before tactical."},
			SourceLoreInfluenceRefs:    []string{"source-law structure only"},
			ForbiddenLiteralCanonTerms: story.ForbiddenCanonTerms(),
			FirstTurnExposition:        []string{"world", "player", "connection", "law", "pressure"},
			ResumeSummary:              "Rin stands before the witness hall.",
			Metadata:                   map[string]string{"test": "true"},
		},
		Packet: scene.StoryPacket{
			ID:          "packet-" + id,
			EntryNodeID: "entry",
			ContextSnapshot: scene.ContextSnapshot{
				WorkspaceID: id,
				SessionID:   "main",
				Location:    "Witness Hall",
				Time:        "opening bell",
			},
			Nodes: []scene.SceneNode{{
				ID:    "entry",
				Title: "The hall asks",
				Beats: []scene.Beat{
					{Text: "The witness hall wakes."},
					{Text: "Rin feels the graft answer."},
					{Text: "The first weight is named."},
					{Text: "A record will vanish if Rin waits."},
				},
				Choices: []scene.Choice{{
					ID:           "open",
					Label:        "Open the ledger.",
					TargetNodeID: "opened",
				}, {
					ID:           "refuse",
					Label:        "Refuse the ledger.",
					TargetNodeID: "refused",
				}},
			}, {
				ID:      "opened",
				Title:   "Opened",
				Beats:   []scene.Beat{{Text: "The ledger opens."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}, {
				ID:      "refused",
				Title:   "Refused",
				Beats:   []scene.Beat{{Text: "The ledger waits."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}},
		},
	}
}
