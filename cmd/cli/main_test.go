package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
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
	if got := resolveWriterScenaristCallCount(-1, "codex", true); got != 24 {
		t.Fatalf("default live refresh scenarists = %d", got)
	}
	if got := resolveWriterScenaristCallCount(-1, "static", true); got != 0 {
		t.Fatalf("static scenarists = %d", got)
	}
	if got := resolveWriterScenaristCallCount(99, "codex", true); got != 24 {
		t.Fatalf("clamped scenarists = %d", got)
	}
	if got := resolveWriterScenaristCallCount(0, "codex", true); got != 0 {
		t.Fatalf("explicit zero scenarists = %d", got)
	}
}

func TestEnvBoolDefaultAllowsOptOut(t *testing.T) {
	t.Setenv("SANS_NOM_SCENARIST_TOOLS", "")
	if !envBoolDefault("SANS_NOM_SCENARIST_TOOLS", true) {
		t.Fatal("empty env should use true fallback")
	}
	t.Setenv("SANS_NOM_SCENARIST_TOOLS", "false")
	if envBoolDefault("SANS_NOM_SCENARIST_TOOLS", true) {
		t.Fatal("false env should disable default-on tools")
	}
	t.Setenv("SANS_NOM_SCENARIST_TOOLS", "true")
	if !envBoolDefault("SANS_NOM_SCENARIST_TOOLS", false) {
		t.Fatal("true env should enable tools")
	}
}

func TestModelTimeoutDefaultsAndOverrides(t *testing.T) {
	t.Setenv("SANS_NOM_MODEL_TIMEOUT_MS", "")
	t.Setenv("SANS_NOM_CODEX_TIMEOUT_MS", "")
	t.Setenv("SANS_NOM_CLAUDE_TIMEOUT_MS", "")
	t.Setenv("SANS_NOM_SCENARIST_TIMEOUT_MS", "")
	t.Setenv("SANS_NOM_WRITER_AWAIT_TIMEOUT_MS", "")
	if got := sharedModelTimeout(); got != 45*time.Minute {
		t.Fatalf("shared timeout = %s", got)
	}

	t.Setenv("SANS_NOM_MODEL_TIMEOUT_MS", "1234")
	if got := codexRunner("Test", nil).Timeout; got != 1234*time.Millisecond {
		t.Fatalf("codex shared timeout = %s", got)
	}
	if got := claudeRunner().Timeout; got != 1234*time.Millisecond {
		t.Fatalf("claude shared timeout = %s", got)
	}
	if got := scenaristModelTimeout(); got != 1234*time.Millisecond {
		t.Fatalf("scenarist shared timeout = %s", got)
	}
	if got := writerAwaitTimeout(); got != 1234*time.Millisecond {
		t.Fatalf("writer await shared timeout = %s", got)
	}

	t.Setenv("SANS_NOM_CODEX_TIMEOUT_MS", "2345")
	t.Setenv("SANS_NOM_CLAUDE_TIMEOUT_MS", "3456")
	t.Setenv("SANS_NOM_SCENARIST_TIMEOUT_MS", "4567")
	t.Setenv("SANS_NOM_WRITER_AWAIT_TIMEOUT_MS", "5678")
	if got := codexRunner("Test", nil).Timeout; got != 2345*time.Millisecond {
		t.Fatalf("codex specific timeout = %s", got)
	}
	if got := claudeRunner().Timeout; got != 3456*time.Millisecond {
		t.Fatalf("claude specific timeout = %s", got)
	}
	if got := scenaristModelTimeout(); got != 4567*time.Millisecond {
		t.Fatalf("scenarist specific timeout = %s", got)
	}
	if got := writerAwaitTimeout(); got != 5678*time.Millisecond {
		t.Fatalf("writer await specific timeout = %s", got)
	}

	t.Setenv("SANS_NOM_CODEX_TIMEOUT_MS", "not-a-number")
	if got := codexRunner("Test", nil).Timeout; got != 1234*time.Millisecond {
		t.Fatalf("invalid codex timeout should fall back to shared timeout, got %s", got)
	}
}

func TestShouldUsePlayTUIDisabledForScriptedInputs(t *testing.T) {
	cases := []struct {
		name      string
		turns     int
		choiceID  string
		freeText  string
		traceJSON bool
	}{
		{name: "turn limit", turns: 1},
		{name: "choice", choiceID: "choice_a"},
		{name: "text", freeText: "go north"},
		{name: "trace", traceJSON: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if shouldUsePlayTUI(tt.turns, tt.choiceID, tt.freeText, tt.traceJSON) {
				t.Fatal("scripted/debug input should stay on raw play path")
			}
		})
	}
}

