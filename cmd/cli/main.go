package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/clitui"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

func main() {
	loadDotEnv(".env")
	noArgs := len(os.Args) == 1

	packetPath := flag.String("packet", "", "StoryPacket JSON path for test/debug packet runs")
	nodeID := flag.String("node", "", "node id to roll out; defaults to packet entry node")
	choiceID := flag.String("choice", "", "selected choice id")
	freeText := flag.String("text", "", "free-text player input")
	useClaude := flag.Bool("claude", false, "use claude -p Sonnet adapters instead of static local adapters")
	modelBackend := flag.String("model-backend", os.Getenv("SANS_NOM_MODEL_BACKEND"), "model backend: codex, claude, or static")
	noWriterRefresh := flag.Bool("no-writer-refresh", false, "do not refresh the Creative Writer between play turns")
	seedOutput := flag.String("seed-output", "", "optional path for generated seed bundle output")
	storyID := flag.String("story-id", "", "create or resume a durable story id")
	storyRoot := flag.String("story-root", story.DefaultRoot, "root directory for durable story state")
	globalLoreCollection := flag.String("global-lore-collection", story.DefaultGlobalLoreCollection, "global source-lore Qdrant collection")
	storyCollection := flag.String("story-collection", "", "story Qdrant collection; defaults from story id")
	replaceStory := flag.Bool("replace-story", false, "replace an existing durable story directory and seed collection")
	writerScenarists := flag.Int("writer-scenarists", -1, "optional max call_scenarists requests per tool call; 0 disables; default is provider count")
	scenaristProvider := flag.String("scenarist-provider", defaultScenaristProviderPool(), "Claude-compatible scenarist provider pool, comma-separated, for example ZAI,MINIMAX")
	scenaristModel := flag.String("scenarist-model", os.Getenv("SANS_NOM_SCENARIST_MODEL"), "Claude-compatible scenarist model override")
	scenaristTools := flag.Bool("scenarist-tools", envBool("SANS_NOM_SCENARIST_TOOLS"), "allow Scenarists read-only lore tools")
	scenaristOutputRoot := flag.String("scenarist-output-root", "", "optional root for Scenarist material output")
	writerScenaristRounds := flag.Int("writer-scenarist-rounds", envInt("SANS_NOM_WRITER_SCENARIST_ROUNDS", 24), "maximum Writer tool rounds while refreshing; 0 uses the 24-round safety cap")
	noPlay := flag.Bool("no-play", false, "do not enter the interactive play loop after loading or creating a story")
	noCommit := flag.Bool("no-commit", false, "do not commit witnessed scene canon to Honcho during play")
	turns := flag.Int("turns", 0, "number of automated play turns; 0 reads interactively")
	playTraceJSONL := flag.Bool("play-trace-jsonl", false, "emit machine-readable JSONL trace events during play")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	contextPreview := flag.String("context-preview", "", "preview assembled context: narrator or writer")
	contextText := flag.Bool("context-text", false, "print model-ready context text instead of pretty JSON")
	flag.Parse()
	packetExplicit := flagPassed("packet")

	if *loreSearch != "" {
		if err := runLoreSearch(context.Background(), *bridgeURL, *globalLoreCollection, *loreSearch, *loreK); err != nil {
			log.Fatal(err)
		}
		return
	}

	cardStore, err := loadCardStore()
	if err != nil {
		log.Fatal(err)
	}
	backend := resolveBackend(*modelBackend, *useClaude)
	resolvedBridgeURL := runtimeBridgeURL(backend, *bridgeURL)
	playMode := !*noPlay && *contextPreview == ""
	commitMode := !*noCommit
	playRefreshWriter := !*noWriterRefresh
	if noArgs {
		if err := runLauncher(context.Background(), launcherConfig{
			Backend:             backend,
			BridgeURL:           resolvedBridgeURL,
			CardStore:           cardStore,
			StoryRoot:           *storyRoot,
			GlobalCollection:    *globalLoreCollection,
			StoryCollection:     *storyCollection,
			ScenaristProvider:   *scenaristProvider,
			ScenaristModel:      *scenaristModel,
			ScenaristTools:      *scenaristTools,
			ScenaristOutputRoot: *scenaristOutputRoot,
			ScenaristRounds:     *writerScenaristRounds,
			RefreshWrite:        true,
			Commit:              commitMode,
			WriterScenarists:    resolveWriterScenaristCount(*writerScenarists, backend, true),
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	createStory := shouldCreateStory(*storyID, *packetPath, *storyRoot, *replaceStory)
	stopBridge, err := ensureFlagBridge(context.Background(), flagBridgeConfig{
		Backend:   backend,
		BridgeURL: resolvedBridgeURL,
		Needed:    true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stopBridge()

	var packet scene.StoryPacket
	var bible *story.StoryBible
	var workspace *story.Workspace
	var state *story.StoryState
	var bundle *story.SeedBundle
	if createStory {
		session, err := newStorySession(context.Background(), storyapp.StoryActionOptions{
			StoryID:              *storyID,
			StoryRoot:            *storyRoot,
			GlobalLoreCollection: *globalLoreCollection,
			StoryCollection:      *storyCollection,
			ReplaceStory:         *replaceStory,
		}, backend, resolvedBridgeURL)
		if err != nil {
			log.Fatal(err)
		}
		workspace = &session.Workspace
		state = &session.State
		bible = &session.Bible
		packet = session.Packet
		bundle = &session.Bundle
		if *seedOutput != "" {
			if err := printSeedBundle(*bundle, *seedOutput); err != nil {
				log.Fatal(err)
			}
		}
		if *nodeID == "" {
			*nodeID = session.NodeID
		}
		if !playMode && *contextPreview == "" {
			if *seedOutput == "" {
				if err := printSeedBundle(*bundle, ""); err != nil {
					log.Fatal(err)
				}
			}
			return
		}
	} else if *storyID != "" {
		session, err := storyapp.ResumeStory(context.Background(), *storyID, storyapp.StoryActionOptions{StoryRoot: *storyRoot})
		if err != nil {
			log.Fatal(err)
		}
		workspace = &session.Workspace
		state = &session.State
		bible = &session.Bible
		packet = session.Packet
		if *nodeID == "" {
			*nodeID = session.NodeID
		}
	} else if *packetPath != "" {
		if err := validatePacketSourceForBackend(backend, packetExplicit, *packetPath); err != nil {
			log.Fatal(err)
		}
		packet, err = openPacket(*packetPath)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("no story source: run without flags for the live menu, or pass -story-id or -packet")
	}
	decision := scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}
	if playMode {
		if err := runPlay(context.Background(), playConfig{
			Packet:              packet,
			NodeID:              *nodeID,
			Backend:             backend,
			BridgeURL:           resolvedBridgeURL,
			CardStore:           cardStore,
			Workspace:           workspace,
			State:               state,
			Bible:               bible,
			GlobalCollection:    *globalLoreCollection,
			StoryCollection:     resolvedStoryCollection(state, *storyCollection),
			InitialDecision:     decision,
			Turns:               *turns,
			Commit:              commitMode,
			RefreshWrite:        playRefreshWriter,
			TraceJSONL:          *playTraceJSONL,
			WriterScenarists:    resolveWriterScenaristCount(*writerScenarists, backend, playRefreshWriter),
			ScenaristProvider:   *scenaristProvider,
			ScenaristModel:      *scenaristModel,
			ScenaristTools:      *scenaristTools,
			ScenaristOutputRoot: *scenaristOutputRoot,
			ScenaristRounds:     *writerScenaristRounds,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *contextPreview != "" {
		if err := runContextPreview(context.Background(), *contextPreview, *contextText, resolvedBridgeURL, packet, *nodeID, decision, bible, *globalLoreCollection, resolvedStoryCollection(state, *storyCollection), cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}
	runner := claudeRunner()
	registry, err := actorRegistry(cardStore)
	if err != nil {
		log.Fatal(err)
	}
	assembler := contextAssembler(resolvedBridgeURL, false, cardStore, *globalLoreCollection, resolvedStoryCollection(state, *storyCollection))
	narratorContext, err := assembler.AssembleNarrator(context.Background(), contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   *nodeID,
		Decision: decision,
		Bible:    bible,
	})
	if err != nil {
		log.Fatal(err)
	}
	var actorInvoker actors.Invoker = actors.StaticInvoker{}
	var renderer narrator.Renderer = narrator.StaticRenderer{}
	switch backend {
	case "claude":
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	case "codex":
		actorInvoker = actors.ClaudeInvoker{Runner: codexRunner("ActorOutput", prompt.ActorOutputSchema())}
		renderer = narrator.ClaudeRenderer{Runner: codexRunner("NarratorDraft", prompt.NarratorDraftSchema())}
	}
	svc := narrator.Service{
		Actors: &actors.Service{
			Registry: registry,
			Invoker:  actorInvoker,
		},
		Renderer: renderer,
	}
	rollout, err := svc.Rollout(context.Background(), packet, *nodeID, scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}, narratorContext.RenderText())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(rollout.Draft.Prose)
	if len(rollout.Draft.Choices) > 0 {
		fmt.Println("\nChoices:")
		for _, choice := range rollout.Draft.Choices {
			fmt.Printf("- %s: %s\n", choice.ID, choice.Label)
		}
	}
	fmt.Printf("\nCommit prepared: packet=%s node=%s witnesses=%s events=%d\n",
		rollout.Commit.PacketID,
		rollout.Commit.NodeID,
		strings.Join(rollout.Commit.Witnesses, ","),
		len(rollout.Commit.Events),
	)
}

func runLoreSearch(ctx context.Context, bridgeURL string, collection string, query string, k int) error {
	retriever := lore.BridgeRetriever{
		Client:     bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN")),
		Collection: collection,
	}
	results, err := retriever.Search(ctx, query, k)
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Printf("%.3f %s %s\n", result.Score, result.ID, result.Source)
		if text := preview(result.Text, 240); text != "" {
			fmt.Printf("  %s\n", text)
		}
	}
	return nil
}

func printSeedBundle(bundle story.SeedBundle, outputPath string) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if outputPath != "" {
		return os.WriteFile(outputPath, data, 0o644)
	}
	_, err = os.Stdout.Write(data)
	return err
}

type launcherConfig struct {
	Backend             string
	BridgeURL           string
	CardStore           *contextbundle.LocalCardStore
	StoryRoot           string
	GlobalCollection    string
	StoryCollection     string
	WriterScenarists    int
	ScenaristProvider   string
	ScenaristModel      string
	ScenaristTools      bool
	ScenaristOutputRoot string
	ScenaristRounds     int
	RefreshWrite        bool
	Commit              bool
}

type flagBridgeConfig struct {
	Backend   string
	BridgeURL string
	Needed    bool
}

func runLauncher(ctx context.Context, cfg launcherConfig) error {
	stopBridge, err := ensureLauncherBridge(ctx, cfg)
	if err != nil {
		return err
	}
	defer stopBridge()

	action, err := clitui.Run(ctx, cfg.StoryRoot)
	if err != nil {
		return err
	}
	switch action.Kind {
	case clitui.ActionNewStory:
		session, err := newStorySession(ctx, storyapp.StoryActionOptions{
			StoryRoot:            cfg.StoryRoot,
			GlobalLoreCollection: cfg.GlobalCollection,
			StoryCollection:      cfg.StoryCollection,
		}, cfg.Backend, cfg.BridgeURL)
		if err != nil {
			return err
		}
		return runPlay(ctx, playConfigFromSession(session, cfg))
	case clitui.ActionResumeStory:
		session, err := storyapp.ResumeStory(ctx, action.StoryID, storyapp.StoryActionOptions{StoryRoot: cfg.StoryRoot})
		if err != nil {
			return err
		}
		return runPlay(ctx, playConfigFromSession(session, cfg))
	case clitui.ActionExit, clitui.ActionNone:
		return nil
	default:
		return fmt.Errorf("unknown launcher action %d", action.Kind)
	}
}

func newStorySession(ctx context.Context, opts storyapp.StoryActionOptions, backend string, bridgeURL string) (storyapp.StorySession, error) {
	opts.GenerateSeedBundle = func(ctx context.Context, action storyapp.StoryActionOptions) (story.SeedBundle, error) {
		return generateSeedBundle(ctx, backend, bridgeURL, action.GlobalLoreCollection)
	}
	opts.SeedExternal = func(ctx context.Context, session storyapp.StorySession, bundle story.SeedBundle) error {
		return maybeSeedExternalStory(ctx, backend, bridgeURL, session.Workspace, session.State, bundle)
	}
	return storyapp.NewStory(ctx, opts)
}

func ensureFlagBridge(ctx context.Context, cfg flagBridgeConfig) (func(), error) {
	if !cfg.Needed || cfg.Backend == "static" {
		return func() {}, nil
	}
	return ensureBridge(ctx, cfg.Backend, cfg.BridgeURL)
}

func ensureLauncherBridge(ctx context.Context, cfg launcherConfig) (func(), error) {
	return ensureBridge(ctx, cfg.Backend, cfg.BridgeURL)
}

func ensureBridge(ctx context.Context, backend string, bridgeURL string) (func(), error) {
	if backend == "static" {
		return func() {}, nil
	}
	client := bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN"))
	if err := bridgeHealth(ctx, client, 500*time.Millisecond); err == nil {
		return func() {}, nil
	} else if !bridgeDialFailure(err) {
		return nil, fmt.Errorf("bridge health check failed at %s: %w", client.BaseURL, err)
	}

	host, port, err := bridgeListenAddress(client.BaseURL)
	if err != nil {
		return nil, err
	}
	logPath := filepath.Join("tmp", "bridge-sidecar.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("uv", "run", "--project", "bridge", "uvicorn", "bridge.main:app", "--host", host, "--port", port)
	cmd.Dir = repoRoot()
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start bridge sidecar: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		_ = logFile.Close()
	}()

	stop := func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			return nil, fmt.Errorf("bridge sidecar exited during startup: %w; see %s%s", err, logPath, logTail(logPath))
		default:
		}
		if err := bridgeHealth(ctx, client, time.Second); err == nil {
			return stop, nil
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	stop()
	return nil, fmt.Errorf("bridge sidecar did not become healthy at %s: %w; see %s%s", client.BaseURL, lastErr, logPath, logTail(logPath))
}

func bridgeHealth(ctx context.Context, client bridgeclient.Client, timeout time.Duration) error {
	healthCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Health(healthCtx)
}

func bridgeListenAddress(rawURL string) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	if !localBridgeHost(host) {
		return "", "", fmt.Errorf("bridge unavailable at %s and auto-start only supports localhost bridge URLs", rawURL)
	}
	port := parsed.Port()
	if port == "" {
		switch parsed.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return host, port, nil
}

func localBridgeHost(host string) bool {
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func bridgeDialFailure(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "connect: connection refused") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such host") ||
		strings.Contains(text, "dial tcp")
}

func logTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > 2000 {
		data = data[len(data)-2000:]
	}
	return "\n\n" + strings.TrimSpace(string(data))
}

func playConfigFromSession(session storyapp.StorySession, cfg launcherConfig) playConfig {
	workspace := session.Workspace
	state := session.State
	bible := session.Bible
	globalCollection := cfg.GlobalCollection
	if state.GlobalLoreCollection != "" {
		globalCollection = state.GlobalLoreCollection
	}
	return playConfig{
		Packet:              session.Packet,
		NodeID:              session.NodeID,
		Backend:             cfg.Backend,
		BridgeURL:           cfg.BridgeURL,
		CardStore:           cfg.CardStore,
		Workspace:           &workspace,
		State:               &state,
		Bible:               &bible,
		GlobalCollection:    globalCollection,
		StoryCollection:     resolvedStoryCollection(&state, cfg.StoryCollection),
		Commit:              cfg.Commit,
		RefreshWrite:        cfg.RefreshWrite,
		WriterScenarists:    cfg.WriterScenarists,
		ScenaristProvider:   cfg.ScenaristProvider,
		ScenaristModel:      cfg.ScenaristModel,
		ScenaristTools:      cfg.ScenaristTools,
		ScenaristOutputRoot: cfg.ScenaristOutputRoot,
		ScenaristRounds:     cfg.ScenaristRounds,
	}
}

func generateSeedBundle(ctx context.Context, backend string, bridgeURL string, globalCollection string) (story.SeedBundle, error) {
	contextText := seed.ContextText()
	if backend != "static" {
		contextText = buildSeedContext(ctx, seedLoreRetriever(bridgeURL, globalCollection))
	}
	return seed.Service{
		Scenarist:     seedRunner(backend, true),
		Verifier:      seedRunner(backend, false),
		MaxIterations: 2,
	}.Generate(ctx, contextText)
}

const seedInfluenceQuery = "original science fantasy story seed gravity law taboo body technology witness memory inheritance refusal authority spectacle suspension of disbelief Bushitaka law"

func seedLoreRetriever(bridgeURL string, globalCollection string) lore.Retriever {
	if globalCollection == "" {
		globalCollection = story.DefaultGlobalLoreCollection
	}
	return lore.BridgeRetriever{
		Client:     bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN")),
		Collection: globalCollection,
	}
}

func buildSeedContext(ctx context.Context, retriever lore.Retriever) string {
	contextText := seed.ContextText()
	if retriever == nil {
		return contextText
	}
	hits, err := retriever.Search(ctx, seedInfluenceQuery, 8)
	if err != nil {
		return contextText + "\n\nSource-lore retrieval warning: " + err.Error() + "\nProceed from seed doctrine only. Do not copy any fallback fixture."
	}
	if len(hits) == 0 {
		return contextText + "\n\nSource-lore influence snippets: no live snippets found. Proceed from seed doctrine only."
	}
	var b strings.Builder
	b.WriteString(contextText)
	b.WriteString("\n\nSource-lore influence snippets (craft DNA only; do not copy names, events, factions, or proper nouns):")
	for _, hit := range hits {
		label := hit.ID
		if hit.Kind != "" {
			label = hit.Kind + " " + label
		}
		if hit.Source != "" {
			label += " " + hit.Source
		}
		b.WriteString("\n- ")
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(preview(hit.Text, 700))
	}
	b.WriteString("\n\nUse these snippets to infer structural craft: laws that carry consequence, character refusal, body-tech pressure, credible social reaction, and suspension of disbelief. The seed must remain an original story.")
	return b.String()
}

func seedRunner(backend string, scenarist bool) prompt.Runner {
	switch backend {
	case "static":
		return nil
	case "claude":
		return claudeRunner()
	default:
		if scenarist {
			return codexRunner("SeedBundle", prompt.SeedBundleSchema())
		}
		return codexRunner("SeedReview", prompt.SeedReviewSchema())
	}
}

func maybeSeedExternalStory(ctx context.Context, backend string, bridgeURL string, ws story.Workspace, state story.StoryState, bundle story.SeedBundle) error {
	if backend == "static" {
		return nil
	}
	client := bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN"))
	docs, err := ws.ReadIndexDocuments()
	if err != nil {
		return err
	}
	if _, err := client.IngestStoryIndex(ctx, bridgeclient.StoryIndexIngestRequest{
		Collection: state.StoryCollection,
		Documents:  bridgeStoryDocs(docs),
		Replace:    true,
	}); err != nil {
		return fmt.Errorf("seed story collection %q: %w", state.StoryCollection, err)
	}
	for peerID, facts := range seedPeerCards(bundle.Bible) {
		if _, err := client.PatchCard(ctx, state.WorkspaceID, peerID, bridgeclient.PeerCardPatch{Facts: facts, Replace: true}); err != nil {
			return fmt.Errorf("seed Honcho card %q: %w", peerID, err)
		}
	}
	return nil
}

func bridgeStoryDocs(docs []story.IndexDocument) []bridgeclient.StoryIndexDocument {
	out := make([]bridgeclient.StoryIndexDocument, 0, len(docs))
	for _, doc := range docs {
		out = append(out, bridgeclient.StoryIndexDocument{
			DocID:        doc.DocID,
			PointID:      doc.PointID,
			Kind:         doc.Kind,
			SourcePath:   doc.SourcePath,
			SourceSHA256: doc.SourceSHA256,
			TextSHA256:   doc.TextSHA256,
			Text:         doc.Text,
			StoryID:      doc.StoryID,
			PacketID:     doc.PacketID,
			NodeID:       doc.NodeID,
			Turn:         doc.Turn,
			Heading:      doc.Heading,
			Payload:      doc.Payload,
		})
	}
	return out
}

func seedPeerCards(bible story.StoryBible) map[string][]string {
	lawFacts := make([]string, 0, len(bible.OriginalLaws))
	for _, law := range bible.OriginalLaws {
		lawFacts = append(lawFacts, fmt.Sprintf("INSTRUCTION: Enforce original story law %s: %s", law.Name, law.Enforcement))
	}
	baseBoundaries := []string{
		"BOUNDARY: Source lore is influence material, not story canon.",
		"BOUNDARY: Do not import Metabarons canon names, places, factions, or events into the generated story.",
	}
	narrator := []string{
		"Name: narrator",
		"ROLE: Player-facing narrator for " + bible.Title + ".",
		"INSTRUCTION: Render the selected node into witnessed prose grounded in the Story Bible.",
		"INSTRUCTION: Keep private actor interiors and Writer planning out of player-facing prose.",
	}
	narrator = append(narrator, lawFacts...)
	narrator = append(narrator, baseBoundaries...)

	writer := []string{
		"Name: creative-writer",
		"ROLE: Background planner for bounded StoryPacket revisions in " + bible.Title + ".",
		"INSTRUCTION: Use the Story Bible as binding doctrine and source lore as creative influence.",
		"INSTRUCTION: Create original branches and preserve witness-bound canon boundaries.",
	}
	writer = append(writer, lawFacts...)
	writer = append(writer, baseBoundaries...)

	player := []string{
		"Name: " + bible.PlayerCharacter.Name,
		"ROLE: Player character peer " + bible.PlayerCharacter.PeerID + ".",
		"TRAIT: " + bible.PlayerCharacter.Description,
		"TRAIT: Desire: " + bible.PlayerCharacter.Desire,
		"TRAIT: Wound: " + bible.PlayerCharacter.Wound,
		"BOUNDARY: " + bible.PlayerCharacter.Boundary,
		"RELATIONSHIP: world: " + bible.PlayerWorldConnection,
	}
	player = append(player, lawFacts...)
	return map[string][]string{
		"narrator":                   trimCardFacts(narrator, 40),
		"creative-writer":            trimCardFacts(writer, 40),
		bible.PlayerCharacter.PeerID: trimCardFacts(player, 40),
	}
}

func trimCardFacts(facts []string, max int) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		out = append(out, fact)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func runContextPreview(
	ctx context.Context,
	role string,
	asText bool,
	bridgeURL string,
	packet scene.StoryPacket,
	nodeID string,
	decision scene.PlayerDecision,
	bible *story.StoryBible,
	globalCollection string,
	storyCollection string,
	cardStore *contextbundle.LocalCardStore,
) error {
	assembler := contextAssembler(bridgeURL, bridgeURL != "", cardStore, globalCollection, storyCollection)
	req := contextbundle.BuildRequest{Packet: packet, NodeID: nodeID, Decision: decision, Bible: bible}
	switch role {
	case "narrator":
		context, err := assembler.AssembleNarrator(ctx, req)
		if err != nil {
			return err
		}
		if asText {
			fmt.Println(context.RenderText())
			return nil
		}
		return printPrettyJSON(context)
	case "writer":
		context, err := assembler.AssembleWriter(ctx, req)
		if err != nil {
			return err
		}
		if asText {
			fmt.Println(context.RenderText())
			return nil
		}
		return printPrettyJSON(context)
	default:
		return fmt.Errorf("unknown context preview role %q", role)
	}
}

type playConfig struct {
	Packet              scene.StoryPacket
	NodeID              string
	Backend             string
	BridgeURL           string
	CardStore           *contextbundle.LocalCardStore
	Workspace           *story.Workspace
	State               *story.StoryState
	Bible               *story.StoryBible
	GlobalCollection    string
	StoryCollection     string
	InitialDecision     scene.PlayerDecision
	Turns               int
	Commit              bool
	RefreshWrite        bool
	TraceJSONL          bool
	WriterScenarists    int
	ScenaristProvider   string
	ScenaristModel      string
	ScenaristTools      bool
	ScenaristOutputRoot string
	ScenaristRounds     int
}

func runPlay(ctx context.Context, cfg playConfig) error {
	packet := cfg.Packet
	nodeID := cfg.NodeID
	decision := cfg.InitialDecision
	var state *story.StoryState
	if cfg.State != nil {
		copied := *cfg.State
		state = &copied
	}
	reader := bufio.NewReader(os.Stdin)
	totalTurns := cfg.Turns
	for turn := 1; totalTurns <= 0 || turn <= totalTurns; turn++ {
		rollout, err := rolloutOnce(ctx, packet, nodeID, decision, cfg)
		if err != nil {
			return err
		}
		fmt.Printf("\n--- Turn %d ---\n\n%s\n", turn, rollout.Draft.Prose)
		if len(rollout.Draft.Choices) > 0 {
			fmt.Println("\nChoices:")
			for _, choice := range rollout.Draft.Choices {
				fmt.Printf("- %s: %s\n", choice.ID, choice.Label)
			}
		}
		if cfg.Commit {
			client := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
			if err := client.CommitScene(ctx, rollout.Commit); err != nil {
				return err
			}
			fmt.Printf("\nCommitted: packet=%s node=%s\n", rollout.Commit.PacketID, rollout.Commit.NodeID)
		} else {
			fmt.Printf("\nCommit prepared: packet=%s node=%s witnesses=%s events=%d\n",
				rollout.Commit.PacketID,
				rollout.Commit.NodeID,
				strings.Join(rollout.Commit.Witnesses, ","),
				len(rollout.Commit.Events),
			)
		}
		if cfg.TraceJSONL {
			if err := writePlayTrace(playTraceTurn{
				Event:            "turn",
				Turn:             turn,
				PacketID:         rollout.Commit.PacketID,
				NodeID:           rollout.Commit.NodeID,
				Decision:         rollout.Commit.PlayerDecision,
				ChoiceCount:      len(rollout.Draft.Choices),
				ProseChars:       len([]rune(rollout.Draft.Prose)),
				CommitEventCount: len(rollout.Commit.Events),
				Witnesses:        rollout.Commit.Witnesses,
			}); err != nil {
				return err
			}
		}
		if totalTurns > 0 && turn == totalTurns {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", cfg.Commit); err != nil {
				return err
			}
			break
		}
		if len(rollout.Node.Choices) == 0 && !cfg.RefreshWrite {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", cfg.Commit); err != nil {
				return err
			}
			fmt.Println("\nNo choices available; play loop stopped.")
			break
		}
		nextDecision := automatedDecision(rollout.Node)
		rawInput := ""
		if cfg.Turns <= 0 {
			fmt.Print("\nchoice id or free text> ")
			line, readErr := reader.ReadString('\n')
			if readErr != nil && strings.TrimSpace(line) == "" {
				fmt.Println("\nInput closed; play loop stopped.")
				break
			}
			fmt.Println()
			rawInput = strings.TrimSpace(line)
			nextDecision = parseDecision(rawInput, rollout.Node)
		}
		nextNodeID := ""
		if cfg.RefreshWrite {
			refreshCfg := cfg
			refreshCfg.State = state
			next, err := refreshPacket(ctx, packet, nodeID, nextDecision, refreshCfg)
			if err == nil {
				nextNodeID = entryNodeID(next)
				if err := persistStoryTurn(ctx, cfg, state, packet, rollout, nextNodeID, cfg.Commit); err != nil {
					return err
				}
				if err := saveCurrentStoryPacket(ctx, cfg, state, next, nextNodeID); err != nil {
					return err
				}
				packet = next
				nodeID = nextNodeID
				decision = nextDecision
				if cfg.TraceJSONL {
					if err := writePlayTrace(decisionTrace(rawInput, nextDecision, nextNodeID)); err != nil {
						return err
					}
				}
				continue
			}
			fmt.Fprintf(os.Stderr, "writer refresh failed, continuing current packet: %v\n", err)
		}
		if choice, err := rollout.Node.Choice(nextDecision.ChoiceID); err == nil && choice.TargetNodeID != "" {
			nextNodeID = choice.TargetNodeID
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, nextNodeID, cfg.Commit); err != nil {
				return err
			}
			nodeID = nextNodeID
			decision = nextDecision
			if cfg.TraceJSONL {
				if err := writePlayTrace(decisionTrace(rawInput, nextDecision, nextNodeID)); err != nil {
					return err
				}
			}
		} else if nextDecision.FreeText != "" && !cfg.RefreshWrite {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", cfg.Commit); err != nil {
				return err
			}
			if cfg.TraceJSONL {
				if err := writePlayTrace(decisionTrace(rawInput, nextDecision, "")); err != nil {
					return err
				}
			}
			fmt.Println("\nFree text needs Writer refresh enabled to create the next packet; play loop stopped.")
			break
		} else if len(rollout.Node.Choices) == 0 {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", cfg.Commit); err != nil {
				return err
			}
			fmt.Println("\nNo choices available; play loop stopped.")
			break
		}
	}
	return nil
}

