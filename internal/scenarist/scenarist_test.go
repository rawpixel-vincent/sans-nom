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

func TestToolRunnerWritesFreeFormMaterialAndReturnsPathsOnly(t *testing.T) {
	outputDir := t.TempDir()
	tool := (&ToolRunner{
		Runner:      &fakeScenaristRunner{response: "raw free-form material\nnot json"},
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
}

func TestToolRunnerRejectsMoreRequestsThanProviderCount(t *testing.T) {
	tool := (&ToolRunner{
		Runner:      &fakeScenaristRunner{response: "material"},
		OutputDir:   t.TempDir(),
		MaxParallel: 1,
	}).LocalTool()
	_, err := tool.Handler(context.Background(), ".", `{"requests":[{"topic_id":"one","topic":"One","request":"first","hard_boundaries":["original"],"relevant_paths":[]},{"topic_id":"two","topic":"Two","request":"second","hard_boundaries":["original"],"relevant_paths":[]}]}`)
	if err == nil {
		t.Fatal("expected provider-count cap error")
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
	delay     time.Duration
	mu        sync.Mutex
	active    int
	maxActive int
}

func (r *fakeScenaristRunner) RunScenarist(context.Context, string, string) (string, error) {
	r.mu.Lock()
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
	return r.response, nil
}
