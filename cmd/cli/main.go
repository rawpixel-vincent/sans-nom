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
	playMode := flag.Bool("play", false, "run a small CLI play loop")
	commitMode := flag.Bool("commit", false, "commit prepared scene canon to Honcho during -play")
	turns := flag.Int("turns", 0, "number of automated play turns; 0 reads interactively in -play")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	contextPreview := flag.String("context-preview", "", "preview assembled context: narrator or writer")
	contextText := flag.Bool("context-text", false, "print model-ready context text instead of pretty JSON")
	flag.Parse()

	if *loreSearch != "" {
		if err := runLoreSearch(context.Background(), *bridgeURL, *loreSearch, *loreK); err != nil {
			log.Fatal(err)
		}
		return
	}

	cardStore, err := loadCardStore()
	if err != nil {
		log.Fatal(err)
	}
	backend := resolveBackend(*modelBackend, *useClaude)

	if *seedStory && !*playMode {
		if err := runSeedStory(context.Background(), backend, *bridgeURL, *seedOutput, cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}

	var packet scene.StoryPacket
	if *seedStory {
		packet, err = generateSeedPacket(context.Background(), backend, *bridgeURL, cardStore)
	} else {
		packet, err = openPacket(*packetPath)
	}
	if err != nil {
		log.Fatal(err)
	}
	decision := scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}
	if *playMode {
		if err := runPlay(context.Background(), playConfig{
			Packet:       packet,
			NodeID:       *nodeID,
			Backend:      backend,
			BridgeURL:    *bridgeURL,
			CardStore:    cardStore,
			Turns:        *turns,
			Commit:       *commitMode,
			RefreshWrite: backend != "static",
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *contextPreview != "" {
		if err := runContextPreview(context.Background(), *contextPreview, *contextText, *bridgeURL, packet, *nodeID, decision, cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}
	runner := claudeRunner()
	if *refreshWriter {
		assembler := contextAssembler(*bridgeURL, *bridgeURL != "", cardStore)
		writerContext, err := assembler.AssembleWriter(context.Background(), contextbundle.BuildRequest{
			Packet:   packet,
			NodeID:   *nodeID,
			Decision: decision,
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
	assembler := contextAssembler(*bridgeURL, false, cardStore)
	narratorContext, err := assembler.AssembleNarrator(context.Background(), contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   *nodeID,
		Decision: decision,
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

func runLoreSearch(ctx context.Context, bridgeURL string, query string, k int) error {
	retriever := lore.BridgeRetriever{
		Client: bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN")),
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

func runSeedStory(ctx context.Context, backend string, bridgeURL string, outputPath string, cardStore *contextbundle.LocalCardStore) error {
	packet, err := generateSeedPacket(ctx, backend, bridgeURL, cardStore)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(packet, "", "  ")
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

func generateSeedPacket(ctx context.Context, backend string, bridgeURL string, cardStore *contextbundle.LocalCardStore) (scene.StoryPacket, error) {
	contextText := seed.ContextText()
	if bridgeURL != "" {
		assembler := contextAssembler(bridgeURL, false, cardStore)
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
			return codexRunner("StoryPacket", prompt.StoryPacketSchema())
		}
		return codexRunner("SeedReview", prompt.SeedReviewSchema())
	}
}

func runContextPreview(
	ctx context.Context,
	role string,
	asText bool,
	bridgeURL string,
	packet scene.StoryPacket,
	nodeID string,
	decision scene.PlayerDecision,
	cardStore *contextbundle.LocalCardStore,
) error {
	assembler := contextAssembler(bridgeURL, bridgeURL != "", cardStore)
	req := contextbundle.BuildRequest{Packet: packet, NodeID: nodeID, Decision: decision}
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
	Packet       scene.StoryPacket
	NodeID       string
	Backend      string
	BridgeURL    string
	CardStore    *contextbundle.LocalCardStore
	Turns        int
	Commit       bool
	RefreshWrite bool
}

func runPlay(ctx context.Context, cfg playConfig) error {
	packet := cfg.Packet
	nodeID := cfg.NodeID
	reader := bufio.NewReader(os.Stdin)
	totalTurns := cfg.Turns
	if totalTurns <= 0 {
		totalTurns = 1
	}
	for turn := 1; turn <= totalTurns; turn++ {
		rollout, err := rolloutOnce(ctx, packet, nodeID, scene.PlayerDecision{}, cfg)
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
		if turn == totalTurns {
			break
		}
		if len(rollout.Node.Choices) == 0 && !cfg.RefreshWrite {
			fmt.Println("\nNo choices available; play loop stopped.")
			break
		}
		decision := automatedDecision(rollout.Node)
		if cfg.Turns <= 0 {
			fmt.Print("\nchoice id or free text> ")
			line, _ := reader.ReadString('\n')
			decision = parseDecision(strings.TrimSpace(line), rollout.Node)
		}
		if cfg.RefreshWrite {
			next, err := refreshPacket(ctx, packet, nodeID, decision, cfg)
			if err == nil {
				packet = next
				nodeID = ""
				continue
			}
			fmt.Fprintf(os.Stderr, "writer refresh failed, continuing current packet: %v\n", err)
		}
		if choice, err := rollout.Node.Choice(decision.ChoiceID); err == nil && choice.TargetNodeID != "" {
			nodeID = choice.TargetNodeID
		} else if len(rollout.Node.Choices) == 0 {
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
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore)
	narratorContext, err := assembler.AssembleNarrator(ctx, contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   nodeID,
		Decision: decision,
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
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore)
	writerContext, err := assembler.AssembleWriter(ctx, contextbundle.BuildRequest{
		Packet:   packet,
		NodeID:   nodeID,
		Decision: decision,
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
	for _, choice := range node.Choices {
		if line == choice.ID {
			return scene.PlayerDecision{ChoiceID: line}
		}
	}
	return scene.PlayerDecision{FreeText: line}
}

func printPrettyJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func contextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore) contextbundle.Assembler {
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
		assembler.Lore = lore.BridgeRetriever{Client: client}
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