func rolloutOnce(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg playConfig) (narrator.Rollout, error) {
	registry, err := actorRegistry(cfg.CardStore)
	if err != nil {
		return narrator.Rollout{}, err
	}
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection)
	narratorContext, err := assembler.AssembleNarrator(ctx, contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   nodeID,
		Decision: decision,
		Bible:    cfg.Bible,
	})
	if err != nil {
		return narrator.Rollout{}, err
	}
	actorInvoker := actors.Invoker(actors.StaticInvoker{})
	renderer := narrator.Renderer(narrator.StaticRenderer{})
	switch cfg.Backend {
	case "claude":
		runner := claudeRunner()
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	case "codex":
		actorInvoker = actors.ClaudeInvoker{Runner: codexRunner("ActorOutput", prompt.ActorOutputSchema())}
		renderer = narrator.ClaudeRenderer{Runner: codexRunner("NarratorDraft", prompt.NarratorDraftSchema())}
	}
	return narrator.Service{
		Actors:   &actors.Service{Registry: registry, Invoker: actorInvoker},
		Renderer: renderer,
	}.Rollout(ctx, packet, nodeID, decision, narratorContext.RenderText())
}

func refreshPacket(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg playConfig) (scene.StoryPacket, error) {
	outputDir := scenaristOutputDir(cfg)
	scenaristEnabled := cfg.WriterScenarists != 0 && strings.TrimSpace(outputDir) != "" && len(scenaristProviderRunners(cfg)) > 0
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection)
	if scenaristEnabled {
		assembler.Options.WriterLoreK = 4
	}
	writerContext, err := assembler.AssembleWriter(ctx, contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   nodeID,
		Decision: decision,
		Bible:    cfg.Bible,
	})
	if err != nil {
		return scene.StoryPacket{}, err
	}
	runner := claudeRunner()
	toolRunner, err := scenaristToolRunner(cfg, writerContext.RenderText(), outputDir)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	if toolRunner != nil && cfg.Backend == "claude" {
		return scene.StoryPacket{}, fmt.Errorf("scenarist Writer tool is only supported for codex backend; use -model-backend codex or -writer-scenarists 0")
	}
	planner := writerPlannerWithMaterialAccess(cfg.Backend, runner, outputDir, toolRunner, cfg.ScenaristRounds)
	if toolRunner != nil {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return scene.StoryPacket{}, err
		}
	}
	job, err := writer.Service{Planner: planner}.RefreshAsync(ctx, writer.Input{
		Current:        packet,
		LatestDecision: decisionText(decision.ChoiceID, decision.FreeText),
		ContextText:    writerContext.RenderText(),
	})
	if err != nil {
		return scene.StoryPacket{}, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, writer.DefaultAwaitTimeout())
	defer cancel()
	result := job.Await(waitCtx)
	return result.Packet, result.Err
}

