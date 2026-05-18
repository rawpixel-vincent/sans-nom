package scenarist

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

func TestToolRequestAllowsBroadRequestsButRequiresBoundaries(t *testing.T) {
	request := ToolRequest{
		TopicID:        "seed-ideas",
		Topic:          "Seed ideas",
		Request:        "Any story seed suggestions of all kind.",
		HardBoundaries: []string{"Create original material only."},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("broad request should be valid: %v", err)
	}
	request.HardBoundaries = nil
	if err := request.Validate(); err == nil {
		t.Fatal("expected hard-boundary rejection")
	}
}

func TestScenaristPromptKeepsChoicesAsNonSchemaMaterial(t *testing.T) {
	system := ScenaristSystemPrompt("", false, false)
	for _, required := range []string{
		"Do not write a StoryPacket",
		"Do not invent schema target_node_id values",
		"future placeholder node ids",
	} {
		if !strings.Contains(system, required) {
			t.Fatalf("scenarist system prompt missing %q:\n%s", required, system)
		}
	}

	query := ScenaristToolQuery("writer context", ToolRequest{
		Topic:          "Branch pressure",
		Request:        "Suggest choices.",
		HardBoundaries: []string{"Original material only."},
	})
	for _, required := range []string{
		"not as StoryPacket schema choices or target node ids",
		"Write free-form creative material",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("scenarist query missing %q:\n%s", required, query)
		}
	}
}

func TestScenaristSystemPromptExposesLoreExpertWhenAvailable(t *testing.T) {
	withExpert := ScenaristSystemPrompt("", true, true)
	if !strings.Contains(withExpert, "@LoreExpert:") {
		t.Fatalf("scenarist prompt should explain lore expert access:\n%s", withExpert)
	}
	withoutExpert := ScenaristSystemPrompt("", true, false)
	if strings.Contains(withoutExpert, "@LoreExpert:") {
		t.Fatalf("scenarist prompt should not mention unavailable lore expert:\n%s", withoutExpert)
	}
}

func TestScenaristSystemPromptListsRealReadPointers(t *testing.T) {
	system := ScenaristSystemPrompt("/repo", true, true, []string{
		"pack manifest file: lore/packs/metabarons/lore-pack.json (use Read)",
		"comics directory: lore/packs/metabarons/comics (use LS/Grep)",
	})
	for _, required := range []string{
		"Use @LoreExpert first for source-pattern questions",
		"Use Read for files and LS or Grep for directories",
		"pack manifest file: lore/packs/metabarons/lore-pack.json",
		"comics directory: lore/packs/metabarons/comics",
	} {
		if !strings.Contains(system, required) {
			t.Fatalf("scenarist system prompt missing %q:\n%s", required, system)
		}
	}
}

func TestScenaristQueryGuidesEmptyRelevantPaths(t *testing.T) {
	query := ScenaristToolQuery("writer context", ToolRequest{
		Topic:          "Seed heat",
		Request:        "Brainstorm.",
		HardBoundaries: []string{"Original material only."},
	})
	for _, required := range []string{
		"No request-specific files were provided",
		"ask @LoreExpert",
		"instead of guessing filenames",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("scenarist query missing %q:\n%s", required, query)
		}
	}
}