func TestSeedWriterRunnerExcludesScenaristToolAfterWorkshop(t *testing.T) {
	t.Setenv("ZAI__ANTHROPIC_BASE_URL", "https://example.invalid")
	t.Setenv("ZAI__ANTHROPIC_AUTH_TOKEN", "token")
	t.Setenv("ZAI__ANTHROPIC_MODEL", "model")
	cfg := seedRuntimeConfig{
		Backend:              "codex",
		WriterScenaristCalls: 1,
		ScenaristProvider:    "ZAI",
		ScenaristRounds:      7,
	}
	outputDir := filepath.Join(t.TempDir(), "agent", "scenarists")
	scope := seedAgentScopeFromScenaristOutputDir(outputDir)
	loreTool := &loreexpert.ToolRunner{PackID: "metabarons"}
	toolRunner, err := seedScenaristToolRunner(cfg, "writer context", outputDir, scope, nil, loreTool)
	if err != nil {
		t.Fatal(err)
	}
	if toolRunner.LoreExpert != loreTool {
		t.Fatal("seed scenarist tool should receive lore expert access")
	}
	if toolRunner.MaxCalls != 1 {
		t.Fatalf("seed scenarist tool max calls = %d", toolRunner.MaxCalls)
	}
	logger := gamelog.NewGlobalWriter(t.TempDir())
	runner, err := seedWriterRunner(cfg, scope, toolRunner, loreTool, logger)
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
	if codex.ToolRoot != scope.Root {
		t.Fatalf("seed writer tool root = %q, want %q", codex.ToolRoot, scope.Root)
	}
	if codex.Logger != logger || codex.ToolCategory != "writer" || codex.CallerRole != "creative-writer" {
		t.Fatalf("seed writer runner missing tool logging metadata: %#v", codex)
	}
	if hasTool(codex.ExtraTools, "ask_scenarist") {
		t.Fatalf("final seed writer should not get scenarist tool: %#v", codex.ExtraTools)
	}
	if !hasTool(codex.ExtraTools, "ask_lore_expert") {
		t.Fatalf("seed writer missing lore expert tool: %#v", codex.ExtraTools)
	}
	workshop, err := seedWorkshopRunner(cfg, scope, toolRunner, loreTool, logger)
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
	if !hasTool(workshopCodex.ExtraTools, "ask_scenarist") || !hasTool(workshopCodex.ExtraTools, "ask_lore_expert") {
		t.Fatalf("workshop writer missing local tools: %#v", workshopCodex.ExtraTools)
	}
	if workshopCodex.Logger != logger || workshopCodex.ToolCategory != "writer" || workshopCodex.CallerRole != "creative-writer" {
		t.Fatalf("workshop writer missing tool logging metadata: %#v", workshopCodex)
	}
}

func TestCodexMaterialRunnerInjectsStagedMasterLoreIntoSystemPrompt(t *testing.T) {
	scope := agentfs.New(filepath.Join(t.TempDir(), "agent"), "writer").WithVisiblePaths("lorepack")
	if _, err := scope.StageText("lorepack/LORE.md", "# Master Lore\n\nDynasty law lives here.\n"); err != nil {
		t.Fatal(err)
	}

	runner := codexRunnerWithMaterialAccess("StoryPacket", prompt.StoryPacketSchema(), scope, nil, nil, 0)

	for _, required := range []string{
		"ACTIVE LOREPACK MASTER SUMMARY",
		"Dynasty law lives here.",
	} {
		if !strings.Contains(runner.SystemPrompt, required) {
			t.Fatalf("runner system prompt missing %q:\n%s", required, runner.SystemPrompt)
		}
	}
}

func TestSeedVerifierRunnerUsesVerifierCodexEnv(t *testing.T) {
	t.Setenv("SANS_NOM_CODEX_MODEL", "writer-model")
	t.Setenv("SANS_NOM_CODEX_EFFORT", "low")
	t.Setenv("SANS_NOM_VERIFIER_CODEX_MODEL", "gpt-5.5")
	t.Setenv("SANS_NOM_VERIFIER_CODEX_EFFORT", "xhigh")

	runner := seedRunner("codex", false)
	codex, ok := runner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", runner)
	}
	if codex.SchemaName != "SeedReview" {
		t.Fatalf("schema = %q", codex.SchemaName)
	}
	if codex.Model != "gpt-5.5" || codex.Effort != "xhigh" {
		t.Fatalf("verifier model/effort = %q/%q", codex.Model, codex.Effort)
	}

	writerRunner := seedRunner("codex", true)
	writerCodex, ok := writerRunner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", writerRunner)
	}
	if writerCodex.Model != "writer-model" || writerCodex.Effort != "low" {
		t.Fatalf("writer seed runner should keep global model/effort, got %q/%q", writerCodex.Model, writerCodex.Effort)
	}
}

func TestSeedVerifierRunnerDefaultsToXHighGPT55(t *testing.T) {
	t.Setenv("SANS_NOM_CODEX_MODEL", "writer-model")
	t.Setenv("SANS_NOM_CODEX_EFFORT", "low")

	runner := seedRunner("codex", false)
	codex, ok := runner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", runner)
	}
	if codex.Model != "gpt-5.5" || codex.Effort != "xhigh" {
		t.Fatalf("verifier default model/effort = %q/%q", codex.Model, codex.Effort)
	}
}

func TestRefreshWriterPlannerGetsScenaristTool(t *testing.T) {
	logger := gamelog.NewWriter(t.TempDir(), "story-test")
	scope := agentfs.New(t.TempDir(), "writer").WithVisiblePaths(".")
	planner := writerPlannerWithMaterialAccess("codex", prompt.ClaudeRunner{}, scope, &scenarist.ToolRunner{MaxCalls: 1}, &loreexpert.ToolRunner{PackID: "metabarons"}, 5, logger, "run-1")
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
	if codex.ToolRoot != scope.Root {
		t.Fatalf("refresh writer tool root = %q, want %q", codex.ToolRoot, scope.Root)
	}
	if !hasTool(codex.ExtraTools, "ask_scenarist") || !hasTool(codex.ExtraTools, "ask_lore_expert") {
		t.Fatalf("refresh writer missing local tools: %#v", codex.ExtraTools)
	}
	if codex.Logger != logger || codex.ToolCategory != "writer" || codex.CallerRole != "writer" || codex.RunID != "run-1" {
		t.Fatalf("refresh writer missing tool logging metadata: %#v", codex)
	}
}