func persistStoryTurn(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, nextNodeID string, committed bool) error {
	if cfg.Workspace == nil || state == nil || cfg.Bible == nil {
		return nil
	}
	record := story.TurnRecord{
		StoryID:    state.StoryID,
		PacketID:   rollout.Commit.PacketID,
		NodeID:     rollout.Commit.NodeID,
		Decision:   rollout.Commit.PlayerDecision,
		Prose:      rollout.Draft.Prose,
		Choices:    rollout.Node.Choices,
		Commit:     rollout.Commit,
		Committed:  committed,
		NextNodeID: nextNodeID,
	}
	nextState, err := cfg.Workspace.RecordTurn(*state, packet, record)
	if err != nil {
		return err
	}
	*state = nextState
	return refreshStoryIndex(ctx, cfg, state, packet)
}

func saveCurrentStoryPacket(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket, nodeID string) error {
	if cfg.Workspace == nil || state == nil || cfg.Bible == nil {
		return nil
	}
	if err := cfg.Workspace.SavePacket(packet); err != nil {
		return err
	}
	state.CurrentPacketID = packet.ID
	state.CurrentPacketPath = filepath.ToSlash(filepath.Join("packets", packet.ID+".json"))
	state.CurrentNodeID = nodeID
	if err := cfg.Workspace.SaveState(*state); err != nil {
		return err
	}
	return refreshStoryIndex(ctx, cfg, state, packet)
}