func TestProviderEnvMapsProviderPrefixedVariables(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ZAI__ANTHROPIC_BASE_URL=https://example.invalid",
		"ZAI__ANTHROPIC_AUTH_TOKEN=token",
		"ZAI__ANTHROPIC_MODEL=glm-4.7",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	env, model, err := ProviderEnv("ZAI", "", []string{envPath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if model != "glm-4.7" {
		t.Fatalf("model = %q", model)
	}
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{
		"\nANTHROPIC_BASE_URL=https://example.invalid\n",
		"\nANTHROPIC_AUTH_TOKEN=token\n",
		"\nANTHROPIC_MODEL=glm-4.7\n",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in env:\n%s", want, joined)
		}
	}
}

func TestProviderEnvFailsOnMissingRequiredValues(t *testing.T) {
	_, _, err := ProviderEnv("ZAI", "", nil, []string{"ZAI__ANTHROPIC_MODEL=glm-4.7"})
	if err == nil {
		t.Fatal("expected missing provider value error")
	}
}

func TestProviderNamesParsesPool(t *testing.T) {
	names := ProviderNames("zai, minimax ZAI;xiaomi")
	want := []string{"ZAI", "MINIMAX", "XIAOMI"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("provider names = %#v", names)
	}
}

func TestProviderNamesDefaultsToEnabledPool(t *testing.T) {
	t.Setenv("SANS_NOM_SCENARIST_PROVIDERS", "")
	t.Setenv("SANS_NOM_SCENARIST_PROVIDER", "")
	names := ProviderNames("")
	want := []string{"ZAI", "MINIMAX", "XIAOMI"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("default provider names = %#v", names)
	}
}

func TestUsableProvidersFiltersMissingOrPlaceholderEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(strings.Join([]string{
		"ZAI__ANTHROPIC_BASE_URL=https://example.invalid",
		"ZAI__ANTHROPIC_AUTH_TOKEN=token",
		"ZAI__ANTHROPIC_MODEL=glm-4.7",
		"MINIMAX__ANTHROPIC_BASE_URL=https://example.invalid",
		"MINIMAX__ANTHROPIC_AUTH_TOKEN=***",
		"MINIMAX__ANTHROPIC_MODEL=MiniMax-M2.7",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	got := UsableProviders([]string{"ZAI", "MINIMAX", "XIAOMI"}, "", []string{envPath}, nil)
	if strings.Join(got, ",") != "ZAI" {
		t.Fatalf("usable providers = %#v", got)
	}
}

func TestProviderPoolRunnerRoundRobinsAvailableSlots(t *testing.T) {
	pool := &ProviderPoolRunner{Runners: []ProviderRunner{{Provider: "ZAI"}, {Provider: "MINIMAX"}}}
	slot, release, err := pool.acquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := slot.runner.Provider; got != "ZAI" {
		t.Fatalf("first runner = %q", got)
	}
	slot, release2, err := pool.acquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	if got := slot.runner.Provider; got != "MINIMAX" {
		t.Fatalf("second runner = %q", got)
	}
	release()
	slot, release3, err := pool.acquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release3()
	if got := slot.runner.Provider; got != "ZAI" {
		t.Fatalf("third runner = %q", got)
	}
}

func TestProviderPoolWaitsWhenProviderBusy(t *testing.T) {
	pool := &ProviderPoolRunner{Runners: []ProviderRunner{{Provider: "ZAI"}}}
	_, release, err := pool.acquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waiting := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(waiting)
		_, release2, err := pool.acquireSlot(context.Background())
		if err == nil {
			release2()
		}
		done <- err
	}()
	<-waiting
	select {
	case err := <-done:
		t.Fatalf("provider acquired while busy: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClaudeStreamParserLogsReadToolUseAndResult(t *testing.T) {
	logger := gamelog.NewGlobalWriter(t.TempDir())
	parser := newClaudeStreamParser(ProviderRunner{Provider: "ZAI", Model: "glm-4.7", Logger: logger})
	parser.consume(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"data/seed-runs/example/output.md"}}]}}`)
	parser.consume(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"workshop material body"}]}}`)
	parser.consume(`{"type":"result","result":"final scenarist material"}`)
	if got := parser.result(); got != "final scenarist material" {
		t.Fatalf("stream result = %q", got)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "scenarist"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected tool use/result events, got %#v", events)
	}
	if events[0].Event != "scenarist.provider.tool_use" || events[0].Fields["tool_name"] != "Read" || events[0].Fields["target"] != "data/seed-runs/example/output.md" {
		t.Fatalf("unexpected tool use event: %#v", events[0])
	}
	if events[1].Event != "scenarist.provider.tool_result" || !strings.Contains(events[1].Message, "workshop material body") {
		t.Fatalf("unexpected tool result event: %#v", events[1])
	}
	detail, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mustJSON(t, detail)), "toolu_1") {
		t.Fatalf("tool detail missing raw id: %#v", detail)
	}
}