func TestScenaristProviderRunnersUseArtifactDirsNotLoreDir(t *testing.T) {
	t.Setenv("ZAI__ANTHROPIC_BASE_URL", "https://example.invalid")
	t.Setenv("ZAI__ANTHROPIC_AUTH_TOKEN", "token")
	t.Setenv("ZAI__ANTHROPIC_MODEL", "model")
	workspaceRoot := filepath.Join(t.TempDir(), "data", "seed-runs", "seed-1", "agent")
	outputDir := filepath.Join(workspaceRoot, "scenarists")
	runners := scenaristProviderRunners(playConfig{
		ScenaristProvider: "ZAI",
		ScenaristTools:    true,
	}, outputDir, workspaceRoot)
	if len(runners) != 1 {
		t.Fatalf("runners = %#v", runners)
	}
	if runners[0].WorkDir != workspaceRoot {
		t.Fatalf("workdir = %q, want %q", runners[0].WorkDir, workspaceRoot)
	}
	for _, dir := range runners[0].AddDirs {
		if dir == "lore" || strings.Contains(filepath.ToSlash(dir), "/lore") {
			t.Fatalf("scenarist runner should not expose lore dir: %#v", runners[0].AddDirs)
		}
	}
	if !containsStringWith(runners[0].AddDirs, filepath.ToSlash(workspaceRoot)) {
		t.Fatalf("scenarist runner missing workspace root: %#v", runners[0].AddDirs)
	}
}