func refreshStoryIndex(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket) error {
	if cfg.Workspace == nil || state == nil || cfg.Bible == nil {
		return nil
	}
	turns, err := cfg.Workspace.LoadTurns()
	if err != nil {
		return err
	}
	docs := story.BuildIndexDocuments(*cfg.Bible, packet, turns)
	if err := cfg.Workspace.WriteIndex(docs, state.StoryCollection); err != nil {
		return err
	}
	if cfg.Backend == "static" {
		return nil
	}
	client := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
	_, err = client.IngestStoryIndex(ctx, bridgeclient.StoryIndexIngestRequest{
		Collection: state.StoryCollection,
		Documents:  bridgeStoryDocs(docs),
		Replace:    false,
	})
	if err != nil {
		return fmt.Errorf("refresh story collection %q: %w", state.StoryCollection, err)
	}
	return nil
}

func entryNodeID(packet scene.StoryPacket) string {
	if packet.EntryNodeID != "" {
		return packet.EntryNodeID
	}
	if node, err := packet.EntryNode(); err == nil {
		return node.ID
	}
	return ""
}

func resolvedStoryCollection(state *story.StoryState, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if state != nil && state.StoryCollection != "" {
		return state.StoryCollection
	}
	return ""
}

func automatedDecision(node scene.SceneNode) scene.PlayerDecision {
	if len(node.Choices) > 0 {
		return scene.PlayerDecision{ChoiceID: node.Choices[0].ID}
	}
	return scene.PlayerDecision{FreeText: "wait"}
}