func TestToolRunnerWritesFreeFormMaterialAndReturnsPathsOnly(t *testing.T) {
	outputDir := t.TempDir()
	tool := (&ToolRunner{
		Runner: &fakeScenaristRunner{
			response: "raw free-form material\nnot json",
			metadata: modelmeta.Metadata{
				ProviderType:  "claude_compatible",
				ModelProvider: "ZAI",
				Model:         "glm-4.7",
			},
		},
		OutputDir:   outputDir,
		ContextText: "writer context",
		MaxParallel: 2,
	}).LocalTool()
	args := `{"requests":[{"topic_id":"body-law","topic":"Body law","request":"Invent original pressure material with taboos, reversals, and choices for a later packet.","hard_boundaries":["Original material only.","Do not write a StoryPacket."],"relevant_paths":[]}]}`
	result, err := tool.Handler(context.Background(), ".", args)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "raw free-form material") {
		t.Fatalf("tool response leaked raw material:\n%s", text)
	}
	if !strings.Contains(text, "body-law.md") {
		t.Fatalf("tool response missing path:\n%s", text)
	}
	matches, err := filepath.Glob(filepath.Join(outputDir, "tool-call-001", "*body-law.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("material matches = %#v", matches)
	}
	fileData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fileData), "raw free-form material") {
		t.Fatalf("missing material: %s", fileData)
	}
	manifestData, err := os.ReadFile(filepath.Join(outputDir, "tool-call-001", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestData), `"model_provider": "ZAI"`) || !strings.Contains(string(manifestData), `"model": "glm-4.7"`) {
		t.Fatalf("manifest missing per-material model metadata:\n%s", manifestData)
	}
}

func TestToolRunnerLetsScenaristAskLoreExpert(t *testing.T) {
	logger := gamelog.NewGlobalWriter(t.TempDir())
	scenaristRunner := &fakeScenaristRunner{responses: []string{
		"@LoreExpert: How do source consequence systems make a local doctrine credible?",
		"final scenarist material after source grounding",
	}}
	loreRunner := &fakeLoreExpertRunner{response: strings.Join([]string{
		"## Influence-only answer",
		"consequence systems work through public cost.",
		"## Source pattern",
		"pattern",
		"## Why it works",
		"why",
		"## How to transform it originally",
		"transform",
		"## Reader-plausibility risks",
		"risk",
		"## Do not copy",
		"copy nothing",
		"## Useful source refs",
		"none",
	}, "\n")}
	outputDir := t.TempDir()
	tool := (&ToolRunner{
		Runner:      scenaristRunner,
		OutputDir:   outputDir,
		ContextText: "writer context",
		MaxParallel: 1,
		Logger:      logger,
		CallerRole:  "creative-writer",
		CallerName:  "Creative Writer",
		LoreExpert: &loreexpert.ToolRunner{
			Runner:        loreRunner,
			CacheRoot:     t.TempDir(),
			PackID:        "metabarons",
			PackLabel:     "Metabarons",
			Collection:    "sans_nom_lore_metabarons",
			MaxSources:    1,
			Logger:        logger,
			CallerRole:    "creative-writer",
			CallerSpeaker: "Creative Writer",
		},
	}).LocalTool()
	args := `{"requests":[{"topic_id":"doctrine","topic":"Doctrine","request":"Brainstorm consequence systems and ask source grounding if useful.","hard_boundaries":["Original material only."],"relevant_paths":["lore/packs/metabarons/wiki/lore.md"]}]}`
	if _, err := tool.Handler(context.Background(), ".", args); err != nil {
		t.Fatal(err)
	}
	if scenaristRunner.calls != 2 {
		t.Fatalf("scenarist calls = %d, want 2", scenaristRunner.calls)
	}
	if loreRunner.calls != 1 {
		t.Fatalf("lore expert calls = %d, want 1", loreRunner.calls)
	}
	if len(scenaristRunner.queries) < 2 || !strings.Contains(scenaristRunner.queries[1], "Lore Expert answers") {
		t.Fatalf("second scenarist query missing lore expert answer:\n%#v", scenaristRunner.queries)
	}
	matches, err := filepath.Glob(filepath.Join(outputDir, "tool-call-001", "*doctrine.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("material matches = %#v", matches)
	}
	fileData, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fileData), "@LoreExpert") || !strings.Contains(string(fileData), "final scenarist material") {
		t.Fatalf("unexpected material file:\n%s", fileData)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "lore.expert.request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("lore expert request events = %#v", events)
	}
	if events[0].Fields["caller_role"] != "scenarist" || events[0].Fields["caller_speaker"] != "Scenarist" {
		t.Fatalf("lore expert call not attributed to scenarist: %#v", events[0].Fields)
	}
}

