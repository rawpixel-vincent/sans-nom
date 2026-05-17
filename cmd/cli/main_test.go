package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
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

func TestBridgeDialFailureDetectsConnectionRefused(t *testing.T) {
	err := errors.New(`seed story collection "x": Post "http://127.0.0.1:8787/v1/story/index/ingest": dial tcp 127.0.0.1:8787: connect: connection refused`)
	if !bridgeDialFailure(err) {
		t.Fatal("expected connection-refused bridge error to be startup eligible")
	}
	if bridgeDialFailure(errors.New("sidecar POST /v1/story/index/ingest: 500 qdrant schema mismatch")) {
		t.Fatal("unexpectedly treated backend error as dial failure")
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