func parseDecision(line string, node scene.SceneNode) scene.PlayerDecision {
	if line == "" {
		return automatedDecision(node)
	}
	if choice, ok := numericChoice(line, node); ok {
		return scene.PlayerDecision{ChoiceID: choice.ID}
	}
	for _, choice := range node.Choices {
		if line == choice.ID {
			return scene.PlayerDecision{ChoiceID: line}
		}
	}
	return scene.PlayerDecision{FreeText: line}
}

func numericChoice(line string, node scene.SceneNode) (scene.Choice, bool) {
	index, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return scene.Choice{}, false
	}
	if index < 1 || index > len(node.Choices) {
		return scene.Choice{}, false
	}
	return node.Choices[index-1], true
}

type playTraceTurn struct {
	Event            string               `json:"event"`
	Turn             int                  `json:"turn"`
	PacketID         string               `json:"packet_id"`
	NodeID           string               `json:"node_id"`
	Decision         scene.PlayerDecision `json:"decision"`
	ChoiceCount      int                  `json:"choice_count"`
	ProseChars       int                  `json:"prose_chars"`
	CommitEventCount int                  `json:"commit_event_count"`
	Witnesses        []string             `json:"witnesses,omitempty"`
}

type playTraceDecision struct {
	Event        string `json:"event"`
	RawInput     string `json:"raw_input,omitempty"`
	ChoiceID     string `json:"choice_id,omitempty"`
	TargetNodeID string `json:"target_node_id,omitempty"`
	FreeText     string `json:"free_text,omitempty"`
}