func TestToolRunnerWarnsOnDependentParallelRequests(t *testing.T) {
	logger := gamelog.NewGlobalWriter(t.TempDir())
	tool := (&ToolRunner{
		Runner:      &fakeScenaristRunner{response: "material"},
		OutputDir:   t.TempDir(),
		ContextText: "writer context",
		MaxParallel: 2,
		Logger:      logger,
	}).LocalTool()
	args := `{"requests":[{"topic_id":"select","topic":"Select strongest germ","request":"Choose the strongest germ from prior material.","hard_boundaries":["Original material only."],"relevant_paths":[]},{"topic_id":"scene","topic":"Scene engines for selected germ","request":"For the best candidate germ, suggest scene engines.","hard_boundaries":["Original material only."],"relevant_paths":[]}]}`
	result, err := tool.Handler(context.Background(), ".", args)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"generated independently",
		"for synthesis or dependent follow-up",
	} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("tool response missing %q:\n%s", required, data)
		}
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "scenarist.call.dependency_warning"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected dependency warning event, got %#v", events)
	}
	if events[0].Level != gamelog.LevelWarn || events[0].Fields["warning_count"] == nil {
		t.Fatalf("unexpected warning event: %#v", events[0])
	}
	detail, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(mustJSON(t, detail))
	if !strings.Contains(text, "Requests in one call run in parallel") || !strings.Contains(text, "selected germ") {
		t.Fatalf("warning detail missing guidance or matched terms:\n%s", text)
	}
}

func TestToolRunnerLogsCallsWithDetailPayloads(t *testing.T) {
	logger := gamelog.NewGlobalWriter(t.TempDir())
	tool := (&ToolRunner{
		Runner:      &fakeScenaristRunner{response: "material"},
		OutputDir:   t.TempDir(),
		ContextText: "writer context",
		MaxParallel: 1,
		Logger:      logger,
		CallerRole:  "writer",
		CallerName:  "Writer",
	}).LocalTool()
	args := `{"requests":[{"topic_id":"law-check","topic":"Law check","request":"Stress test this law for reader plausibility.","hard_boundaries":["Original material only."],"relevant_paths":["lore/packs/metabarons/wiki/lore.md"]}]}`
	if _, err := tool.Handler(context.Background(), ".", args); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Category: "scenarist"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected started and completed logs, got %#v", events)
	}
	for _, event := range events {
		if event.Fields["caller_role"] != "writer" || event.Fields["caller_speaker"] != "Writer" {
			t.Fatalf("missing caller fields: %#v", event.Fields)
		}
		if event.DetailPath == "" {
			t.Fatalf("missing detail path on event %#v", event)
		}
		detail, err := logger.ReadDetail(event.DetailPath)
		if err != nil {
			t.Fatal(err)
		}
		text := string(mustJSON(t, detail))
		if !strings.Contains(text, "law-check") {
			t.Fatalf("detail missing request payload: %s", text)
		}
	}
}

