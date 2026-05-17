package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

func main() {
	loadDotEnv(".env")

	packetPath := flag.String("packet", "testdata/packets/toy.json", "StoryPacket JSON path")
	nodeID := flag.String("node", "", "node id to roll out; defaults to packet entry node")
	choiceID := flag.String("choice", "", "selected choice id")
	freeText := flag.String("text", "", "free-text player input")
	useClaude := flag.Bool("claude", false, "use claude -p Sonnet adapters instead of static local adapters")
	modelBackend := flag.String("model-backend", os.Getenv("SANS_NOM_MODEL_BACKEND"), "model backend: codex, claude, or static")
	refreshWriter := flag.Bool("refresh-writer", false, "run the Creative Writer asynchronously before rollout")
	seedStory := flag.Bool("seed-story", false, "generate the opening original StoryPacket")
	seedOutput := flag.String("seed-output", "", "optional path for -seed-story output")
	storyID := flag.String("story-id", "", "create or resume a durable story id")
	storyRoot := flag.String("story-root", story.DefaultRoot, "root directory for durable story state")
	globalLoreCollection := flag.String("global-lore-collection", story.DefaultGlobalLoreCollection, "global source-lore Qdrant collection")
	storyCollection := flag.String("story-collection", "", "story Qdrant collection; defaults from story id")
	replaceStory := flag.Bool("replace-story", false, "replace an existing durable story directory and seed collection")
	playMode := flag.Bool("play", false, "run a small CLI play loop")
	commitMode := flag.Bool("commit", false, "commit prepared scene canon to Honcho during -play")
	turns := flag.Int("turns", 0, "number of automated play turns; 0 reads interactively in -play")
	playTraceJSONL := flag.Bool("play-trace-jsonl", false, "emit machine-readable JSONL trace events during -play")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	contextPreview := flag.String("context-preview", "", "preview assembled context: narrator or writer")
	contextText := flag.Bool("context-text", false, "print model-ready context text instead of pretty JSON")
	flag.Parse()

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

	var packet scene.StoryPacket
	var bible *story.StoryBible
	var workspace *story.Workspace
	var state *story.StoryState
	if *seedStory {
		ws, st, bundle, err := seedStoryWorkspace(context.Background(), backend, resolvedBridgeURL, *storyID, *storyRoot, *globalLoreCollection, *storyCollection, *replaceStory, cardStore)
		if err != nil {
			log.Fatal(err)
		}
		workspace = &ws
		state = &st
		bible = &bundle.Bible
		packet = bundle.Packet
		if !*playMode {
			if err := printSeedBundle(bundle, *seedOutput); err != nil {
				log.Fatal(err)
			}
			return
		}
		if *nodeID == "" {
			*nodeID = st.CurrentNodeID
		}
	} else if *storyID != "" {
		ws, st, err := story.LoadWorkspace(*storyRoot, *storyID)
		if err != nil {
			log.Fatal(err)
		}
		loadedBible, err := ws.LoadBible()
		if err != nil {
			log.Fatal(err)
		}
		packet, err = ws.LoadPacket(st)
		if err != nil {
			log.Fatal(err)
		}
		workspace = &ws
		state = &st
		bible = &loadedBible
		if *nodeID == "" {
			*nodeID = st.CurrentNodeID
		}
	} else {
		packet, err = openPacket(*packetPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	decision := scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}
	if *playMode {
		if err := runPlay(context.Background(), playConfig{
			Packet:           packet,
			NodeID:           *nodeID,
			Backend:          backend,
			BridgeURL:        resolvedBridgeURL,
			CardStore:        cardStore,
			Workspace:        workspace,
			State:            state,
			Bible:            bible,
			GlobalCollection: *globalLoreCollection,
			StoryCollection:  resolvedStoryCollection(state, *storyCollection),
			InitialDecision:  decision,
			Turns:            *turns,
			Commit:           *commitMode,
			RefreshWrite:     *refreshWriter,
			TraceJSONL:       *playTraceJSONL,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *contextPreview != "" {
		if err := runContextPreview(context.Background(), *contextPreview, *contextText, *bridgeURL, packet, *nodeID, decision, bible, *globalLoreCollection, resolvedStoryCollection(state, *storyCollection), cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}
	runner := claudeRunner()
	if *refreshWriter {
		assembler := contextAssembler(resolvedBridgeURL, resolvedBridgeURL != "", cardStore, *globalLoreCollection, resolvedStoryCollection(state, *storyCollection))
		writerContext, err := assembler.AssembleWriter(context.Background(), contextbundle.BuildRequest{
			Packet:   packet,
			NodeID:   *nodeID,
			Decision: decision,
			Bible:    bible,
		})
		if err != nil {
			log.Fatal(err)
		}
		job, err := writer.Service{
			Planner: writerPlanner(backend, runner),
		}.RefreshAsync(context.Background(), writer.Input{
			Current:        packet,
			LatestDecision: decisionText(*choiceID, *freeText),
			ContextText:    writerContext.RenderText(),
		})
		if err != nil {
			log.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), writer.DefaultAwaitTimeout())
		defer cancel()
		result := job.Await(ctx)
		if result.Err != nil {
			log.Fatal(result.Err)
		}
		packet = result.Packet
	}
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

func seedStoryWorkspace(
	ctx context.Context,
	backend string,
	bridgeURL string,
	storyID string,
	storyRoot string,
	globalCollection string,
	storyCollection string,
	replace bool,
	cardStore *contextbundle.LocalCardStore,
) (story.Workspace, story.StoryState, story.SeedBundle, error) {
	bundle, err := generateSeedBundle(ctx, backend, bridgeURL, globalCollection, "", cardStore)
	if err != nil {
		return story.Workspace{}, story.StoryState{}, story.SeedBundle{}, err
	}
	ws, state, err := story.InitWorkspace(story.InitOptions{
		StoryID:              storyID,
		Root:                 storyRoot,
		GlobalLoreCollection: globalCollection,
		StoryCollection:      storyCollection,
		Replace:              replace,
	}, bundle)
	if err != nil {
		return story.Workspace{}, story.StoryState{}, story.SeedBundle{}, err
	}
	bundle.Bible.StoryID = state.StoryID
	bundle.Packet.ContextSnapshot.WorkspaceID = state.WorkspaceID
	bundle.Packet.ContextSnapshot.SessionID = state.SessionID
	if err := maybeSeedExternalStory(ctx, backend, bridgeURL, ws, state, bundle); err != nil {
		return story.Workspace{}, story.StoryState{}, story.SeedBundle{}, err
	}
	return ws, state, bundle, nil
}

func generateSeedBundle(ctx context.Context, backend string, bridgeURL string, globalCollection string, storyCollection string, cardStore *contextbundle.LocalCardStore) (story.SeedBundle, error) {
	contextText := seed.ContextText()
	if backend != "static" {
		assembler := contextAssembler(bridgeURL, false, cardStore, globalCollection, storyCollection)
		packet := seed.DefaultPacket()
		writerContext, err := assembler.AssembleWriter(ctx, contextbundle.BuildRequest{Packet: packet})
		if err == nil {
			contextText += "\n\nRuntime lore context:\n" + writerContext.RenderText()
		}
	}
	return seed.Service{
		Scenarist:     seedRunner(backend, true),
		Verifier:      seedRunner(backend, false),
		MaxIterations: 2,
	}.Generate(ctx, contextText)
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
	Packet           scene.StoryPacket
	NodeID           string
	Backend          string
	BridgeURL        string
	CardStore        *contextbundle.LocalCardStore
	Workspace        *story.Workspace
	State            *story.StoryState
	Bible            *story.StoryBible
	GlobalCollection string
	StoryCollection  string
	InitialDecision  scene.PlayerDecision
	Turns            int
	Commit           bool
	RefreshWrite     bool
	TraceJSONL       bool
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
			next, err := refreshPacket(ctx, packet, nodeID, nextDecision, cfg)
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
			fmt.Println("\nFree text needs -refresh-writer to create the next packet; play loop stopped.")
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
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection)
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
	job, err := writer.Service{Planner: writerPlanner(cfg.Backend, runner)}.RefreshAsync(ctx, writer.Input{
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

func writerPlanner(backend string, claude prompt.ClaudeRunner) writer.Planner {
	switch backend {
	case "static":
		return writer.StaticPlanner{}
	case "claude":
		return writer.ClaudePlanner{Runner: claude}
	default:
		return writer.ClaudePlanner{Runner: codexRunner("StoryPacket", prompt.StoryPacketSchema())}
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
		Timeout:     3 * time.Minute,
	}
}

func claudeRunner() prompt.ClaudeRunner {
	model := os.Getenv("SANS_NOM_MODEL")
	if model == "" {
		model = "sonnet"
	}
	effort := os.Getenv("SANS_NOM_MODEL_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	return prompt.ClaudeRunner{
		Model:   model,
		Effort:  effort,
		WorkDir: ".",
		Timeout: 3 * time.Minute,
	}
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