func decisionTrace(rawInput string, decision scene.PlayerDecision, targetNodeID string) playTraceDecision {
	return playTraceDecision{
		Event:        "player_decision",
		RawInput:     rawInput,
		ChoiceID:     decision.ChoiceID,
		TargetNodeID: targetNodeID,
		FreeText:     decision.FreeText,
	}
}

func writePlayTrace(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printPrettyJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func contextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore, globalCollection string, storyCollection string) contextbundle.Assembler {
	assembler := contextbundle.Assembler{
		Cards: cardStore,
		Options: contextbundle.Options{
			RequireBridge: strict,
			RequireLore:   strict,
		},
	}
	if strict || bridgeURL != "" {
		client := bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN"))
		assembler.Bridge = client
		var retrievers []lore.Retriever
		if globalCollection == "" {
			globalCollection = story.DefaultGlobalLoreCollection
		}
		retrievers = append(retrievers, lore.BridgeRetriever{Client: client, Collection: globalCollection})
		if storyCollection != "" && storyCollection != globalCollection {
			retrievers = append(retrievers, lore.BridgeRetriever{Client: client, Collection: storyCollection})
		}
		if len(retrievers) == 1 {
			assembler.Lore = retrievers[0]
		} else {
			assembler.Lore = lore.MultiRetriever{Retrievers: retrievers}
		}
	}
	return assembler
}