func TestToolRunnerRejectsMoreRequestsThanProviderCount(t *testing.T) {
	logger := gamelog.NewGlobalWriter(t.TempDir())
	tool := (&ToolRunner{
		Runner:      &fakeScenaristRunner{response: "material"},
		OutputDir:   t.TempDir(),
		MaxParallel: 1,
		Logger:      logger,
	}).LocalTool()
	_, err := tool.Handler(context.Background(), ".", `{"requests":[{"topic_id":"one","topic":"One","request":"first","hard_boundaries":["original"],"relevant_paths":[]},{"topic_id":"two","topic":"Two","request":"second","hard_boundaries":["original"],"relevant_paths":[]}]}`)
	if err == nil {
		t.Fatal("expected provider-count cap error")
	}
	events, readErr := gamelog.Read(logger.Path, gamelog.Filter{Event: "scenarist.call.rejected"})
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(events) != 1 || events[0].DetailPath == "" {
		t.Fatalf("expected rejected call log with details, got %#v", events)
	}
}

func TestToolRunnerRunsRequestsInParallel(t *testing.T) {
	runner := &fakeScenaristRunner{response: "material", delay: 20 * time.Millisecond}
	tool := (&ToolRunner{
		Runner:      runner,
		OutputDir:   t.TempDir(),
		MaxParallel: 2,
	}).LocalTool()
	_, err := tool.Handler(context.Background(), ".", `{"requests":[{"topic_id":"one","topic":"One","request":"first","hard_boundaries":["original"],"relevant_paths":[]},{"topic_id":"two","topic":"Two","request":"second","hard_boundaries":["original"],"relevant_paths":[]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if runner.maxActive < 2 {
		t.Fatalf("expected parallel calls, max active = %d", runner.maxActive)
	}
}

func TestManifestTextDoesNotExposeMaterial(t *testing.T) {
	manifest := Manifest{Materials: []Material{{
		TopicID: "body-law",
		Topic:   "Body law",
		Path:    "data/stories/x/model-runs/scenarists/turn-000001/tool-call-001/01-body-law.md",
	}}}
	manifestText := ManifestText(manifest)
	if strings.Contains(manifestText, "raw free-form material") {
		t.Fatalf("manifest leaked raw material:\n%s", manifestText)
	}
	if !strings.Contains(manifestText, "01-body-law.md") {
		t.Fatalf("manifest missing path:\n%s", manifestText)
	}
}

type fakeScenaristRunner struct {
	response  string
	responses []string
	queries   []string
	systems   []string
	metadata  modelmeta.Metadata
	calls     int
	delay     time.Duration
	mu        sync.Mutex
	active    int
	maxActive int
}

func (r *fakeScenaristRunner) RunScenarist(_ context.Context, systemPrompt string, query string) (string, error) {
	r.mu.Lock()
	call := r.calls
	r.calls++
	r.systems = append(r.systems, systemPrompt)
	r.queries = append(r.queries, query)
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	if call < len(r.responses) {
		return r.responses[call], nil
	}
	return r.response, nil
}

func (r *fakeScenaristRunner) RunScenaristWithMetadata(ctx context.Context, systemPrompt string, query string) (string, modelmeta.Metadata, error) {
	raw, err := r.RunScenarist(ctx, systemPrompt, query)
	return raw, r.metadata, err
}

type fakeLoreExpertRunner struct {
	response string
	calls    int
	prompts  []string
}

func (r *fakeLoreExpertRunner) Run(_ context.Context, prompt string) (string, error) {
	r.calls++
	r.prompts = append(r.prompts, prompt)
	return r.response, nil
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