func TestStageLorePackForAgentCopiesActivePackIntoWorkspace(t *testing.T) {
	source := filepath.Join(t.TempDir(), "pack")
	for _, dir := range []string{
		filepath.Join(source, "wiki"),
		filepath.Join(source, "links"),
		filepath.Join(source, "comics", "volumes"),
		filepath.Join(source, "comics", "characters"),
		filepath.Join(source, "comics", "pages"),
		filepath.Join(source, "comics", "peer-cards", "cards", "accepted"),
		filepath.Join(source, "comics", "peer-cards", "runs", "batch"),
		filepath.Join(source, "comics", "peer-cards", "provenance", "inputs"),
		filepath.Join(source, "index"),
		filepath.Join(source, "testdata"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "lore-pack.json"), []byte(`{"id":"pack","artifact_roots":{"comics":"comics","wiki":"wiki/lore.md","links":"links/matrix.md","summary":"LORE.md","index":"index"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "wiki", "lore.md"), []byte("lore"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "links", "matrix.md"), []byte("links"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "LORE.md"), []byte("# Master Lore\n\nFull pack summary."), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		filepath.Join("comics", "manifest.json"):                                    `{}`,
		filepath.Join("comics", "volumes", "volume-01.json"):                        `{}`,
		filepath.Join("comics", "characters", "characters-volume-01.json"):          `{}`,
		filepath.Join("comics", "pages", "001_page.json"):                           `{}`,
		filepath.Join("comics", "peer-cards", "cards", "accepted", "tonto.json"):    `{}`,
		filepath.Join("comics", "peer-cards", "runs", "batch", "old-run.jsonl"):     `fixture leak`,
		filepath.Join("comics", "peer-cards", "provenance", "inputs", "tonto.json"): `fixture leak`,
		filepath.Join("index", "manifest.json"):                                     `{}`,
		filepath.Join("index", "documents.jsonl"):                                   `fixture leak`,
		filepath.Join("testdata", "fixture.json"):                                   `fixture leak`,
	} {
		if err := os.WriteFile(filepath.Join(source, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scope := agentfs.New(filepath.Join(t.TempDir(), "agent"), "writer").WithVisiblePaths("story")
	stalePath := filepath.Join(scope.Root, "lorepack", "comics", "peer-cards", "runs", "batch", "stale.jsonl")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("stale leak"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageLorePackForAgent(scope, lorepack.Pack{ID: "pack", Path: filepath.Join(source, "lore-pack.json")})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"lorepack/lore-pack.json",
		"lorepack/wiki/lore.md",
		"lorepack/links/matrix.md",
		"lorepack/LORE.md",
		"lorepack/comics/manifest.json",
		"lorepack/comics/volumes/volume-01.json",
		"lorepack/comics/characters/characters-volume-01.json",
		"lorepack/comics/pages/001_page.json",
		"lorepack/comics/peer-cards/cards/accepted/tonto.json",
		"lorepack/index/manifest.json",
		"lorepack/navigation.md",
	} {
		abs, err := staged.Abs(rel)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("missing staged %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"lorepack/comics/peer-cards/runs/batch/old-run.jsonl",
		"lorepack/comics/peer-cards/runs/batch/stale.jsonl",
		"lorepack/comics/peer-cards/provenance/inputs/tonto.json",
		"lorepack/index/documents.jsonl",
		"lorepack/testdata/fixture.json",
	} {
		abs := filepath.Join(staged.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staged forbidden artifact %s, stat err = %v", rel, err)
		}
	}
	if !containsStringWith(staged.VisiblePaths, "lorepack") {
		t.Fatalf("visible paths missing lorepack: %#v", staged.VisiblePaths)
	}
	navPath, err := staged.Abs("lorepack/navigation.md")
	if err != nil {
		t.Fatal(err)
	}
	navBytes, err := os.ReadFile(navPath)
	if err != nil {
		t.Fatal(err)
	}
	nav := string(navBytes)
	for _, required := range []string{
		"Minimum Orientation Before Writing",
		"lorepack/comics/volumes/volume-XX.json",
		"lorepack/comics/characters/characters-volume-XX.json",
		"lorepack/comics/pages/<page_id>.json",
		"Use the Grep tool or rg",
		"Do not draft after reading only lorepack/lore-pack.json",
	} {
		if !strings.Contains(nav, required) {
			t.Fatalf("navigation missing %q:\n%s", required, nav)
		}
	}
}

func TestPrepareStoryAgentScopeStagesLorepackFromState(t *testing.T) {
	bundle := cliTestBundle("agent-lorepack")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID:          "agent-lorepack",
		Root:             t.TempDir(),
		GroundLorePackID: "metabarons",
		Replace:          true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := prepareStoryAgentScope(playConfig{
		Workspace: &workspace,
		State:     &state,
		Bible:     &bundle.Bible,
	}, "narrator", "turn-000001", bundle.Packet, bundle.Packet.EntryNodeID, "context")
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"lorepack/lore-pack.json", "lorepack/LORE.md", "lorepack/contract.md", "lorepack/relevant-excerpts.md", "lorepack/source-anchors.md", "lorepack/navigation.md"} {
		abs, err := scope.Abs(rel)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("missing staged %s: %v", rel, err)
		}
	}
	wiki, err := scope.Abs("lorepack/wiki/lore.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wiki); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("narrator scope should not stage broad wiki lore, stat err = %v", err)
	}
	if !containsStringWith(scope.VisiblePaths, "lorepack") {
		t.Fatalf("visible paths missing lorepack: %#v", scope.VisiblePaths)
	}
	navPath, err := scope.Abs("lorepack/navigation.md")
	if err != nil {
		t.Fatal(err)
	}
	navBytes, err := os.ReadFile(navPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(navBytes), "Narrator Lorepack Navigation") {
		t.Fatalf("narrator navigation missing title:\n%s", string(navBytes))
	}
}

func TestLoreExpertModelRunnerDefaultsToCodexMiniMedium(t *testing.T) {
	t.Setenv("SANS_NOM_LORE_EXPERT_MODEL", "")
	t.Setenv("SANS_NOM_LORE_EXPERT_EFFORT", "")
	runner := loreExpertModelRunner()
	if runner.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("model = %q", runner.Model)
	}
	if runner.Effort != "medium" {
		t.Fatalf("effort = %q", runner.Effort)
	}
}

func TestLoreExpertModelRunnerMapsMinimalEffortToLow(t *testing.T) {
	t.Setenv("SANS_NOM_LORE_EXPERT_MODEL", "")
	t.Setenv("SANS_NOM_LORE_EXPERT_EFFORT", "minimal")
	runner := loreExpertModelRunner()
	if runner.Effort != "low" {
		t.Fatalf("effort = %q", runner.Effort)
	}
}

func TestLoreExpertToolRunnerInjectsMasterLoreAndHashesIt(t *testing.T) {
	tool := loreExpertToolRunner(playConfig{
		Backend:          "codex",
		GroundLorePackID: "metabarons",
		BridgeURL:        "http://127.0.0.1:8787",
	})
	if tool == nil {
		t.Fatal("expected lore expert tool")
	}
	runner, ok := tool.Runner.(prompt.CodexRunner)
	if !ok {
		t.Fatalf("expected CodexRunner, got %T", tool.Runner)
	}
	if tool.MasterLorePath == "" || !strings.HasSuffix(filepath.ToSlash(tool.MasterLorePath), "lore/packs/metabarons/LORE.md") {
		t.Fatalf("unexpected master lore path: %q", tool.MasterLorePath)
	}
	if !strings.Contains(runner.SystemPrompt, "ACTIVE LOREPACK MASTER SUMMARY") || !strings.Contains(runner.SystemPrompt, "The Saga of the Metabarons") {
		t.Fatalf("lore expert system prompt missing master lore:\n%s", runner.SystemPrompt)
	}
}

func hasTool(tools []codexoauth.LocalTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func containsStringWith(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
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
	audio := []byte{0x01, 0x02, 0x03, 0x04}
	longText := strings.TrimSpace(strings.Repeat("Read this full narrator turn. ", 1000))
	var gotRequests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret" {
			t.Fatalf("missing api-key header")
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var gotRequest map[string]any
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatal(err)
		}
		gotRequests = append(gotRequests, gotRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"audio": map[string]any{
						"data": base64.StdEncoding.EncodeToString(audio),
					},
				},
			}},
		})
	}))
	defer server.Close()
	logger := gamelog.NewGlobalWriter(t.TempDir())
	outDir := t.TempDir()
	path, err := synthesizeTurnAudio(context.Background(), playConfig{
		Logger: logger,
		TTS: playTTSConfig{
			Enabled:     true,
			BaseURL:     server.URL + "/v1",
			APIKey:      "secret",
			Model:       "mimo-v2.5-tts",
			Voice:       "Milo",
			Format:      "wav",
			Instruction: "Sci-fi story narrator voice.",
			OutputDir:   outDir,
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
	if len(gotRequests) < 2 {
		t.Fatalf("expected chunked tts requests, got %d", len(gotRequests))
	}
	var submitted []string
	for _, gotRequest := range gotRequests {
		messages, _ := gotRequest["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("messages = %#v", gotRequest["messages"])
		}
		assistant, _ := messages[1].(map[string]any)
		submitted = append(submitted, assistant["content"].(string))
		audioReq, _ := gotRequest["audio"].(map[string]any)
		if audioReq["format"] != "pcm16" || audioReq["voice"] != "Milo" {
			t.Fatalf("bad audio request: %#v", audioReq)
		}
	}
	if got := strings.Join(submitted, " "); got != longText {
		t.Fatalf("assistant text was not submitted intact: got len %d want %d", len(got), len(longText))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 44 || !bytes.HasPrefix(data, []byte("RIFF")) || !bytes.Contains(data[:44], []byte("WAVE")) {
		t.Fatalf("expected synthesized wav container, got %q", data)
	}
	for _, rel := range []string{
		filepath.Join("model-runs", "tts", "turn-000001", "request.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "chunk-001", "request.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "chunk-001", "response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "metadata.json"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "tts.mimo.completed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("missing tts completed detail: %#v", events)
	}
}

func TestSynthesizeTurnAudioFailurePreservesDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "bad text"},
		})
	}))
	defer server.Close()
	logger := gamelog.NewGlobalWriter(t.TempDir())
	outDir := t.TempDir()
	_, err := synthesizeTurnAudio(context.Background(), playConfig{
		Logger: logger,
		TTS: playTTSConfig{
			Enabled:   true,
			BaseURL:   server.URL + "/v1",
			APIKey:    "secret",
			Model:     "mimo-v2.5-tts",
			Voice:     "Milo",
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
		filepath.Join("model-runs", "tts", "turn-000001", "response.json"),
		filepath.Join("model-runs", "tts", "turn-000001", "error.txt"),
	} {
		if _, statErr := os.Stat(filepath.Join(outDir, rel)); statErr != nil {
			t.Fatalf("missing artifact %s: %v", rel, statErr)
		}
	}
	events, readErr := gamelog.Read(logger.Path, gamelog.Filter{Event: "tts.mimo.failed"})
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("missing failed event detail: %#v", events)
	}
}

func TestTTSOutputPathUsesConfiguredFormat(t *testing.T) {
	got := ttsOutputPath(playConfig{TTS: playTTSConfig{OutputDir: "audio-out", Format: "pcm16"}}, nil, 2)
	if filepath.Base(got) != "turn-000002.pcm16" {
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

func TestBuildSeedContextUsesScenarioContractNotRawLore(t *testing.T) {
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
	if strings.Contains(text, "A binding law works") {
		t.Fatalf("seed context should not inject raw lore snippets:\n%s", text)
	}
	if !strings.Contains(text, "Lorepack Scenario Orientation") || !strings.Contains(text, "ask_lore_expert") {
		t.Fatalf("seed context missing scenario/tool guidance:\n%s", text)
	}
	for _, required := range []string{
		"contract_path: lorepack/lore-pack.json",
		"master_lore_path: lorepack/LORE.md",
		"pressure field for faithful play",
		"premise logic",
		"protagonist pressure",
		"Before drafting, orient in the staged lorepack",
		"lorepack/navigation.md",
		"minimum orientation",
		"relevant volume summaries",
		"matching character records",
		"do not rely on shell grep",
		"major characters, family/story pressures, repeated laws, decisive events, and choice logic",
		"choice_principles",
		"copying its synopsis wording",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("seed context missing %q:\n%s", required, text)
		}
	}
	for _, tooSpecific := range []string{
		"young heir inside a brutal warrior dynasty",
		"demanded succession rite",
		"rite meant to prove fitness",
		"must_honor:",
		"must_not_violate:",
	} {
		if strings.Contains(text, tooSpecific) {
			t.Fatalf("seed context should not inline full scenario contract term %q:\n%s", tooSpecific, text)
		}
	}
	for _, forbidden := range []string{
		"Ground-lore DNA summary",
		"creative DNA",
		"ask Scenarists first",
		"source lore is craft DNA",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("seed context kept legacy influence wording %q:\n%s", forbidden, text)
		}
	}
	if retriever.query != "" || retriever.k != 0 {
		t.Fatalf("seed context should not call lore retriever, got query=%q k=%d", retriever.query, retriever.k)
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
	if shouldRunLauncher(false, map[string]struct{}{"read-story": {}}) {
		t.Fatal("read-story is an explicit flow flag")
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
	cmd.Stdin = strings.NewReader("1\n1\n")
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

func TestRunPlaySavesDisplayedTurnBeforeInputAndResumesChoicePrompt(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("awaiting-choice")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "awaiting-choice",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bible := bundle.Bible
	originalStdin := pipeStdin(t, "")
	err = runPlay(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               bundle.Packet.EntryNodeID,
		Backend:              "static",
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         false,
		Commit:               false,
		WriterScenaristCalls: 0,
	})
	os.Stdin = originalStdin
	if err != nil {
		t.Fatal(err)
	}
	_, loaded, err := story.LoadWorkspace(root, "awaiting-choice")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != story.StatusAwaitingPlayerChoice {
		t.Fatalf("status = %q", loaded.Status)
	}
	if loaded.PendingTurnPath == "" {
		t.Fatalf("missing pending turn path: %#v", loaded)
	}
	if loaded.LastNarratorProse == "" {
		t.Fatal("last narrator prose was not checkpointed")
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, filepath.FromSlash(loaded.PendingTurnPath))); err != nil {
		t.Fatalf("missing pending turn file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, "narration", "pending-turn.md")); err != nil {
		t.Fatalf("missing pending narration file: %v", err)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns were finalized before player input: %#v", turns)
	}

	packet, err := workspace.LoadPacket(loaded)
	if err != nil {
		t.Fatal(err)
	}
	originalStdin = pipeStdin(t, "1\n")
	err = runPlay(context.Background(), playConfig{
		Packet:               packet,
		NodeID:               loaded.CurrentNodeID,
		Backend:              "static",
		Workspace:            &workspace,
		State:                &loaded,
		Bible:                &bible,
		RefreshWrite:         false,
		Commit:               false,
		WriterScenaristCalls: 0,
	})
	os.Stdin = originalStdin
	if err != nil {
		t.Fatal(err)
	}
	_, resumed, err := story.LoadWorkspace(root, "awaiting-choice")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != story.StatusAwaitingPlayerChoice {
		t.Fatalf("resumed status = %q", resumed.Status)
	}
	if resumed.PendingTurnPath == "" {
		t.Fatalf("next pending turn was not saved: %#v", resumed)
	}
	pending, err := workspace.LoadPendingTurn(resumed)
	if err != nil {
		t.Fatal(err)
	}
	if pending.NodeID != "decision" {
		t.Fatalf("pending node = %q", pending.NodeID)
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, filepath.FromSlash(resumed.PendingTurnPath))); err != nil {
		t.Fatalf("pending turn file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, "narration", "pending-turn.md")); err != nil {
		t.Fatalf("pending narration file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Dir, "narration", "turn-000001.md")); err != nil {
		t.Fatalf("missing finalized narration file: %v", err)
	}
	turns, err = workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) == 0 {
		t.Fatal("expected finalized turns after resumed choice")
	}
	if turns[0].NextNodeID != "decision" {
		t.Fatalf("first resumed choice next node = %q", turns[0].NextNodeID)
	}
}

func TestRunPlayMarksTerminalRunwayNodeAwaitingContinuation(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("awaiting-continuation")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "awaiting-continuation",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bible := bundle.Bible
	originalStdin := pipeStdin(t, "")
	err = runPlay(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               "opened",
		InitialDecision:      scene.PlayerDecision{ChoiceID: "pay"},
		Backend:              "static",
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         true,
		Commit:               false,
		WriterScenaristCalls: 0,
	})
	os.Stdin = originalStdin
	if err != nil {
		t.Fatal(err)
	}
	_, loaded, err := story.LoadWorkspace(root, "awaiting-continuation")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != story.StatusAwaitingContinuation {
		t.Fatalf("status = %q", loaded.Status)
	}
	pending, err := workspace.LoadPendingTurn(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if pending.NodeID != "opened" || len(pending.Choices) != 0 {
		t.Fatalf("pending turn = %#v", pending)
	}

	planner := &countingPlanner{next: scene.StoryPacket{
		ID:          "packet-next",
		EntryNodeID: "next",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID: "awaiting-continuation",
			SessionID:   "main",
		},
		Nodes: []scene.SceneNode{{
			ID:      "next",
			Title:   "Next",
			Beats:   []scene.Beat{{Text: "The next packet begins."}},
			Choices: []scene.Choice{},
		}},
	}}
	resumeStdin := pipeStdin(t, "\n")
	err = runPlay(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               loaded.CurrentNodeID,
		Backend:              "static",
		Workspace:            &workspace,
		State:                &loaded,
		Bible:                &bible,
		RefreshWrite:         true,
		Commit:               false,
		WriterScenaristCalls: 0,
		WriterPlanner:        planner,
	})
	os.Stdin = resumeStdin
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 1 {
		t.Fatalf("writer refresh calls = %d, want 1", planner.calls)
	}
	if planner.lastInput.LatestDecision != "pay" {
		t.Fatalf("writer refresh latest decision = %q, want prior branch choice", planner.lastInput.LatestDecision)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) == 0 || turns[0].NodeID != "opened" || turns[0].Decision.ChoiceID != "pay" || !story.PlayerDecisionEmpty(turns[0].NextDecision) {
		t.Fatalf("terminal turn was not continued correctly: %#v", turns)
	}
}

func TestDecisionForWriterRefreshAfterSubmitFallsBackToDisplayedChoice(t *testing.T) {
	state := &story.StoryState{LastDecision: scene.PlayerDecision{ChoiceID: "older"}}
	got := decisionForWriterRefreshAfterSubmit(
		scene.PlayerDecision{},
		scene.PlayerDecision{ChoiceID: "choice_carry_the_mask"},
		state,
	)
	if got.ChoiceID != "choice_carry_the_mask" || got.FreeText != "" {
		t.Fatalf("fallback decision = %#v", got)
	}

	got = decisionForWriterRefreshAfterSubmit(
		scene.PlayerDecision{FreeText: "force the steward into public witness"},
		scene.PlayerDecision{ChoiceID: "choice_carry_the_mask"},
		state,
	)
	if got.FreeText != "force the steward into public witness" || got.ChoiceID != "" {
		t.Fatalf("typed direction should win, got %#v", got)
	}
}

func TestRunPlayRetriesUncommittedPendingTurnBeforeContinuing(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("retry-pending-commit")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "retry-pending-commit",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	node := bundle.Packet.Nodes[2]
	rollout := narrator.Rollout{
		Node: node,
		Draft: narrator.Draft{
			Prose: "The ledger opens.",
		},
		Commit: scene.Commit{
			WorkspaceID: state.WorkspaceID,
			SessionID:   "main--opened",
			PacketID:    bundle.Packet.ID,
			NodeID:      node.ID,
			PlayerDecision: scene.PlayerDecision{
				ChoiceID: "pay",
			},
			FinalProse: "The ledger opens.",
			Events: []scene.CanonEvent{{
				PeerID:    "narrator",
				Content:   "The ledger opens.",
				Source:    scene.CommitSourceNarrator,
				Witnesses: []string{"sans-nom"},
			}},
			Witnesses: []string{"sans-nom"},
		},
	}
	var commitCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/scene/commit" {
			http.NotFound(w, r)
			return
		}
		commitCalls++
		_, _ = w.Write([]byte(`{"status":"committed"}`))
	}))
	defer server.Close()
	bible := bundle.Bible
	cfg := playConfig{
		Packet:               bundle.Packet,
		NodeID:               node.ID,
		Backend:              "static",
		BridgeURL:            server.URL,
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         false,
		Commit:               true,
		WriterScenaristCalls: 0,
		Logger:               gamelog.NewWriter(workspace.Dir, state.StoryID),
	}
	if err := savePendingDisplayedTurn(cfg, &state, bundle.Packet, rollout, false, "", 1); err != nil {
		t.Fatal(err)
	}
	if err := runPlay(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", commitCalls)
	}
	_, loaded, err := story.LoadWorkspace(root, "retry-pending-commit")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PendingTurnPath != "" || loaded.Status != story.StatusActive {
		t.Fatalf("pending turn was not cleared after commit retry: %#v", loaded)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || !turns[0].Committed {
		t.Fatalf("turn was not recorded as committed: %#v", turns)
	}
}

func TestRefreshStoryIndexReplacesRemoteCollection(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("replace-story-index")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "replace-story-index",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Collection string            `json:"collection"`
		Documents  []json.RawMessage `json:"documents"`
		Replace    bool              `json:"replace"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/story/index/ingest" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collection": captured.Collection,
			"count":      len(captured.Documents),
			"replaced":   captured.Replace,
		})
	}))
	defer server.Close()
	bible := bundle.Bible
	if err := refreshStoryIndex(context.Background(), playConfig{
		Backend:   "codex",
		BridgeURL: server.URL,
		Workspace: &workspace,
		State:     &state,
		Bible:     &bible,
		Logger:    gamelog.NewWriter(workspace.Dir, state.StoryID),
	}, &state, bundle.Packet); err != nil {
		t.Fatal(err)
	}
	if !captured.Replace {
		t.Fatalf("story refresh index should replace stale remote docs: %#v", captured)
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
	originalStdin := pipeStdin(t, "1\n1\nwait\n")
	defer func() { os.Stdin = originalStdin }()

	err = runPlay(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               bundle.Packet.EntryNodeID,
		Backend:              "static",
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         true,
		Commit:               false,
		WriterScenaristCalls: 0,
		WriterPlanner:        invalidRefreshPlanner{},
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
	if loaded.PendingDecision.FreeText != "wait" {
		t.Fatalf("pending decision = %#v", loaded.PendingDecision)
	}
	if loaded.BlockedWriterRunPath == "" {
		t.Fatalf("missing blocked run path: %#v", loaded)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 {
		t.Fatalf("turn count = %d", len(turns))
	}
	if turns[0].NextDecision.ChoiceID != "open" {
		t.Fatalf("turn 1 next decision = %#v", turns[0].NextDecision)
	}
	if turns[1].Decision.ChoiceID != "" || turns[1].NextDecision.ChoiceID != "pay" {
		t.Fatalf("turn 2 decisions = current %#v next %#v", turns[1].Decision, turns[1].NextDecision)
	}
	if turns[2].Decision.ChoiceID != "pay" || turns[2].NextDecision.FreeText != "wait" {
		t.Fatalf("turn 3 decisions = current %#v next %#v", turns[2].Decision, turns[2].NextDecision)
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
		filepath.Join("model-runs", "writer", "turn-000003", "input.txt"),
		filepath.Join("model-runs", "writer", "turn-000003", "raw-output.json"),
		filepath.Join("model-runs", "writer", "turn-000003", "validation-error.txt"),
	} {
		if _, err := os.Stat(filepath.Join(workspace.Dir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
}

func TestRunPlayRetriesBlockedWriterRefresh(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("retry-blocked")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "retry-blocked",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bible := bundle.Bible
	state.Status = story.StatusBlockedWriterRefresh
	state.BlockedReason = "model exceeded 24 local tool rounds"
	state.BlockedWriterRunID = "turn-000003"
	state.BlockedWriterRunPath = "model-runs/writer/turn-000003"
	state.CurrentNodeID = "opened"
	state.LastDecision = scene.PlayerDecision{ChoiceID: "pay"}
	if err := workspace.SaveState(state); err != nil {
		t.Fatal(err)
	}
	nextPacket := cliTestBundle("retry-next").Packet
	planner := &countingPlanner{next: nextPacket}
	cfg, err := retryBlockedWriterRefreshIfNeeded(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               "opened",
		Backend:              "static",
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         true,
		Commit:               false,
		WriterScenaristCalls: 0,
		WriterPlanner:        planner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls = %d", planner.calls)
	}
	if planner.lastInput.LatestDecision != "pay" {
		t.Fatalf("writer latest decision = %q", planner.lastInput.LatestDecision)
	}
	if cfg.Packet.ID != nextPacket.ID || cfg.NodeID != nextPacket.EntryNodeID {
		t.Fatalf("retry config packet/node = %s/%s", cfg.Packet.ID, cfg.NodeID)
	}
	_, loaded, err := story.LoadWorkspace(root, "retry-blocked")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != story.StatusActive {
		t.Fatalf("status = %q", loaded.Status)
	}
	if loaded.BlockedReason != "" || loaded.BlockedWriterRunID != "" || loaded.BlockedWriterRunPath != "" {
		t.Fatalf("blocked fields not cleared: %#v", loaded)
	}
	if loaded.CurrentPacketID != nextPacket.ID || loaded.CurrentNodeID != nextPacket.EntryNodeID {
		t.Fatalf("loaded packet/node = %s/%s", loaded.CurrentPacketID, loaded.CurrentNodeID)
	}
	events, err := gamelog.Read(gamelog.NewWriter(workspace.Dir, state.StoryID).Path, gamelog.Filter{Event: "writer.refresh.completed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Fields["retry"] != true {
		t.Fatalf("retry completion event = %#v", events)
	}
}

func TestTerminalHandoffBridgeUsesRevisionNote(t *testing.T) {
	got := terminalHandoffBridge(scene.SceneNode{
		RevisionNotes: []string{
			"Next packet from this branch should pursue the armory maintenance rookery and the copied service-band phrase. Keep the moral stain active.",
		},
	})
	want := "The scene carries forward: pursue the armory maintenance rookery and the copied service-band phrase."
	if got != want {
		t.Fatalf("bridge = %q, want %q", got, want)
	}
}

func TestRunPlayUsesSeedRunwayBeforeWriterRefresh(t *testing.T) {
	root := t.TempDir()
	bundle := cliTestBundle("runway")
	workspace, state, err := story.InitWorkspace(story.InitOptions{
		StoryID: "runway",
		Root:    root,
		Replace: true,
	}, bundle)
	if err != nil {
		t.Fatal(err)
	}
	bible := bundle.Bible
	planner := &countingPlanner{next: scene.StoryPacket{
		ID:          "packet-next",
		EntryNodeID: "next",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID: "runway",
			SessionID:   "main",
		},
		Nodes: []scene.SceneNode{{
			ID:      "next",
			Title:   "Next",
			Beats:   []scene.Beat{{Text: "The next packet begins."}},
			Choices: []scene.Choice{},
		}},
	}}
	originalStdin := pipeStdin(t, "1\n2\ncontinue\n")
	defer func() { os.Stdin = originalStdin }()

	err = runPlay(context.Background(), playConfig{
		Packet:               bundle.Packet,
		NodeID:               bundle.Packet.EntryNodeID,
		Backend:              "static",
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		RefreshWrite:         true,
		Commit:               false,
		WriterScenaristCalls: 0,
		WriterPlanner:        planner,
		Turns:                4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if planner.calls != 1 {
		t.Fatalf("writer refresh calls = %d, want 1 after runway branch", planner.calls)
	}
	turns, err := workspace.LoadTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 4 {
		t.Fatalf("turn count = %d", len(turns))
	}
	if turns[0].NodeID != "entry" || turns[0].NextNodeID != "decision" || turns[0].NextDecision.ChoiceID != "open" {
		t.Fatalf("turn 1 did not follow cosmetic intro: %#v", turns[0])
	}
	if turns[1].NodeID != "decision" || turns[1].Decision.ChoiceID != "" || turns[1].NextDecision.ChoiceID != "pay" || turns[1].NextNodeID != "opened" {
		t.Fatalf("turn 2 did not consume real decision correctly: %#v", turns[1])
	}
	if turns[2].NodeID != "opened" || turns[2].Decision.ChoiceID != "pay" || turns[2].NextDecision.FreeText != "wait" {
		t.Fatalf("turn 3 did not trigger continuation correctly: %#v", turns[2])
	}
	if turns[3].NodeID != "next" || turns[3].Decision.FreeText != "wait" {
		t.Fatalf("turn 4 did not render refreshed packet: %#v", turns[3])
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
	if len(turns) < 3 {
		t.Fatalf("expected at least three turn events, got %d:\n%s", len(turns), text)
	}
	if len(decisions) < 2 {
		t.Fatalf("expected a player_decision event:\n%s", text)
	}
	if turns[0].ProseChars <= 0 {
		t.Fatalf("turn 1 had no narrator prose: %#v", turns[0])
	}
	if turns[0].ChoiceCount <= 0 {
		t.Fatalf("turn 1 had no choices: %#v", turns[0])
	}
	introDecision := decisions[0]
	if introDecision.RawInput != "1" {
		t.Fatalf("expected intro raw input 1, got %#v", introDecision)
	}
	if introDecision.ChoiceID == "" || introDecision.TargetNodeID == "" {
		t.Fatalf("expected resolved intro choice and target: %#v", introDecision)
	}
	if turns[1].NodeID != introDecision.TargetNodeID {
		t.Fatalf("turn 2 did not follow selected intro target: got %q want %q\n%s", turns[1].NodeID, introDecision.TargetNodeID, text)
	}
	if turns[1].Decision.ChoiceID != "" {
		t.Fatalf("turn 2 should not carry cosmetic intro choice into canon: %#v", turns[1].Decision)
	}
	realDecision := decisions[1]
	if realDecision.RawInput != "1" || realDecision.ChoiceID == "" || realDecision.TargetNodeID == "" {
		t.Fatalf("expected resolved real choice: %#v", realDecision)
	}
	if turns[2].NodeID != realDecision.TargetNodeID {
		t.Fatalf("turn 3 did not follow selected real target: got %q want %q\n%s", turns[2].NodeID, realDecision.TargetNodeID, text)
	}
	if turns[2].Decision.ChoiceID != realDecision.ChoiceID {
		t.Fatalf("turn 3 commit did not carry real choice id: got %#v want %q", turns[2].Decision, realDecision.ChoiceID)
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

type countingPlanner struct {
	calls     int
	next      scene.StoryPacket
	lastInput writer.Input
}

func (p *countingPlanner) Plan(_ context.Context, input writer.Input) (scene.StoryPacket, error) {
	p.calls++
	p.lastInput = input
	return p.next, nil
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
			ForbiddenLiteralCanonTerms: []string{},
			FirstTurnExposition:        []string{"world", "player", "connection", "law", "pressure"},
			StoryArc:                   []string{"opening witness demand", "changed legal status", "family pressure widens", "public consequence"},
			ResumeSummary:              "Rin stands before the witness hall.",
			Metadata:                   cliTestResumeMetadata(),
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
					{Text: "A rival clerk watches from the public bench."},
					{Text: "Rin chooses how to enter before the real demand is named."},
				},
				Choices: []scene.Choice{{
					ID:           "open",
					Label:        "Open your hand as you enter.",
					TargetNodeID: "decision",
				}, {
					ID:           "quiet",
					Label:        "Keep your hand closed.",
					TargetNodeID: "decision",
				}},
			}, {
				ID:    "decision",
				Title: "The record is named",
				Beats: []scene.Beat{
					{Text: "The first weight is named."},
					{Text: "A record will vanish if Rin waits."},
					{Text: "The rival clerk tries to force a public answer."},
					{Text: "Rin must choose what kind of witness to become."},
				},
				Choices: []scene.Choice{{
					ID:           "pay",
					Label:        "Open the ledger.",
					TargetNodeID: "opened",
				}, {
					ID:           "refuse",
					Label:        "Refuse the ledger.",
					TargetNodeID: "refused",
				}, {
					ID:           "accuse",
					Label:        "Accuse the rival clerk.",
					TargetNodeID: "accused",
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
			}, {
				ID:      "accused",
				Title:   "Accused",
				Beats:   []scene.Beat{{Text: "The clerk's seal cracks."}},
				Choices: []scene.Choice{},
				RevisionNotes: []string{
					"Writer continuation is required after this branch.",
				},
			}},
		},
	}
}

func cliTestResumeMetadata() map[string]string {
	return map[string]string{
		"seed_version":        "test-v1",
		"generated_at":        "test",
		"resume_point":        "entry",
		"current_checkpoint":  "opening witness demand",
		"continuation_hooks":  "continue the witness pressure",
		"canon_validated":     "true",
		"open_threads":        "family debt; civic witness",
		"gameplay_mode":       "test",
		"ground_lore_pack_id": "metabarons",
	}
}