func preview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func validatePacketSourceForBackend(backend string, packetExplicit bool, packetPath string) error {
	if packetPath == "" {
		return errors.New("packet path is required")
	}
	if backend == "static" || packetExplicit {
		return nil
	}
	return errors.New("live packet run requires an explicit -packet; testdata is never selected by default")
}

func flagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func shouldCreateStory(storyID string, packetPath string, storyRoot string, replaceStory bool) bool {
	if strings.TrimSpace(packetPath) != "" {
		return false
	}
	if replaceStory || strings.TrimSpace(storyID) == "" {
		return true
	}
	return !storyWorkspaceDirExists(storyRoot, storyID)
}

func storyWorkspaceDirExists(root string, storyID string) bool {
	ws, err := story.NewWorkspace(root, storyID)
	if err != nil {
		return false
	}
	info, err := os.Stat(ws.Dir)
	return err == nil && info.IsDir()
}

func resolveBackend(value string, useClaude bool) string {
	if useClaude {
		return "claude"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "codex"
	}
	switch value {
	case "codex", "claude", "static":
		return value
	default:
		return "codex"
	}
}

func runtimeBridgeURL(backend string, bridgeURL string) string {
	if strings.TrimSpace(bridgeURL) != "" {
		return bridgeURL
	}
	if backend == "static" {
		return ""
	}
	return bridgeclient.NewClient("", "").BaseURL
}

func writerPlannerWithMaterialAccess(backend string, claude prompt.ClaudeRunner, materialDir string, scenaristTool *scenarist.ToolRunner, maxToolRounds int) writer.Planner {
	switch backend {
	case "static":
		return writer.StaticPlanner{}
	case "claude":
		if materialDir != "" {
			claude.PermissionMode = "dontAsk"
			claude.AllowedTools = []string{"Read", "LS", "Grep"}
			claude.AddDirs = append(claude.AddDirs, materialDir)
		}
		return writer.ClaudePlanner{Runner: claude}
	default:
		return writer.ClaudePlanner{Runner: codexRunnerWithMaterialAccess("StoryPacket", prompt.StoryPacketSchema(), materialDir, scenaristTool, maxToolRounds)}
	}
}

func codexRunner(schemaName string, schema map[string]any) prompt.CodexRunner {
	model := os.Getenv("SANS_NOM_CODEX_MODEL")
	if model == "" {
		model = "gpt-5.5"
	}
	effort := os.Getenv("SANS_NOM_CODEX_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	serviceTier := os.Getenv("SANS_NOM_CODEX_OAUTH_SERVICE_TIER")
	if serviceTier == "" {
		serviceTier = "priority"
	}
	return prompt.CodexRunner{
		Model:       model,
		Effort:      effort,
		ServiceTier: serviceTier,
		SchemaName:  schemaName,
		Schema:      schema,
		Timeout:     15 * time.Minute,
	}
}

func codexRunnerWithMaterialAccess(schemaName string, schema map[string]any, materialDir string, scenaristTool *scenarist.ToolRunner, maxToolRounds int) prompt.CodexRunner {
	runner := codexRunner(schemaName, schema)
	if strings.TrimSpace(materialDir) != "" || scenaristTool != nil {
		runner.LocalTools = true
		runner.ToolRoot = repoRoot()
		if maxToolRounds <= 0 {
			maxToolRounds = 24
		}
		runner.MaxToolRounds = maxToolRounds
		if scenaristTool != nil {
			runner.ExtraTools = []codexoauth.LocalTool{scenaristTool.LocalTool()}
		}
	}
	return runner
}

func claudeRunner() prompt.ClaudeRunner {
	return prompt.ClaudeRunner{
		Model:   "sonnet",
		Effort:  "medium",
		WorkDir: ".",
		Timeout: 15 * time.Minute,
	}
}

func scenaristToolRunner(cfg playConfig, contextText string, outputDir string) (*scenarist.ToolRunner, error) {
	if cfg.WriterScenarists == 0 || strings.TrimSpace(outputDir) == "" {
		return nil, nil
	}
	runners := scenaristProviderRunners(cfg)
	if len(runners) == 0 {
		return nil, nil
	}
	maxParallel := len(runners)
	if cfg.WriterScenarists > 0 && cfg.WriterScenarists < maxParallel {
		maxParallel = cfg.WriterScenarists
	}
	return &scenarist.ToolRunner{
		Runner:      &scenarist.ProviderPoolRunner{Runners: runners},
		OutputDir:   outputDir,
		RepoRoot:    repoRoot(),
		ContextText: contextText,
		Tools:       cfg.ScenaristTools,
		MaxParallel: maxParallel,
	}, nil
}

func scenaristProviderRunners(cfg playConfig) []scenarist.ProviderRunner {
	names := scenarist.ProviderNames(cfg.ScenaristProvider)
	names = scenarist.UsableProviders(names, cfg.ScenaristModel, scenarist.DefaultEnvFiles("."), os.Environ())
	runners := make([]scenarist.ProviderRunner, 0, len(names))
	for _, name := range names {
		runners = append(runners, scenarist.ProviderRunner{
			Provider: name,
			Model:    cfg.ScenaristModel,
			WorkDir:  ".",
			EnvFiles: scenarist.DefaultEnvFiles("."),
			Tools:    cfg.ScenaristTools,
			AddDirs:  scenaristAddDirs(cfg),
			Timeout:  15 * time.Minute,
		})
	}
	return runners
}

func scenaristAddDirs(cfg playConfig) []string {
	dirs := []string{"lore"}
	if cfg.Workspace != nil {
		dirs = append(dirs, cfg.Workspace.Dir)
	}
	return dirs
}

func scenaristOutputDir(cfg playConfig) string {
	root := strings.TrimSpace(cfg.ScenaristOutputRoot)
	if root != "" {
		return filepath.Join(root, scenaristTurnSegment(cfg))
	}
	if cfg.Workspace == nil {
		return ""
	}
	return filepath.Join(cfg.Workspace.ModelRunsDir(), "scenarists", scenaristTurnSegment(cfg))
}

func scenaristTurnSegment(cfg playConfig) string {
	turn := 1
	if cfg.State != nil && cfg.State.LatestTurn >= 0 {
		turn = cfg.State.LatestTurn + 1
	}
	return fmt.Sprintf("turn-%06d", turn)
}

func resolveWriterScenaristCount(value int, backend string, writerRefreshEnabled bool) int {
	if value >= 0 {
		if value > scenarist.MaxToolRequests {
			return scenarist.MaxToolRequests
		}
		return value
	}
	if writerRefreshEnabled && backend != "static" {
		return scenarist.MaxToolRequests
	}
	return 0
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func defaultScenaristProviderPool() string {
	if value := strings.TrimSpace(os.Getenv("SANS_NOM_SCENARIST_PROVIDERS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("SANS_NOM_SCENARIST_PROVIDER")); value != "" {
		return value
	}
	return strings.Join(scenarist.DefaultProviderNames(), ",")
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func decisionText(choiceID string, freeText string) string {
	if freeText != "" {
		return freeText
	}
	if choiceID != "" {
		return choiceID
	}
	return "observe"
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func openPacket(path string) (scene.StoryPacket, error) {
	file, err := os.Open(path)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	defer file.Close()
	return scene.DecodeStoryPacket(file)
}

func loadCardStore() (*contextbundle.LocalCardStore, error) {
	return contextbundle.LoadLocalCardStore(
		filepath.Join("lore", "actors"),
		filepath.Join("lore", "canon", "comics", "peer-cards", "cards", "accepted"),
	)
}

func actorRegistry(cardStore *contextbundle.LocalCardStore) (*actors.Registry, error) {
	if cardStore != nil {
		registry, err := cardStore.Registry()
		if err == nil && len(registry.IDs()) > 0 {
			return registry, nil
		}
	}
	return actors.NewRegistry([]actors.Actor{{
		ID:          "tonto",
		DisplayName: "Tonto",
		Card:        "TRAIT: Tonto tells long witness-bound stories and never becomes a codex.",
	}, {
		ID:          "aghora",
		DisplayName: "Aghora",
		Card:        "TRAIT: Aghora's precision is her love.",
	}})
}
