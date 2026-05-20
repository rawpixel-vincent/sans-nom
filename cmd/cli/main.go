package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/admin"
	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/clitui"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
	ttsclient "github.com/rawpixel-vincent/sans-nom/internal/tts"
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
	readStory := flag.Bool("read-story", false, "print saved story narration and exit")
	seedOutput := flag.String("seed-output", "", "optional path for generated seed bundle output")
	storyID := flag.String("story-id", "", "create or resume a durable story id")
	storyRoot := flag.String("story-root", story.DefaultRoot, "root directory for durable story state")
	groundLorePackID := flag.String("ground-lore-pack", lorepack.DefaultPackID(), "ground lore pack used for new stories and source-lore search")
	storyCollection := flag.String("story-collection", "", "story Qdrant collection; defaults from story id")
	replaceStory := flag.Bool("replace-story", false, "replace an existing durable story directory and seed collection")
	writerScenaristCalls := flag.Int("writer-scenarist-calls", -1, "optional max ask_scenarist calls per Writer run; 0 disables; default is 24 for live runs")
	scenaristProvider := flag.String("scenarist-provider", defaultScenaristProviderPool(), "Claude-compatible scenarist provider pool, comma-separated, for example ZAI,MINIMAX")
	scenaristModel := flag.String("scenarist-model", os.Getenv("SANS_NOM_SCENARIST_MODEL"), "Claude-compatible scenarist model override")
	scenaristTools := flag.Bool("scenarist-tools", envBoolDefault("SANS_NOM_SCENARIST_TOOLS", true), "allow Scenarists read-only lore tools")
	scenaristOutputRoot := flag.String("scenarist-output-root", "", "optional root for Scenarist material output")
	writerScenaristRounds := flag.Int("writer-scenarist-rounds", envInt("SANS_NOM_WRITER_SCENARIST_ROUNDS", 24), "maximum Writer tool rounds while refreshing; 0 uses the 24-round safety cap")
	noPlay := flag.Bool("no-play", false, "do not enter the interactive play loop after loading or creating a story")
	noCommit := flag.Bool("no-commit", false, "do not commit witnessed scene canon to Honcho during play")
	turns := flag.Int("turns", 0, "number of automated play turns; 0 reads interactively")
	playTraceJSONL := flag.Bool("play-trace-jsonl", false, "emit machine-readable JSONL trace events during play")
	readTurnTTS := flag.Bool("tts", envBool("SANS_NOM_TTS"), "synthesize each narrator turn with Xiaomi MiMo v2.5 TTS")
	ttsBaseURL := flag.String("tts-base-url", firstNonEmpty(os.Getenv("SANS_NOM_TTS_BASE_URL"), ttsclient.DefaultBaseURL), "MiMo OpenAI-compatible TTS base URL")
	ttsModel := flag.String("tts-model", firstNonEmpty(os.Getenv("SANS_NOM_TTS_MODEL"), ttsclient.DefaultModel), "MiMo TTS model")
	ttsVoice := flag.String("tts-voice", firstNonEmpty(os.Getenv("SANS_NOM_TTS_VOICE"), ttsclient.DefaultVoice), "MiMo TTS voice id")
	ttsFormat := flag.String("tts-format", firstNonEmpty(os.Getenv("SANS_NOM_TTS_FORMAT"), ttsclient.DefaultAudioFormat), "TTS output format: wav or pcm16")
	ttsInstruction := flag.String("tts-instruction", firstNonEmpty(os.Getenv("SANS_NOM_TTS_INSTRUCTION"), ttsclient.DefaultNarratorInstruction), "MiMo TTS style/director instruction; spoken text stays in the assistant message")
	ttsOutputDir := flag.String("tts-output-dir", os.Getenv("SANS_NOM_TTS_OUTPUT_DIR"), "optional TTS output directory; defaults to the story audio directory")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	adminMode := flag.Bool("admin", false, "start the local admin web UI and do not enter the game loop")
	adminAddr := flag.String("admin-addr", "127.0.0.1:8790", "local admin web UI listen address")
	contextPreview := flag.String("context-preview", "", "preview assembled context: narrator or writer")
	contextText := flag.Bool("context-text", false, "print model-ready context text instead of pretty JSON")
	flag.Parse()
	packetExplicit := flagPassed("packet")
	groundPack, err := lorepack.GetPack(repoRoot(), *groundLorePackID)
	if err != nil {
		log.Fatal(err)
	}
	groundLoreCollection := groundPack.Collection
	backend := resolveBackend(*modelBackend, *useClaude)
	resolvedBridgeURL := runtimeBridgeURL(backend, *bridgeURL)

	if *loreSearch != "" {
		loreBridgeURL := firstNonEmpty(resolvedBridgeURL, bridgeclient.NewClient("", "").BaseURL)
		stopBridge, err := ensureBridge(context.Background(), "codex", loreBridgeURL)
		if err != nil {
			log.Fatal(err)
		}
		defer stopBridge()
		if err := runLoreSearch(context.Background(), loreBridgeURL, groundLoreCollection, *loreSearch, *loreK); err != nil {
			log.Fatal(err)
		}
		return
	}

	cardStore, err := loadCardStore()
	if err != nil {
		log.Fatal(err)
	}
	if *adminMode {
		if err := runAdmin(context.Background(), adminConfig{
			Addr:                 *adminAddr,
			StoryRoot:            *storyRoot,
			BridgeURL:            resolvedBridgeURL,
			Backend:              backend,
			CardStore:            cardStore,
			GroundLorePackID:     groundPack.ID,
			GlobalCollection:     groundLoreCollection,
			StoryCollection:      *storyCollection,
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			WriterScenaristCalls: resolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *readStory {
		if strings.TrimSpace(*storyID) == "" {
			log.Fatal("-read-story requires -story-id")
		}
		workspace, _, err := story.LoadWorkspace(*storyRoot, *storyID)
		if err != nil {
			log.Fatal(err)
		}
		if err := printStoryNarration(workspace); err != nil {
			log.Fatal(err)
		}
		return
	}
	playMode := !*noPlay && *contextPreview == ""
	commitMode := !*noCommit
	playRefreshWriter := !*noWriterRefresh
	if shouldRunLauncher(noArgs, passedFlagNames()) {
		ttsCfg := playTTSConfigFromFlags(*readTurnTTS, *ttsBaseURL, *ttsModel, *ttsVoice, *ttsFormat, *ttsInstruction, *ttsOutputDir)
		if err := runLauncher(context.Background(), launcherConfig{
			Backend:              backend,
			BridgeURL:            resolvedBridgeURL,
			CardStore:            cardStore,
			StoryRoot:            *storyRoot,
			GroundLorePackID:     groundPack.ID,
			GlobalCollection:     groundLoreCollection,
			StoryCollection:      *storyCollection,
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			RefreshWrite:         playRefreshWriter,
			Commit:               commitMode,
			WriterScenaristCalls: resolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
			TTS:                  ttsCfg,
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
		session, err := newStorySessionWithDisconnectRetry(context.Background(), storyapp.StoryActionOptions{
			StoryID:              *storyID,
			StoryRoot:            *storyRoot,
			GroundLorePackID:     groundPack.ID,
			GroundLoreCollection: groundLoreCollection,
			StoryCollection:      *storyCollection,
			ReplaceStory:         *replaceStory,
		}, seedRuntimeConfig{
			Backend:              backend,
			BridgeURL:            resolvedBridgeURL,
			GroundLorePackID:     groundPack.ID,
			WriterScenaristCalls: resolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			InteractiveRetry:     shouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL),
		}, shouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL))
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
		ttsCfg := playTTSConfigFromFlags(*readTurnTTS, *ttsBaseURL, *ttsModel, *ttsVoice, *ttsFormat, *ttsInstruction, *ttsOutputDir)
		if err := runPlay(context.Background(), playConfig{
			Packet:               packet,
			NodeID:               *nodeID,
			Backend:              backend,
			BridgeURL:            resolvedBridgeURL,
			CardStore:            cardStore,
			Workspace:            workspace,
			State:                state,
			Bible:                bible,
			GroundLorePackID:     sourceLorePackID(state, groundPack.ID),
			GlobalCollection:     sourceLoreCollection(state, groundLoreCollection),
			StoryCollection:      resolvedStoryCollection(state, *storyCollection),
			InitialDecision:      decision,
			Turns:                *turns,
			Commit:               commitMode,
			RefreshWrite:         playRefreshWriter,
			TraceJSONL:           *playTraceJSONL,
			UseTUI:               shouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL),
			WriterScenaristCalls: resolveWriterScenaristCallCount(*writerScenaristCalls, backend, playRefreshWriter),
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			Logger:               storyLogger(workspace, state),
			TTS:                  ttsCfg,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *contextPreview != "" {
		if err := runContextPreview(context.Background(), *contextPreview, *contextText, resolvedBridgeURL, packet, *nodeID, decision, bible, sourceLoreCollection(state, groundLoreCollection), resolvedStoryCollection(state, *storyCollection), cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}
	runner := claudeRunner()
	registry, err := actorRegistry(cardStore)
	if err != nil {
		log.Fatal(err)
	}
	assembler := contextAssembler(resolvedBridgeURL, false, cardStore, sourceLoreCollection(state, groundLoreCollection), resolvedStoryCollection(state, *storyCollection), nil)
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
		scope := agentfs.New(filepath.Dir(absOrClean(*packetPath)), "narrator").WithVisiblePaths(".")
		renderer = narrator.ClaudeRenderer{Runner: codexRunnerWithReadTools("NarratorDraft", prompt.NarratorDraftSchema(), scope, nil, "narrator", "narrator", "Narrator", 0, packet.ID, *nodeID)}
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

func printStoryNarration(workspace story.Workspace) error {
	files, err := workspace.LoadNarrationFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Printf("No narration files found in %s\n", filepath.ToSlash(workspace.NarrationDir()))
		return nil
	}
	for i, file := range files {
		if i > 0 {
			fmt.Print("\n---\n\n")
		}
		fmt.Print(stripNarrationMetadata(file.Text))
		fmt.Println()
	}
	return nil
}

func stripNarrationMetadata(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "# ") {
		if idx := strings.Index(text, "\n\n"); idx >= 0 {
			text = strings.TrimSpace(text[idx+2:])
		}
	}
	if strings.HasPrefix(text, "<!--") {
		if idx := strings.Index(text, "-->"); idx >= 0 {
			text = strings.TrimSpace(text[idx+3:])
		}
	}
	return text
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
	Backend              string
	BridgeURL            string
	CardStore            *contextbundle.LocalCardStore
	StoryRoot            string
	GroundLorePackID     string
	GlobalCollection     string
	StoryCollection      string
	WriterScenaristCalls int
	ScenaristProvider    string
	ScenaristModel       string
	ScenaristTools       bool
	ScenaristOutputRoot  string
	ScenaristRounds      int
	RefreshWrite         bool
	Commit               bool
	TTS                  playTTSConfig
}

type adminConfig struct {
	Addr                 string
	StoryRoot            string
	BridgeURL            string
	Backend              string
	CardStore            *contextbundle.LocalCardStore
	GroundLorePackID     string
	GlobalCollection     string
	StoryCollection      string
	WriterScenaristCalls int
	ScenaristProvider    string
	ScenaristModel       string
	ScenaristTools       bool
	ScenaristOutputRoot  string
	ScenaristRounds      int
}

type seedRuntimeConfig struct {
	Backend              string
	BridgeURL            string
	GroundLorePackID     string
	WriterScenaristCalls int
	ScenaristProvider    string
	ScenaristModel       string
	ScenaristTools       bool
	ScenaristOutputRoot  string
	ScenaristRounds      int
	InteractiveRetry     bool
}

type playTTSConfig struct {
	Enabled     bool
	BaseURL     string
	Model       string
	Voice       string
	Format      string
	Instruction string
	OutputDir   string
	APIKey      string
	Timeout     time.Duration
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

	action, err := clitui.Run(ctx, cfg.StoryRoot, lorepack.ListPacks)
	if err != nil {
		return err
	}
	switch action.Kind {
	case clitui.ActionNewStory:
		packID := firstNonEmpty(action.GroundLorePackID, cfg.GroundLorePackID, lorepack.DefaultPackID())
		pack, err := lorepack.GetPack(repoRoot(), packID)
		if err != nil {
			return err
		}
		groundCollection := cfg.GlobalCollection
		if groundCollection == "" || pack.ID != cfg.GroundLorePackID {
			groundCollection = pack.Collection
		}
		session, err := newStorySessionWithDisconnectRetry(ctx, storyapp.StoryActionOptions{
			StoryRoot:            cfg.StoryRoot,
			GroundLorePackID:     pack.ID,
			GroundLoreCollection: groundCollection,
			StoryCollection:      cfg.StoryCollection,
		}, seedRuntimeConfig{
			Backend:              cfg.Backend,
			BridgeURL:            cfg.BridgeURL,
			GroundLorePackID:     pack.ID,
			WriterScenaristCalls: cfg.WriterScenaristCalls,
			ScenaristProvider:    cfg.ScenaristProvider,
			ScenaristModel:       cfg.ScenaristModel,
			ScenaristTools:       cfg.ScenaristTools,
			ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
			ScenaristRounds:      cfg.ScenaristRounds,
			InteractiveRetry:     interactiveStdin(),
		}, interactiveStdin())
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

func newStorySession(ctx context.Context, opts storyapp.StoryActionOptions, cfg seedRuntimeConfig) (storyapp.StorySession, error) {
	globalLogger := gamelog.NewGlobalWriter(opts.StoryRoot)
	logEvent(globalLogger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.started",
		Message:  "story seed generation started",
		Fields: map[string]any{
			"backend":                cfg.Backend,
			"ground_lore_pack_id":    opts.GroundLorePackID,
			"ground_lore_collection": firstNonEmpty(opts.GroundLoreCollection, opts.GlobalLoreCollection, story.DefaultGroundLoreCollection),
			"story_collection":       opts.StoryCollection,
			"replace_story":          opts.ReplaceStory,
			"writer_scenarist_calls": cfg.WriterScenaristCalls,
		},
		Detail: map[string]any{
			"story_action_options": map[string]any{
				"story_id":               opts.StoryID,
				"story_root":             opts.StoryRoot,
				"ground_lore_pack_id":    opts.GroundLorePackID,
				"ground_lore_collection": opts.GroundLoreCollection,
				"global_lore_collection": opts.GlobalLoreCollection,
				"story_collection":       opts.StoryCollection,
				"replace_story":          opts.ReplaceStory,
			},
			"bridge_url":   cfg.BridgeURL,
			"seed_runtime": cfg,
		},
	})
	opts.GenerateSeedBundle = func(ctx context.Context, action storyapp.StoryActionOptions) (story.SeedBundle, error) {
		return generateSeedBundle(ctx, cfg, action.StoryRoot, action.GroundLorePackID, firstNonEmpty(action.GroundLoreCollection, action.GlobalLoreCollection, story.DefaultGroundLoreCollection), globalLogger)
	}
	opts.SeedExternal = func(ctx context.Context, session storyapp.StorySession, bundle story.SeedBundle) error {
		logger := storyLogger(&session.Workspace, &session.State)
		err := maybeSeedExternalStory(ctx, cfg.Backend, cfg.BridgeURL, session.Workspace, session.State, bundle, logger)
		if err != nil {
			logEvent(globalLogger, gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "story",
				Event:    "story.seed.external.failed",
				StoryID:  session.State.StoryID,
				Message:  err.Error(),
			})
		}
		return err
	}
	session, err := storyapp.NewStory(ctx, opts)
	if err != nil {
		logEvent(globalLogger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "story",
			Event:    "story.seed.failed",
			Message:  err.Error(),
			Detail: map[string]any{
				"error":        err.Error(),
				"story_root":   opts.StoryRoot,
				"bridge_url":   cfg.BridgeURL,
				"seed_runtime": cfg,
			},
		})
		return storyapp.StorySession{}, err
	}
	logger := storyLogger(&session.Workspace, &session.State)
	logEvent(globalLogger, gamelog.Event{
		Category: "story",
		Event:    "story.created",
		StoryID:  session.State.StoryID,
		Message:  "story workspace created",
		PacketID: session.Packet.ID,
		NodeID:   session.NodeID,
		Fields: map[string]any{
			"title":                  session.Bible.Title,
			"backend":                cfg.Backend,
			"story_id":               session.State.StoryID,
			"ground_lore_pack_id":    session.State.SourceLorePackID(),
			"ground_lore_collection": session.State.SourceLoreCollection(),
			"story_collection":       session.State.StoryCollection,
		},
		Detail: map[string]any{
			"state":  session.State,
			"bible":  session.Bible,
			"packet": session.Packet,
		},
	})
	logEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.generated",
		Message:  "story bible and opening packet generated",
		PacketID: session.Packet.ID,
		NodeID:   session.NodeID,
		Fields: map[string]any{
			"title":                session.Bible.Title,
			"backend":              cfg.Backend,
			"lore_principle_count": len(session.Bible.BindingLorePrinciples()),
			"node_count":           len(session.Packet.Nodes),
		},
	})
	logEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.created",
		Message:  "story workspace created",
		PacketID: session.Packet.ID,
		NodeID:   session.NodeID,
		Fields: map[string]any{
			"title":                  session.Bible.Title,
			"backend":                cfg.Backend,
			"ground_lore_pack_id":    session.State.SourceLorePackID(),
			"ground_lore_collection": session.State.SourceLoreCollection(),
			"story_collection":       session.State.StoryCollection,
		},
	})
	return session, nil
}

func newStorySessionWithDisconnectRetry(ctx context.Context, opts storyapp.StoryActionOptions, cfg seedRuntimeConfig, interactive bool) (storyapp.StorySession, error) {
	for {
		session, err := newStorySession(ctx, opts, cfg)
		if err == nil {
			return session, nil
		}
		if !waitForDisconnectRetry(ctx, "story seed", err, interactive) {
			return storyapp.StorySession{}, err
		}
	}
}

func runAdmin(ctx context.Context, cfg adminConfig) error {
	stopBridge, err := ensureBridge(ctx, cfg.Backend, cfg.BridgeURL)
	if err != nil {
		return err
	}
	defer stopBridge()
	bridge := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
	server := admin.NewServer(cfg.StoryRoot, bridge, filepath.Join("admin", "dist"), admin.Callbacks{
		RefreshWriter: func(ctx context.Context, storyID string) error {
			session, err := storyapp.ResumeStory(ctx, storyID, storyapp.StoryActionOptions{StoryRoot: cfg.StoryRoot})
			if err != nil {
				return err
			}
			workspace := session.Workspace
			state := session.State
			bible := session.Bible
			refreshCfg := playConfig{
				Packet:               session.Packet,
				NodeID:               session.NodeID,
				Backend:              cfg.Backend,
				BridgeURL:            cfg.BridgeURL,
				CardStore:            cfg.CardStore,
				Workspace:            &workspace,
				State:                &state,
				Bible:                &bible,
				GroundLorePackID:     state.SourceLorePackID(),
				GlobalCollection:     sourceLoreCollection(&state, cfg.GlobalCollection),
				StoryCollection:      resolvedStoryCollection(&state, cfg.StoryCollection),
				InitialDecision:      decisionForWriterRefresh(&state),
				RefreshWrite:         true,
				WriterScenaristCalls: cfg.WriterScenaristCalls,
				ScenaristProvider:    cfg.ScenaristProvider,
				ScenaristModel:       cfg.ScenaristModel,
				ScenaristTools:       cfg.ScenaristTools,
				ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
				ScenaristRounds:      cfg.ScenaristRounds,
				Logger:               storyLogger(&workspace, &state),
			}
			next, err := refreshPacket(ctx, session.Packet, session.NodeID, decisionForWriterRefresh(&state), refreshCfg)
			if err != nil {
				if failure, ok := asWriterFailure(err); ok {
					_ = markStoryBlocked(refreshCfg, &state, failure)
				}
				return err
			}
			return saveCurrentStoryPacket(ctx, refreshCfg, &state, next, entryNodeID(next))
		},
		NewStory: func(ctx context.Context, packID string) (storyapp.StorySession, error) {
			pack, err := lorepack.GetPack(repoRoot(), firstNonEmpty(packID, cfg.GroundLorePackID, lorepack.DefaultPackID()))
			if err != nil {
				return storyapp.StorySession{}, err
			}
			return newStorySession(ctx, storyapp.StoryActionOptions{
				StoryRoot:            cfg.StoryRoot,
				GroundLorePackID:     pack.ID,
				GroundLoreCollection: firstNonEmpty(pack.Collection, cfg.GlobalCollection),
				StoryCollection:      cfg.StoryCollection,
			}, seedRuntimeConfig{
				Backend:              cfg.Backend,
				BridgeURL:            cfg.BridgeURL,
				GroundLorePackID:     pack.ID,
				WriterScenaristCalls: cfg.WriterScenaristCalls,
				ScenaristProvider:    cfg.ScenaristProvider,
				ScenaristModel:       cfg.ScenaristModel,
				ScenaristTools:       cfg.ScenaristTools,
				ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
				ScenaristRounds:      cfg.ScenaristRounds,
			})
		},
	})
	fmt.Printf("Admin UI: http://%s/admin\n", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, server)
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
	return playConfig{
		Packet:               session.Packet,
		NodeID:               session.NodeID,
		Backend:              cfg.Backend,
		BridgeURL:            cfg.BridgeURL,
		CardStore:            cfg.CardStore,
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		GroundLorePackID:     state.SourceLorePackID(),
		GlobalCollection:     sourceLoreCollection(&state, cfg.GlobalCollection),
		StoryCollection:      resolvedStoryCollection(&state, cfg.StoryCollection),
		Commit:               cfg.Commit,
		RefreshWrite:         cfg.RefreshWrite,
		UseTUI:               shouldUsePlayTUI(0, "", "", false),
		WriterScenaristCalls: cfg.WriterScenaristCalls,
		ScenaristProvider:    cfg.ScenaristProvider,
		ScenaristModel:       cfg.ScenaristModel,
		ScenaristTools:       cfg.ScenaristTools,
		ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
		ScenaristRounds:      cfg.ScenaristRounds,
		Logger:               storyLogger(&workspace, &state),
		TTS:                  cfg.TTS,
	}
}

func generateSeedBundle(ctx context.Context, cfg seedRuntimeConfig, storyRoot string, groundPackID string, globalCollection string, logger *gamelog.Writer) (story.SeedBundle, error) {
	pack, err := lorepack.GetPack(repoRoot(), firstNonEmpty(groundPackID, cfg.GroundLorePackID, lorepack.DefaultPackID()))
	if err != nil {
		return story.SeedBundle{}, err
	}
	if strings.TrimSpace(globalCollection) == "" {
		globalCollection = pack.Collection
	}
	logEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.context.started",
		Message:  "building seed context",
		Fields: map[string]any{
			"backend":                cfg.Backend,
			"ground_lore_pack_id":    pack.ID,
			"ground_lore_pack":       pack.Label,
			"ground_lore_collection": globalCollection,
		},
	})
	contextText := buildSeedContextForPack(pack)
	scenaristOutputDir := seedScenaristOutputDir(storyRoot, cfg.ScenaristOutputRoot)
	agentScope := seedAgentScopeFromScenaristOutputDir(scenaristOutputDir)
	agentScope, err = stageLorePackForAgent(agentScope, pack)
	if err != nil {
		return story.SeedBundle{}, err
	}
	loreExpertTool := seedLoreExpertToolRunner(cfg, pack, globalCollection, logger)
	var scenaristTool *scenarist.ToolRunner
	var workshopRunner prompt.Runner
	workshopOutputDir := ""
	if !faithfulSeedMode(pack) {
		workshopOutputDir = seedWorkshopOutputDir(scenaristOutputDir)
		scenaristTool, err = seedScenaristToolRunner(cfg, contextText, scenaristOutputDir, agentScope, logger, loreExpertTool)
		if err != nil {
			return story.SeedBundle{}, err
		}
		workshopRunner, err = seedWorkshopRunner(cfg, agentScope, scenaristTool, loreExpertTool, logger)
		if err != nil {
			return story.SeedBundle{}, err
		}
	}
	writerRunner, err := seedWriterRunner(cfg, agentScope, scenaristTool, loreExpertTool, logger)
	if err != nil {
		return story.SeedBundle{}, err
	}
	logEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.context.completed",
		Message:  "seed context ready",
		Fields: map[string]any{
			"backend":                   cfg.Backend,
			"collection":                globalCollection,
			"ground_lore_pack_id":       pack.ID,
			"context_chars":             len(contextText),
			"writer_scenarist_calls":    cfg.WriterScenaristCalls,
			"scenarist_output_dir":      filepath.ToSlash(scenaristOutputDir),
			"workshop_output_dir":       filepath.ToSlash(workshopOutputDir),
			"faithful_seed_mode":        faithfulSeedMode(pack),
			"agent_workspace_root":      filepath.ToSlash(agentScope.Root),
			"writer_tools_attached":     writerRunnerHasTools(writerRunner),
			"scenarist_tool_attached":   scenaristTool != nil,
			"lore_expert_tool_attached": loreExpertTool != nil,
		},
		Detail: map[string]any{
			"context":              contextText,
			"seed_runtime":         cfg,
			"ground_lore_pack":     pack,
			"scenarist_output_dir": filepath.ToSlash(scenaristOutputDir),
			"workshop_output_dir":  filepath.ToSlash(workshopOutputDir),
			"agent_workspace_root": filepath.ToSlash(agentScope.Root),
			"visible_paths":        agentScope.VisiblePaths,
		},
	})
	return seed.Service{
		Writer:          writerRunner,
		WorkshopWriter:  workshopRunner,
		Verifier:        seedRunner(cfg.Backend, false),
		MaxIterations:   2,
		Logger:          logger,
		WorkshopDir:     workshopOutputDir,
		AgentRoot:       agentScope.Root,
		RecoverRunError: seedRunErrorRecoverer(ctx, cfg),
	}.Generate(ctx, contextText)
}

func seedRunErrorRecoverer(ctx context.Context, cfg seedRuntimeConfig) func(string, error) bool {
	if !cfg.InteractiveRetry {
		return nil
	}
	return func(stage string, err error) bool {
		return waitForDisconnectRetry(ctx, stage, err, true)
	}
}

func seedLoreRetriever(bridgeURL string, globalCollection string) lore.Retriever {
	if globalCollection == "" {
		globalCollection = story.DefaultGroundLoreCollection
	}
	return lore.BridgeRetriever{
		Client:     bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN")),
		Collection: globalCollection,
	}
}

func buildSeedContext(ctx context.Context, retriever lore.Retriever) string {
	_ = ctx
	_ = retriever
	pack, err := lorepack.GetPack(repoRoot(), lorepack.DefaultPackID())
	if err != nil {
		return buildSeedContextForPack(lorepack.Pack{
			ID:          story.DefaultGroundLorePackID,
			Label:       "Metabarons",
			Description: "Default lorepack for Sans-Nom stories.",
		})
	}
	return buildSeedContextForPack(pack)
}

func buildSeedContextForPack(pack lorepack.Pack) string {
	var b strings.Builder
	b.WriteString(seed.ContextText())
	if scenario := pack.ScenarioOrientationText(); scenario != "" {
		b.WriteString("\n\n")
		b.WriteString(scenario)
		b.WriteString("\n\nSeed fidelity instructions:\n")
		b.WriteString("- Treat the lorepack scenario contract as the source of truth for premise logic, protagonist pressure, opening pressure, and choice logic.\n")
		b.WriteString("- Read lorepack/lore-pack.json when local read tools are available, then stage and structure that authored scenario for playable opening turns.\n")
		b.WriteString("- Keep the contract's pressure alive through scene choices without copying its synopsis wording into every node.\n")
		b.WriteString("- Use ask_lore_expert as a lore authority/reference check when fidelity is uncertain.\n")
		b.WriteString("- Preserve the contract's must_honor items, avoid must_not_violate drift, and make choices express its choice_principles.\n")
		return b.String()
	}
	b.WriteString("\n\nLegacy ground-lore influence summary:\n")
	if pack.ID != "" || pack.Label != "" {
		fmt.Fprintf(&b, "- selected lore pack: %s (%s)\n", firstNonEmpty(pack.Label, pack.ID), firstNonEmpty(pack.ID, "unknown"))
	}
	b.WriteString("- pack note: legacy pack selected for story pressure; concrete source analysis comes through tools when needed.\n")
	b.WriteString("- influence style: heightened speculative adventure; intense family and function pressure; refusal under expectation; spectacle; material cost; credible consequence inside the protagonist's lived order.\n")
	b.WriteString("- binding doctrine: include an original deep rule as support for the player character's conflict; it should sharpen desire, opposition, and cost rather than dominate the premise.\n")
	b.WriteString("- reader plausibility: strange material needs internal cause, social practice, emotional credibility, visible cost, and consequence.\n")
	b.WriteString("- material imagination: choose the story's dominant condition from its own premise, such as infrastructure, ecology, transport, weather, resource scarcity, labor, workplace tools, maintenance routines, money pressure, family pressure, or local trade.\n")
	b.WriteString("- creative workshop: use ask_scenarist as the preferred room for story heat, suggestions, critique, story seed risks, character contradictions, scene engines, choice pressure, reversals, and reader criticism.\n")
	b.WriteString("- scenarist sequence: ask one clear advisory question, read the returned file, then ask any dependent follow-up with that path in relevant_paths.\n")
	b.WriteString("- source reference: use ask_lore_expert when a question depends on concrete patterns from the selected lorepack, such as source-pattern analysis, tone lineage, relationship pressure, or contradiction checks.\n")
	b.WriteString("- practical sequence: ask Scenarists first when exploring what this generated story could become; ask LoreExpert when a specific source-pack pattern needs grounding.\n")
	b.WriteString("- concrete source analysis comes through cached ask_lore_expert calls when needed.\n")
	return b.String()
}

func faithfulSeedMode(pack lorepack.Pack) bool {
	return pack.GameplayMode == lorepack.GameplayModeFaithfulCanonPrototype
}

func scenarioContractTextForPack(packID string) string {
	pack, err := lorepack.GetPack(repoRoot(), firstNonEmpty(packID, lorepack.DefaultPackID()))
	if err != nil {
		return ""
	}
	return pack.ScenarioContractText()
}

func seedWriterRunner(cfg seedRuntimeConfig, scope agentfs.Scope, _ *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedCreativeRunner(cfg, "SeedBundle", prompt.SeedBundleSchema(), scope, nil, loreExpertTool, logger)
}

func seedWorkshopRunner(cfg seedRuntimeConfig, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedCreativeRunner(cfg, "", nil, scope, toolRunner, loreExpertTool, logger)
}

func seedCreativeRunner(cfg seedRuntimeConfig, schemaName string, schema map[string]any, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	if cfg.Backend == "static" {
		return nil, nil
	}
	if (toolRunner != nil || loreExpertTool != nil) && cfg.Backend == "claude" {
		return nil, fmt.Errorf("Writer local tools are only supported for codex backend; use -model-backend codex or disable writer tools")
	}
	switch cfg.Backend {
	case "claude":
		return claudeRunner(), nil
	default:
		if strings.TrimSpace(scope.Root) != "" {
			if err := scope.Ensure(); err != nil {
				return nil, err
			}
		}
		runner := codexRunnerWithMaterialAccess(schemaName, schema, scope, toolRunner, loreExpertTool, cfg.ScenaristRounds)
		return configureCodexToolLogging(runner, logger, "writer", "creative-writer", "Creative Writer", "", 0, "", "", ""), nil
	}
}

func seedScenaristToolRunner(cfg seedRuntimeConfig, contextText string, outputDir string, scope agentfs.Scope, logger *gamelog.Writer, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	if cfg.WriterScenaristCalls == 0 || cfg.Backend == "static" {
		return nil, nil
	}
	tool, err := scenaristToolRunner(playConfig{
		WriterScenaristCalls: cfg.WriterScenaristCalls,
		GroundLorePackID:     cfg.GroundLorePackID,
		ScenaristProvider:    cfg.ScenaristProvider,
		ScenaristModel:       cfg.ScenaristModel,
		ScenaristTools:       cfg.ScenaristTools,
		ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
		Logger:               logger,
	}, contextText, outputDir, scope, loreExpertTool)
	if tool != nil {
		tool.CallerRole = "creative-writer"
		tool.CallerName = "Creative Writer"
	}
	return tool, err
}

func seedLoreExpertToolRunner(cfg seedRuntimeConfig, pack lorepack.Pack, collection string, logger *gamelog.Writer) *loreexpert.ToolRunner {
	if cfg.Backend == "static" || cfg.Backend == "claude" {
		return nil
	}
	if strings.TrimSpace(collection) == "" {
		collection = pack.Collection
	}
	tool := loreExpertToolRunner(playConfig{
		Backend:          cfg.Backend,
		BridgeURL:        cfg.BridgeURL,
		GroundLorePackID: pack.ID,
		GlobalCollection: collection,
		Logger:           logger,
	})
	if tool != nil {
		tool.CallerRole = "creative-writer"
		tool.CallerSpeaker = "Creative Writer"
	}
	return tool
}

func seedScenaristOutputDir(storyRoot string, outputRoot string) string {
	segment := "seed-" + strings.NewReplacer(".", "", ":", "", "-", "").Replace(time.Now().UTC().Format(time.RFC3339Nano))
	root := strings.TrimSpace(outputRoot)
	if root != "" {
		return absOrClean(filepath.Join(root, "seed-runs", segment, "agent", "scenarists"))
	}
	if strings.TrimSpace(storyRoot) == "" {
		storyRoot = story.DefaultRoot
	}
	return absOrClean(filepath.Join(filepath.Dir(filepath.Clean(storyRoot)), "seed-runs", segment, "agent", "scenarists"))
}

func seedWorkshopOutputDir(scenaristOutputDir string) string {
	return filepath.Join(filepath.Dir(scenaristOutputDir), "workshop")
}

func seedAgentScopeFromScenaristOutputDir(scenaristOutputDir string) agentfs.Scope {
	root := filepath.Dir(scenaristOutputDir)
	seedID := filepath.Base(filepath.Dir(root))
	return agentfs.New(root, "creative-writer").
		WithSeedRunID(seedID)
}

func stageLorePackForAgent(scope agentfs.Scope, pack lorepack.Pack) (agentfs.Scope, error) {
	if strings.TrimSpace(scope.Root) == "" || strings.TrimSpace(pack.Path) == "" {
		return scope, nil
	}
	if err := scope.Ensure(); err != nil {
		return scope, err
	}
	if _, err := scope.StageDir(filepath.Dir(pack.Path), "lorepack"); err != nil {
		return scope, fmt.Errorf("stage active lorepack %q: %w", pack.ID, err)
	}
	return scope.WithVisiblePaths("lorepack"), nil
}

func stageLorePackForAgentByID(scope agentfs.Scope, packID string) (agentfs.Scope, error) {
	pack, err := lorepack.GetPack(repoRoot(), firstNonEmpty(packID, lorepack.DefaultPackID()))
	if err != nil {
		return scope, err
	}
	return stageLorePackForAgent(scope, pack)
}

func writerRunnerHasTools(runner prompt.Runner) bool {
	codex, ok := runner.(prompt.CodexRunner)
	return ok && codex.LocalTools && len(codex.ExtraTools) > 0
}

func seedRunner(backend string, seedBundle bool) prompt.Runner {
	switch backend {
	case "static":
		return nil
	case "claude":
		return claudeRunner()
	default:
		if seedBundle {
			return codexRunner("SeedBundle", prompt.SeedBundleSchema())
		}
		return seedVerifierCodexRunner()
	}
}

func seedVerifierCodexRunner() prompt.CodexRunner {
	runner := codexRunner("SeedReview", prompt.SeedReviewSchema())
	if model := strings.TrimSpace(os.Getenv("SANS_NOM_VERIFIER_CODEX_MODEL")); model != "" {
		runner.Model = model
	} else {
		runner.Model = "gpt-5.5"
	}
	if effort := strings.TrimSpace(os.Getenv("SANS_NOM_VERIFIER_CODEX_EFFORT")); effort != "" {
		runner.Effort = effort
	} else {
		runner.Effort = "xhigh"
	}
	return runner
}

func maybeSeedExternalStory(ctx context.Context, backend string, bridgeURL string, ws story.Workspace, state story.StoryState, bundle story.SeedBundle, logger *gamelog.Writer) error {
	if backend == "static" {
		return nil
	}
	client := bridgeclient.NewClient(bridgeURL, os.Getenv("SIDECAR_TOKEN"))
	docs, err := ws.ReadIndexDocuments()
	if err != nil {
		return err
	}
	ingestReq := bridgeclient.StoryIndexIngestRequest{
		Collection: state.StoryCollection,
		Documents:  bridgeStoryDocs(docs),
		Replace:    true,
	}
	logEvent(logger, gamelog.Event{
		Category: "qdrant",
		Event:    "story.seed.index.request",
		Message:  "seed story index ingest request prepared",
		Fields: map[string]any{
			"collection":     state.StoryCollection,
			"document_count": len(docs),
			"replace":        true,
		},
		Detail: map[string]any{
			"request": ingestReq,
		},
	})
	ingestResp, err := client.IngestStoryIndex(ctx, ingestReq)
	if err != nil {
		logEvent(logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "qdrant",
			Event:    "story.seed.index.failed",
			Message:  err.Error(),
			Fields:   map[string]any{"collection": state.StoryCollection},
			Detail: map[string]any{
				"request": ingestReq,
				"error":   err.Error(),
			},
		})
		return fmt.Errorf("seed story collection %q: %w", state.StoryCollection, err)
	}
	logEvent(logger, gamelog.Event{
		Category: "qdrant",
		Event:    "story.seed.index.ingested",
		Message:  "seed story index ingested",
		Fields:   map[string]any{"collection": state.StoryCollection, "document_count": len(docs), "response_count": ingestResp.Count},
		Detail: map[string]any{
			"request":  ingestReq,
			"response": ingestResp,
		},
	})
	for peerID, facts := range seedPeerCards(bundle.Bible) {
		patch := bridgeclient.PeerCardPatch{Facts: facts, Replace: true}
		logEvent(logger, gamelog.Event{
			Category: "honcho",
			Event:    "story.seed.card.request",
			Message:  "base Honcho peer card request prepared",
			Fields:   map[string]any{"peer_id": peerID, "fact_count": len(facts), "workspace_id": state.WorkspaceID},
			Detail: map[string]any{
				"workspace_id": state.WorkspaceID,
				"peer_id":      peerID,
				"request":      patch,
			},
		})
		cardResp, err := client.PatchCard(ctx, state.WorkspaceID, peerID, patch)
		if err != nil {
			logEvent(logger, gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "honcho",
				Event:    "story.seed.card.failed",
				Message:  err.Error(),
				Fields:   map[string]any{"peer_id": peerID},
				Detail: map[string]any{
					"workspace_id": state.WorkspaceID,
					"peer_id":      peerID,
					"request":      patch,
					"error":        err.Error(),
				},
			})
			return fmt.Errorf("seed Honcho card %q: %w", peerID, err)
		}
		logEvent(logger, gamelog.Event{
			Category: "honcho",
			Event:    "story.seed.card.seeded",
			Message:  "base Honcho peer card seeded",
			Fields:   map[string]any{"peer_id": peerID, "fact_count": len(facts), "response_fact_count": len(cardResp)},
			Detail: map[string]any{
				"workspace_id": state.WorkspaceID,
				"peer_id":      peerID,
				"request":      patch,
				"response":     cardResp,
			},
		})
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
	lawFacts := make([]string, 0, len(bible.BindingLorePrinciples()))
	for _, law := range bible.BindingLorePrinciples() {
		lawFacts = append(lawFacts, fmt.Sprintf("INSTRUCTION: Enforce binding lore principle %s: %s", law.Name, law.Enforcement))
	}
	baseBoundaries := []string{
		"BOUNDARY: Lorepack references support the active scenario contract; unused lore is not automatic scene canon.",
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
		"INSTRUCTION: Use the Story Bible and lorepack scenario contract as the active story contract.",
		"INSTRUCTION: Preserve witness-bound canon boundaries.",
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
	req := contextbundle.BuildRequest{Packet: packet, NodeID: nodeID, Decision: decision, Bible: bible}
	switch role {
	case "narrator":
		assembler := contextAssembler(bridgeURL, bridgeURL != "", cardStore, globalCollection, storyCollection, nil)
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
		assembler := writerRefreshContextAssembler(bridgeURL, bridgeURL != "", cardStore, storyCollection, nil)
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
	Packet               scene.StoryPacket
	NodeID               string
	Backend              string
	BridgeURL            string
	CardStore            *contextbundle.LocalCardStore
	Workspace            *story.Workspace
	State                *story.StoryState
	Bible                *story.StoryBible
	GroundLorePackID     string
	GlobalCollection     string
	StoryCollection      string
	InitialDecision      scene.PlayerDecision
	Turns                int
	Commit               bool
	RefreshWrite         bool
	TraceJSONL           bool
	UseTUI               bool
	Status               func(string)
	WriterScenaristCalls int
	ScenaristProvider    string
	ScenaristModel       string
	ScenaristTools       bool
	ScenaristOutputRoot  string
	ScenaristRounds      int
	Logger               *gamelog.Writer
	WriterPlanner        writer.Planner
	TTS                  playTTSConfig
}

type writerRefreshFailure struct {
	Err    error
	RunID  string
	RunDir string
}

func (e writerRefreshFailure) Error() string {
	if e.RunDir != "" {
		return fmt.Sprintf("writer refresh failed: %v (artifacts: %s)", e.Err, filepath.ToSlash(e.RunDir))
	}
	return fmt.Sprintf("writer refresh failed: %v", e.Err)
}

func (e writerRefreshFailure) Unwrap() error {
	return e.Err
}

func runPlay(ctx context.Context, cfg playConfig) error {
	if cfg.UseTUI {
		return runPlayTUI(ctx, cfg)
	}
	return runPlayRaw(ctx, cfg)
}

func runPlayRaw(ctx context.Context, cfg playConfig) error {
	packet := cfg.Packet
	nodeID := cfg.NodeID
	decision := cfg.InitialDecision
	var state *story.StoryState
	if cfg.State != nil {
		copied := *cfg.State
		state = &copied
	}
	if cfg.Logger == nil {
		cfg.Logger = storyLogger(cfg.Workspace, state)
	}
	if state != nil && state.Status == story.StatusBlockedWriterRefresh {
		return fmt.Errorf("story is blocked by Writer refresh: %s (artifacts: %s)", state.BlockedReason, state.BlockedWriterRunPath)
	}
	if err := recoverPendingDisplayedTurnFromLogs(cfg, state, packet, nodeID); err != nil {
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "story",
			Event:    "story.turn.awaiting_choice_recovery_failed",
			Message:  err.Error(),
			PacketID: packet.ID,
			NodeID:   nodeID,
		})
	}
	if story.PlayerDecisionEmpty(decision) && state != nil && state.HasPendingDecision() {
		decision = state.PendingDecision
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "story",
		Event:    "story.play.started",
		Message:  "play loop started",
		PacketID: packet.ID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"backend":        cfg.Backend,
			"commit":         cfg.Commit,
			"refresh_writer": cfg.RefreshWrite,
			"turn_limit":     cfg.Turns,
		},
	})
	reader := bufio.NewReader(os.Stdin)
	totalTurns := cfg.Turns
	for turn := 1; totalTurns <= 0 || turn <= totalTurns; turn++ {
		displayTurn := turn
		turnAudioPath := ""
		rollout, pendingRecord, resumedPending, err := loadPendingDisplayedRollout(cfg, state, packet, nodeID, decision, turn)
		if err != nil {
			return err
		}
		if resumedPending {
			if pendingRecord.Turn > 0 {
				displayTurn = pendingRecord.Turn
			}
			turnAudioPath = pendingRecord.AudioPath
			logEvent(cfg.Logger, gamelog.Event{
				Category: "story",
				Event:    "story.turn.awaiting_choice_resumed",
				Message:  "resumed displayed turn awaiting player choice",
				Turn:     displayTurn,
				PacketID: rollout.Commit.PacketID,
				NodeID:   rollout.Commit.NodeID,
				Fields: map[string]any{
					"pending_turn_path": state.PendingTurnPath,
				},
			})
		} else {
			rollout, err = rolloutOnce(ctx, packet, nodeID, decision, cfg, displayTurn)
			if err != nil {
				metadata := narratorModelMetadata(cfg)
				logEvent(cfg.Logger, gamelog.Event{
					Level:    gamelog.LevelError,
					Category: "narrator",
					Event:    "narrator.rollout.failed",
					Message:  err.Error(),
					PacketID: packet.ID,
					NodeID:   nodeID,
					Fields:   modelmeta.MergeFields(nil, metadata),
					Detail: map[string]any{
						"error":          err.Error(),
						"model_metadata": metadata,
					},
				})
				return err
			}
			metadata := narratorModelMetadata(cfg)
			logEvent(cfg.Logger, gamelog.Event{
				Category: "narrator",
				Event:    "narrator.rollout.completed",
				Message:  "narrator rollout completed",
				Turn:     displayTurn,
				PacketID: rollout.Commit.PacketID,
				NodeID:   rollout.Commit.NodeID,
				Fields: modelmeta.MergeFields(map[string]any{
					"choice_count":       len(rollout.Draft.Choices),
					"prose_chars":        len([]rune(rollout.Draft.Prose)),
					"commit_event_count": len(rollout.Commit.Events),
					"witness_count":      len(rollout.Commit.Witnesses),
				}, metadata),
				Detail: map[string]any{
					"draft":          rollout.Draft,
					"commit":         rollout.Commit,
					"model_metadata": metadata,
				},
			})
		}
		fmt.Printf("\n--- Turn %d ---\n\n%s\n", displayTurn, rollout.Draft.Prose)
		if len(rollout.Draft.Choices) > 0 {
			fmt.Println("\nChoices:")
			for _, choice := range rollout.Draft.Choices {
				fmt.Printf("- %s: %s\n", choice.ID, choice.Label)
			}
		}
		if !resumedPending {
			turnAudioPath, err = synthesizeTurnAudio(ctx, cfg, state, turn, rollout)
			if err != nil {
				return err
			}
			if turnAudioPath != "" {
				fmt.Printf("\nAudio saved: %s\n", filepath.ToSlash(turnAudioPath))
			}
			if cfg.Commit {
				if err := commitDisplayedTurn(ctx, cfg, state, packet, rollout, turnAudioPath, displayTurn); err != nil {
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
			if err := savePendingDisplayedTurn(cfg, state, packet, rollout, cfg.Commit, turnAudioPath, turn); err != nil {
				return err
			}
		} else if pendingRecord.Committed {
			fmt.Printf("\nCommitted: packet=%s node=%s (already recorded)\n", rollout.Commit.PacketID, rollout.Commit.NodeID)
		} else if cfg.Commit {
			fmt.Println("\nRetrying pending Honcho commit...")
			if err := commitDisplayedTurn(ctx, cfg, state, packet, rollout, turnAudioPath, displayTurn); err != nil {
				return err
			}
			if err := savePendingDisplayedTurn(cfg, state, packet, rollout, true, turnAudioPath, displayTurn); err != nil {
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
				Turn:             displayTurn,
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
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", scene.PlayerDecision{}, cfg.Commit, turnAudioPath); err != nil {
				return err
			}
			break
		}
		if len(rollout.Node.Choices) == 0 && !cfg.RefreshWrite {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", scene.PlayerDecision{}, cfg.Commit, turnAudioPath); err != nil {
				return err
			}
			fmt.Println("\nNo choices available; play loop stopped.")
			break
		}
		terminalHandoff := len(rollout.Node.Choices) == 0 && cfg.RefreshWrite
		if terminalHandoff && cfg.Turns <= 0 {
			fmt.Println("\nNo choices in this handoff. Press Enter to continue, or type a direction for Writer.")
		}
		nextDecision := automatedDecision(rollout.Node)
		rawInput := ""
		if cfg.Turns <= 0 {
			if terminalHandoff {
				fmt.Print("\ncontinuation> ")
			} else {
				fmt.Print("\nchoice id or free text> ")
			}
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
		if targetNodeID, ok := resolvedChoiceTarget(packet, rollout.Node, nextDecision); ok {
			nextNodeID = targetNodeID
			nextRolloutDecision := nextDecision
			if isCosmeticIntroChoice(packet, rollout.Node) {
				nextRolloutDecision = scene.PlayerDecision{}
			}
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, nextNodeID, nextDecision, cfg.Commit, turnAudioPath); err != nil {
				return err
			}
			nodeID = nextNodeID
			decision = nextRolloutDecision
			if cfg.TraceJSONL {
				if err := writePlayTrace(decisionTrace(rawInput, nextDecision, nextNodeID)); err != nil {
					return err
				}
			}
			continue
		}
		if cfg.RefreshWrite {
			refreshCfg := cfg
			refreshCfg.State = state
			runID, runDir := writerRunInfo(refreshCfg)
			metadata := writerRefreshModelMetadata(refreshCfg)
			if err := savePendingDecision(cfg, state, nextDecision, runID, runDir); err != nil {
				return err
			}
			logEvent(cfg.Logger, gamelog.Event{
				Category: "writer",
				Event:    "writer.refresh.started",
				Message:  "writer refresh started",
				Turn:     turn,
				PacketID: packet.ID,
				NodeID:   nodeID,
				RunID:    runID,
				Fields: modelmeta.MergeFields(map[string]any{
					"artifact_dir": filepath.ToSlash(runDir),
				}, metadata),
				Detail: map[string]any{
					"artifact_dir":   filepath.ToSlash(runDir),
					"model_metadata": metadata,
				},
			})
			next, err := refreshPacket(ctx, packet, nodeID, nextDecision, refreshCfg)
			if err == nil {
				nextNodeID = entryNodeID(next)
				logEvent(cfg.Logger, gamelog.Event{
					Category: "writer",
					Event:    "writer.refresh.completed",
					Message:  "writer refresh completed",
					Turn:     turn,
					PacketID: next.ID,
					NodeID:   nextNodeID,
					RunID:    runID,
					Fields: modelmeta.MergeFields(map[string]any{
						"artifact_dir": filepath.ToSlash(runDir),
					}, metadata),
					Detail: map[string]any{
						"artifact_dir":   filepath.ToSlash(runDir),
						"model_metadata": metadata,
					},
				})
				if err := persistStoryTurn(ctx, cfg, state, packet, rollout, nextNodeID, nextDecision, cfg.Commit, turnAudioPath); err != nil {
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
			if isRecoverableDisconnect(err) {
				return err
			}
			failure := writerFailure(err, runID, runDir)
			detail := writerFailureDetails(failure)
			detail["model_metadata"] = metadata
			logEvent(cfg.Logger, gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "writer",
				Event:    "writer.refresh.failed",
				Message:  failure.Err.Error(),
				Turn:     turn,
				PacketID: packet.ID,
				NodeID:   nodeID,
				RunID:    failure.RunID,
				Fields: modelmeta.MergeFields(map[string]any{
					"artifact_dir": filepath.ToSlash(failure.RunDir),
				}, metadata),
				Detail: detail,
			})
			if err := markStoryBlocked(cfg, state, failure); err != nil {
				return err
			}
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", nextDecision, cfg.Commit, turnAudioPath); err != nil {
				return err
			}
			if cfg.TraceJSONL {
				if err := writePlayTrace(decisionTrace(rawInput, nextDecision, "")); err != nil {
					return err
				}
			}
			return failure
		}
		if nextDecision.FreeText != "" && !cfg.RefreshWrite {
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", nextDecision, cfg.Commit, turnAudioPath); err != nil {
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
			if err := persistStoryTurn(ctx, cfg, state, packet, rollout, "", nextDecision, cfg.Commit, turnAudioPath); err != nil {
				return err
			}
			fmt.Println("\nNo choices available; play loop stopped.")
			break
		}
	}
	return nil
}

func commitDisplayedTurn(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, audioPath string, displayTurn int) error {
	client := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
	commitStarted := time.Now()
	logEvent(cfg.Logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.commit.started",
		Message:  "witnessed canon commit started",
		Turn:     displayTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"session_id":    rollout.Commit.SessionID,
			"event_count":   len(rollout.Commit.Events),
			"witness_count": len(rollout.Commit.Witnesses),
		},
		Detail: map[string]any{
			"request": rollout.Commit,
		},
	})
	if err := client.CommitScene(ctx, rollout.Commit); err != nil {
		if saveErr := savePendingDisplayedTurn(cfg, state, packet, rollout, false, audioPath, displayTurn); saveErr != nil {
			err = fmt.Errorf("%w; additionally failed to save pending displayed turn: %v", err, saveErr)
		}
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "honcho",
			Event:    "honcho.commit.failed",
			Message:  err.Error(),
			Turn:     displayTurn,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: map[string]any{
				"session_id":  rollout.Commit.SessionID,
				"elapsed_ms":  time.Since(commitStarted).Milliseconds(),
				"event_count": len(rollout.Commit.Events),
			},
			Detail: map[string]any{
				"request": rollout.Commit,
				"error":   err.Error(),
			},
		})
		return err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.commit.completed",
		Message:  "witnessed canon committed",
		Turn:     displayTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"session_id":    rollout.Commit.SessionID,
			"elapsed_ms":    time.Since(commitStarted).Milliseconds(),
			"event_count":   len(rollout.Commit.Events),
			"witness_count": len(rollout.Commit.Witnesses),
		},
		Detail: map[string]any{
			"request": rollout.Commit,
		},
	})
	return nil
}

type playLoopController struct {
	cfg        playConfig
	packet     scene.StoryPacket
	nodeID     string
	decision   scene.PlayerDecision
	state      *story.StoryState
	turn       int
	current    *playControllerTurn
	statusMu   sync.Mutex
	status     []clitui.PlayStatus
	statusSink func(clitui.PlayStatus)
}

type playControllerTurn struct {
	displayTurn int
	rollout     narrator.Rollout
	audioPath   string
	committed   bool
}

func runPlayTUI(ctx context.Context, cfg playConfig) error {
	controller, err := newPlayLoopController(cfg)
	if err != nil {
		return err
	}
	return clitui.Play(ctx, controller)
}

func newPlayLoopController(cfg playConfig) (*playLoopController, error) {
	packet := cfg.Packet
	nodeID := cfg.NodeID
	decision := cfg.InitialDecision
	var state *story.StoryState
	if cfg.State != nil {
		copied := *cfg.State
		state = &copied
	}
	if cfg.Logger == nil {
		cfg.Logger = storyLogger(cfg.Workspace, state)
	}
	if state != nil && state.Status == story.StatusBlockedWriterRefresh {
		return nil, fmt.Errorf("story is blocked by Writer refresh: %s (artifacts: %s)", state.BlockedReason, state.BlockedWriterRunPath)
	}
	if err := recoverPendingDisplayedTurnFromLogs(cfg, state, packet, nodeID); err != nil {
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "story",
			Event:    "story.turn.awaiting_choice_recovery_failed",
			Message:  err.Error(),
			PacketID: packet.ID,
			NodeID:   nodeID,
		})
	}
	if story.PlayerDecisionEmpty(decision) && state != nil && state.HasPendingDecision() {
		decision = state.PendingDecision
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "story",
		Event:    "story.play.started",
		Message:  "play loop started",
		PacketID: packet.ID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"backend":        cfg.Backend,
			"commit":         cfg.Commit,
			"refresh_writer": cfg.RefreshWrite,
			"turn_limit":     cfg.Turns,
			"ui":             "tui",
		},
	})
	return &playLoopController{
		cfg:      cfg,
		packet:   packet,
		nodeID:   nodeID,
		decision: decision,
		state:    state,
	}, nil
}

func (c *playLoopController) SetStatusSink(sink func(clitui.PlayStatus)) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.statusSink = sink
}

func (c *playLoopController) IsRecoverableError(err error) bool {
	return isRecoverableDisconnect(err)
}

func (c *playLoopController) NextTurn(ctx context.Context) (clitui.TurnView, error) {
	c.turn++
	displayTurn := currentStoryTurnNumber(c.state, c.turn)
	c.resetStatus("Rendering narrator turn...")
	cfg := c.config()
	turnAudioPath := ""
	committed := false
	rollout, pendingRecord, resumedPending, err := loadPendingDisplayedRollout(cfg, c.state, c.packet, c.nodeID, c.decision, displayTurn)
	if err != nil {
		return clitui.TurnView{}, err
	}
	if resumedPending {
		if pendingRecord.Turn > 0 {
			displayTurn = pendingRecord.Turn
		}
		turnAudioPath = pendingRecord.AudioPath
		committed = pendingRecord.Committed
		c.addStatus("", "Resumed a displayed turn awaiting player input.")
		logEvent(cfg.Logger, gamelog.Event{
			Category: "story",
			Event:    "story.turn.awaiting_choice_resumed",
			Message:  "resumed displayed turn awaiting player choice",
			Turn:     displayTurn,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: map[string]any{
				"pending_turn_path": c.state.PendingTurnPath,
			},
		})
		if cfg.Commit && !committed {
			c.addStatus("", "Retrying pending Honcho commit...")
			if err := commitDisplayedTurn(ctx, cfg, c.state, c.packet, rollout, turnAudioPath, displayTurn); err != nil {
				return clitui.TurnView{}, err
			}
			committed = true
			if err := savePendingDisplayedTurn(cfg, c.state, c.packet, rollout, true, turnAudioPath, displayTurn); err != nil {
				return clitui.TurnView{}, err
			}
			c.addStatus("", "Pending Honcho commit completed.")
		}
	} else {
		rollout, err = rolloutOnce(ctx, c.packet, c.nodeID, c.decision, cfg, displayTurn)
		if err != nil {
			metadata := narratorModelMetadata(cfg)
			logEvent(cfg.Logger, gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "narrator",
				Event:    "narrator.rollout.failed",
				Message:  err.Error(),
				PacketID: c.packet.ID,
				NodeID:   c.nodeID,
				Fields:   modelmeta.MergeFields(nil, metadata),
				Detail: map[string]any{
					"error":          err.Error(),
					"model_metadata": metadata,
				},
			})
			return clitui.TurnView{}, err
		}
		metadata := narratorModelMetadata(cfg)
		logEvent(cfg.Logger, gamelog.Event{
			Category: "narrator",
			Event:    "narrator.rollout.completed",
			Message:  "narrator rollout completed",
			Turn:     displayTurn,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: modelmeta.MergeFields(map[string]any{
				"choice_count":       len(rollout.Draft.Choices),
				"prose_chars":        len([]rune(rollout.Draft.Prose)),
				"commit_event_count": len(rollout.Commit.Events),
				"witness_count":      len(rollout.Commit.Witnesses),
			}, metadata),
			Detail: map[string]any{
				"draft":          rollout.Draft,
				"commit":         rollout.Commit,
				"model_metadata": metadata,
			},
		})
		if cfg.TTS.Enabled {
			c.addStatus("", "Synthesizing turn audio...")
		}
		turnAudioPath, err = synthesizeTurnAudio(ctx, cfg, c.state, displayTurn, rollout)
		if err != nil {
			return clitui.TurnView{}, err
		}
		if cfg.Commit {
			c.addStatus("", "Committing witnessed canon...")
			if err := commitDisplayedTurn(ctx, cfg, c.state, c.packet, rollout, turnAudioPath, displayTurn); err != nil {
				return clitui.TurnView{}, err
			}
			committed = true
			c.addStatus("", "Honcho commit completed.")
		}
		if err := savePendingDisplayedTurn(cfg, c.state, c.packet, rollout, cfg.Commit, turnAudioPath, displayTurn); err != nil {
			return clitui.TurnView{}, err
		}
		c.addStatus("", "Turn checkpoint saved.")
	}
	if cfg.TraceJSONL {
		if err := writePlayTrace(playTraceTurn{
			Event:            "turn",
			Turn:             displayTurn,
			PacketID:         rollout.Commit.PacketID,
			NodeID:           rollout.Commit.NodeID,
			Decision:         rollout.Commit.PlayerDecision,
			ChoiceCount:      len(rollout.Draft.Choices),
			ProseChars:       len([]rune(rollout.Draft.Prose)),
			CommitEventCount: len(rollout.Commit.Events),
			Witnesses:        rollout.Commit.Witnesses,
		}); err != nil {
			return clitui.TurnView{}, err
		}
	}
	c.current = &playControllerTurn{
		displayTurn: displayTurn,
		rollout:     rollout,
		audioPath:   turnAudioPath,
		committed:   committed,
	}
	return clitui.TurnView{
		Turn:            displayTurn,
		PacketID:        rollout.Commit.PacketID,
		NodeID:          rollout.Commit.NodeID,
		Prose:           rollout.Draft.Prose,
		Choices:         choiceViews(rollout),
		TerminalHandoff: len(rollout.Node.Choices) == 0 && cfg.RefreshWrite,
		CommitLabel:     commitLabel(rollout, committed, cfg.Commit),
		AudioPath:       filepath.ToSlash(turnAudioPath),
		Status:          c.statusSnapshot(),
	}, nil
}

func (c *playLoopController) Submit(ctx context.Context, submitted clitui.SubmittedDecision) (bool, error) {
	if submitted.Exit {
		return true, nil
	}
	if c.current == nil {
		return true, nil
	}
	cfg := c.config()
	rollout := c.current.rollout
	displayTurn := c.current.displayTurn
	nextDecision := scene.PlayerDecision{ChoiceID: submitted.ChoiceID, FreeText: submitted.FreeText}
	rawInput := strings.TrimSpace(submitted.RawInput)
	if rawInput == "" {
		rawInput = firstNonEmpty(nextDecision.ChoiceID, nextDecision.FreeText)
	}
	if cfg.Turns > 0 && displayTurn >= cfg.Turns {
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", scene.PlayerDecision{}, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		return true, nil
	}
	if len(rollout.Node.Choices) == 0 && !cfg.RefreshWrite {
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", scene.PlayerDecision{}, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		return true, nil
	}
	if targetNodeID, ok := resolvedChoiceTarget(c.packet, rollout.Node, nextDecision); ok {
		nextRolloutDecision := nextDecision
		if isCosmeticIntroChoice(c.packet, rollout.Node) {
			nextRolloutDecision = scene.PlayerDecision{}
		}
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, targetNodeID, nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		c.nodeID = targetNodeID
		c.decision = nextRolloutDecision
		c.current = nil
		if cfg.TraceJSONL {
			if err := writePlayTrace(decisionTrace(rawInput, nextDecision, targetNodeID)); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if cfg.RefreshWrite {
		refreshCfg := cfg
		refreshCfg.State = c.state
		runID, runDir := writerRunInfo(refreshCfg)
		metadata := writerRefreshModelMetadata(refreshCfg)
		c.resetStatus("Refreshing Writer packet...")
		if err := savePendingDecision(cfg, c.state, nextDecision, runID, runDir); err != nil {
			return false, err
		}
		logEvent(cfg.Logger, gamelog.Event{
			Category: "writer",
			Event:    "writer.refresh.started",
			Message:  "writer refresh started",
			Turn:     displayTurn,
			PacketID: c.packet.ID,
			NodeID:   c.nodeID,
			RunID:    runID,
			Fields: modelmeta.MergeFields(map[string]any{
				"artifact_dir": filepath.ToSlash(runDir),
			}, metadata),
			Detail: map[string]any{
				"artifact_dir":   filepath.ToSlash(runDir),
				"model_metadata": metadata,
			},
		})
		next, err := refreshPacket(ctx, c.packet, c.nodeID, nextDecision, refreshCfg)
		if err == nil {
			nextNodeID := entryNodeID(next)
			logEvent(cfg.Logger, gamelog.Event{
				Category: "writer",
				Event:    "writer.refresh.completed",
				Message:  "writer refresh completed",
				Turn:     displayTurn,
				PacketID: next.ID,
				NodeID:   nextNodeID,
				RunID:    runID,
				Fields: modelmeta.MergeFields(map[string]any{
					"artifact_dir": filepath.ToSlash(runDir),
				}, metadata),
				Detail: map[string]any{
					"artifact_dir":   filepath.ToSlash(runDir),
					"model_metadata": metadata,
				},
			})
			if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, nextNodeID, nextDecision, cfg.Commit, c.current.audioPath); err != nil {
				return false, err
			}
			if err := saveCurrentStoryPacket(ctx, cfg, c.state, next, nextNodeID); err != nil {
				return false, err
			}
			c.packet = next
			c.nodeID = nextNodeID
			c.decision = nextDecision
			c.current = nil
			if cfg.TraceJSONL {
				if err := writePlayTrace(decisionTrace(rawInput, nextDecision, nextNodeID)); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		if isRecoverableDisconnect(err) {
			return false, err
		}
		failure := writerFailure(err, runID, runDir)
		detail := writerFailureDetails(failure)
		detail["model_metadata"] = metadata
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "writer",
			Event:    "writer.refresh.failed",
			Message:  failure.Err.Error(),
			Turn:     displayTurn,
			PacketID: c.packet.ID,
			NodeID:   c.nodeID,
			RunID:    failure.RunID,
			Fields: modelmeta.MergeFields(map[string]any{
				"artifact_dir": filepath.ToSlash(failure.RunDir),
			}, metadata),
			Detail: detail,
		})
		if err := markStoryBlocked(cfg, c.state, failure); err != nil {
			return false, err
		}
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		if cfg.TraceJSONL {
			if err := writePlayTrace(decisionTrace(rawInput, nextDecision, "")); err != nil {
				return false, err
			}
		}
		return false, failure
	}
	if nextDecision.FreeText != "" {
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		if cfg.TraceJSONL {
			if err := writePlayTrace(decisionTrace(rawInput, nextDecision, "")); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	if len(rollout.Node.Choices) == 0 {
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		return true, nil
	}
	return true, nil
}

func (c *playLoopController) config() playConfig {
	cfg := c.cfg
	cfg.State = c.state
	cfg.Status = c.reportStatus
	return cfg
}

func (c *playLoopController) resetStatus(message string) {
	c.statusMu.Lock()
	c.status = nil
	c.statusMu.Unlock()
	c.reportStatus(message)
}

func (c *playLoopController) reportStatus(message string) {
	c.addStatus("", message)
}

func (c *playLoopController) addStatus(level string, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	status := clitui.PlayStatus{Message: message, Level: level}
	c.statusMu.Lock()
	c.status = append(c.status, status)
	sink := c.statusSink
	c.statusMu.Unlock()
	if sink != nil {
		sink(status)
	}
}

func (c *playLoopController) statusSnapshot() []clitui.PlayStatus {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	return append([]clitui.PlayStatus{}, c.status...)
}

func choiceViews(rollout narrator.Rollout) []clitui.ChoiceView {
	choices := rollout.Draft.Choices
	if len(choices) == 0 {
		choices = rollout.Node.Choices
	}
	views := make([]clitui.ChoiceView, 0, len(choices))
	for _, choice := range choices {
		views = append(views, clitui.ChoiceView{ID: choice.ID, Label: choice.Label})
	}
	return views
}

func commitLabel(rollout narrator.Rollout, committed bool, commitEnabled bool) string {
	if committed {
		return fmt.Sprintf("Committed: packet=%s node=%s", rollout.Commit.PacketID, rollout.Commit.NodeID)
	}
	if commitEnabled {
		return fmt.Sprintf("Commit pending: packet=%s node=%s", rollout.Commit.PacketID, rollout.Commit.NodeID)
	}
	return fmt.Sprintf("Commit prepared: packet=%s node=%s witnesses=%s events=%d",
		rollout.Commit.PacketID,
		rollout.Commit.NodeID,
		strings.Join(rollout.Commit.Witnesses, ","),
		len(rollout.Commit.Events),
	)
}

func recoverPendingDisplayedTurnFromLogs(cfg playConfig, state *story.StoryState, packet scene.StoryPacket, nodeID string) error {
	if cfg.Workspace == nil || state == nil || cfg.Logger == nil || state.PendingTurnPath != "" || state.Status != story.StatusActive || state.LatestTurn != 0 || strings.TrimSpace(state.LastNarratorProse) != "" {
		return nil
	}
	events, err := gamelog.Read(cfg.Logger.Path, gamelog.Filter{Event: "narrator.rollout.completed"})
	if err != nil {
		return err
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.PacketID != packet.ID || event.NodeID != nodeID || event.DetailPath == "" {
			continue
		}
		var detail struct {
			Draft  narrator.Draft `json:"draft"`
			Commit scene.Commit   `json:"commit"`
		}
		if err := readLogDetailAs(cfg.Logger, event.DetailPath, &detail); err != nil {
			return err
		}
		if strings.TrimSpace(detail.Draft.Prose) == "" {
			continue
		}
		node, err := packet.Node(nodeID)
		if err != nil {
			return err
		}
		rollout := narrator.Rollout{
			Node:   node,
			Draft:  detail.Draft,
			Commit: detail.Commit,
		}
		committed := matchingLogEventExists(cfg.Logger.Path, "honcho.commit.completed", event.Turn, packet.ID, nodeID)
		audioPath := recoveredAudioPath(cfg.Workspace, event.Turn)
		if err := savePendingDisplayedTurn(cfg, state, packet, rollout, committed, audioPath, event.Turn); err != nil {
			return err
		}
		logEvent(cfg.Logger, gamelog.Event{
			Category: "story",
			Event:    "story.turn.awaiting_choice_recovered",
			Message:  "recovered displayed turn checkpoint from prior rollout log",
			Turn:     event.Turn,
			PacketID: packet.ID,
			NodeID:   nodeID,
			Fields: map[string]any{
				"source_event_id": event.EventID,
				"committed":       committed,
				"audio_path":      filepath.ToSlash(audioPath),
			},
		})
		return nil
	}
	return nil
}

func readLogDetailAs(logger *gamelog.Writer, detailPath string, out any) error {
	detail, err := logger.ReadDetail(detailPath)
	if err != nil {
		return err
	}
	data, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func matchingLogEventExists(path string, eventName string, turn int, packetID string, nodeID string) bool {
	events, err := gamelog.Read(path, gamelog.Filter{Event: eventName})
	if err != nil {
		return false
	}
	for _, event := range events {
		if event.Turn == turn && event.PacketID == packetID && event.NodeID == nodeID {
			return true
		}
	}
	return false
}

func recoveredAudioPath(workspace *story.Workspace, turn int) string {
	if workspace == nil || turn <= 0 {
		return ""
	}
	path := filepath.Join(workspace.Dir, "audio", fmt.Sprintf("turn-%06d.wav", turn))
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func loadPendingDisplayedRollout(cfg playConfig, state *story.StoryState, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, loopTurn int) (narrator.Rollout, story.TurnRecord, bool, error) {
	if cfg.Workspace == nil || state == nil || !isAwaitingTurnInputStatus(state.Status) || !story.PlayerDecisionEmpty(decision) {
		return narrator.Rollout{}, story.TurnRecord{}, false, nil
	}
	record, err := cfg.Workspace.LoadPendingTurn(*state)
	if err != nil {
		return narrator.Rollout{}, story.TurnRecord{}, false, fmt.Errorf("load pending displayed turn: %w", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Workspace.NarrationDir(), "pending-turn.md")); errors.Is(err, os.ErrNotExist) {
		if _, err := cfg.Workspace.SavePendingTurn(record); err != nil {
			return narrator.Rollout{}, story.TurnRecord{}, false, fmt.Errorf("backfill pending narration: %w", err)
		}
	}
	resolvedNodeID := firstNonEmpty(record.NodeID, nodeID)
	node, err := packet.Node(resolvedNodeID)
	if err != nil {
		return narrator.Rollout{}, story.TurnRecord{}, false, err
	}
	choices := record.Choices
	if len(choices) == 0 {
		choices = node.Choices
	}
	if record.Turn == 0 {
		record.Turn = currentStoryTurnNumber(state, loopTurn)
	}
	rollout := narrator.Rollout{
		Node: node,
		Draft: narrator.Draft{
			Prose:   record.Prose,
			Choices: choices,
		},
		Commit: record.Commit,
	}
	return rollout, record, true, nil
}

func isAwaitingTurnInputStatus(status string) bool {
	return status == story.StatusAwaitingPlayerChoice || status == story.StatusAwaitingContinuation
}

func savePendingDisplayedTurn(cfg playConfig, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, committed bool, audioPath string, loopTurn int) error {
	if cfg.Workspace == nil || state == nil {
		return nil
	}
	awaitingContinuation := len(rollout.Node.Choices) == 0 && len(rollout.Node.RevisionNotes) > 0
	record := story.TurnRecord{
		Turn:      currentStoryTurnNumber(state, loopTurn),
		StoryID:   state.StoryID,
		PacketID:  rollout.Commit.PacketID,
		NodeID:    rollout.Commit.NodeID,
		Decision:  rollout.Commit.PlayerDecision,
		Prose:     rollout.Draft.Prose,
		Choices:   rollout.Node.Choices,
		Commit:    rollout.Commit,
		Committed: committed,
		AudioPath: filepath.ToSlash(audioPath),
		CreatedAt: story.NowString(),
	}
	rel, err := cfg.Workspace.SavePendingTurn(record)
	if err != nil {
		return err
	}
	state.CurrentPacketID = packet.ID
	state.CurrentPacketPath = filepath.ToSlash(filepath.Join("packets", packet.ID+".json"))
	state.CurrentNodeID = rollout.Commit.NodeID
	state.Status = story.StatusAwaitingPlayerChoice
	eventName := "story.turn.awaiting_choice_saved"
	message := "displayed turn saved before player choice"
	if awaitingContinuation {
		state.Status = story.StatusAwaitingContinuation
		eventName = "story.turn.awaiting_continuation_saved"
		message = "displayed terminal handoff saved before Writer continuation"
	}
	state.LastDecision = rollout.Commit.PlayerDecision
	state.LastNarratorProse = rollout.Draft.Prose
	state.PendingTurnPath = rel
	if err := cfg.Workspace.SaveState(*state); err != nil {
		return err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "memory",
		Event:    eventName,
		Message:  message,
		Turn:     record.Turn,
		PacketID: record.PacketID,
		NodeID:   record.NodeID,
		Fields: map[string]any{
			"pending_turn_path":            rel,
			"choice_count":                 len(record.Choices),
			"requires_writer_continuation": awaitingContinuation,
			"committed":                    committed,
			"audio_path":                   record.AudioPath,
		},
		Detail: map[string]any{
			"turn_record": record,
		},
	})
	return nil
}

func rolloutOnce(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg playConfig, turn int) (narrator.Rollout, error) {
	registry, err := actorRegistry(cfg.CardStore)
	if err != nil {
		return narrator.Rollout{}, err
	}
	packet, removedNeeds := actors.PruneUnsupportedPacketNeeds(packet, registry)
	logUnsupportedActorNeeds(cfg.Logger, "actor.needs.filtered", "unsupported actor_needs filtered before narrator context assembly", turn, packet.ID, nodeID, removedNeeds)
	assembler := contextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection, cfg.Logger)
	assembler.Options.SkipMissingActorCards = true
	scenarioContract := scenarioContractTextForPack(cfg.GroundLorePackID)
	logLoreFidelityContext(cfg.Logger, "narrator", "", turn, packet.ID, nodeID, cfg.GroundLorePackID, scenarioContract)
	narratorContext, err := assembler.AssembleNarrator(ctx, contextbundle.BuildRequest{
		Packet:               packet,
		NodeID:               nodeID,
		Decision:             decision,
		Bible:                cfg.Bible,
		ScenarioContractText: scenarioContract,
	})
	if err != nil {
		return narrator.Rollout{}, err
	}
	narratorContextText := narratorContext.RenderText()
	actorInvoker := actors.Invoker(actors.StaticInvoker{})
	renderer := narrator.Renderer(narrator.StaticRenderer{})
	switch cfg.Backend {
	case "claude":
		runner := claudeRunner()
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	case "codex":
		actorInvoker = actors.ClaudeInvoker{Runner: codexRunner("ActorOutput", prompt.ActorOutputSchema())}
		scope, err := prepareStoryAgentScope(cfg, "narrator", fmt.Sprintf("turn-%06d", turn), packet, nodeID, narratorContextText)
		if err != nil {
			return narrator.Rollout{}, err
		}
		renderer = narrator.ClaudeRenderer{Runner: codexRunnerWithReadTools("NarratorDraft", prompt.NarratorDraftSchema(), scope, cfg.Logger, "narrator", "narrator", "Narrator", turn, packet.ID, nodeID)}
	}
	return narrator.Service{
		Actors:           &actors.Service{Registry: registry, Invoker: actorInvoker, Logger: cfg.Logger, PacketID: packet.ID, Turn: turn, SkipMissingActors: true},
		Renderer:         renderer,
		Logger:           cfg.Logger,
		Turn:             turn,
		ActorContextText: narratorContext.RenderActorText(),
	}.Rollout(ctx, packet, nodeID, decision, narratorContextText)
}

func synthesizeTurnAudio(ctx context.Context, cfg playConfig, state *story.StoryState, loopTurn int, rollout narrator.Rollout) (string, error) {
	if !cfg.TTS.Enabled {
		return "", nil
	}
	text := strings.TrimSpace(rollout.Draft.Prose)
	if text == "" {
		return "", nil
	}
	format := normalizeTTSFormat(cfg.TTS.Format)
	if format == "" {
		return "", fmt.Errorf("unsupported tts format %q; expected wav or pcm16", cfg.TTS.Format)
	}
	turnNumber := currentStoryTurnNumber(state, loopTurn)
	outputPath := ttsOutputPath(cfg, state, loopTurn)
	runDir := ttsRunDir(cfg, state, loopTurn)
	client := ttsclient.Client{
		BaseURL:     cfg.TTS.BaseURL,
		APIKey:      firstNonEmpty(cfg.TTS.APIKey, os.Getenv("SANS_NOM_TTS_API_KEY")),
		Model:       cfg.TTS.Model,
		Voice:       cfg.TTS.Voice,
		Format:      format,
		Instruction: cfg.TTS.Instruction,
		Timeout:     cfg.TTS.Timeout,
	}
	chunks := splitTTSChunks(text, envInt("SANS_NOM_TTS_CHUNK_CHARS", defaultTTSChunkChars))
	requestFormat := format
	if len(chunks) > 1 && format == "wav" {
		requestFormat = "pcm16"
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	requestSummary := ttsRequestSummary(client, text, chunks, format, requestFormat, outputPath)
	ttsMetadata := ttsModelMetadata(cfg.TTS, stringFromAny(requestSummary["model"]))
	if err := writeJSONFileAtomic(filepath.Join(runDir, "request.json"), map[string]any{
		"request":         requestSummary,
		"model_metadata":  ttsMetadata,
		"base_url":        firstNonEmpty(cfg.TTS.BaseURL, ttsclient.DefaultBaseURL),
		"output_path":     filepath.ToSlash(outputPath),
		"instruction":     cfg.TTS.Instruction,
		"text_sha256":     textSHA256(text),
		"text_chars":      len([]rune(text)),
		"chunk_count":     len(chunks),
		"chunk_max_chars": envInt("SANS_NOM_TTS_CHUNK_CHARS", defaultTTSChunkChars),
	}); err != nil {
		return "", err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "model",
		Event:    "tts.mimo.request",
		Message:  "mimo turn tts request prepared",
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"model":          requestSummary["model"],
			"voice":          requestSummary["voice"],
			"format":         format,
			"request_format": requestFormat,
			"text_chars":     len([]rune(text)),
			"chunk_count":    len(chunks),
			"output_path":    filepath.ToSlash(outputPath),
			"run_dir":        filepath.ToSlash(runDir),
		}, ttsMetadata),
		Detail: map[string]any{
			"request":        requestSummary,
			"base_url":       firstNonEmpty(cfg.TTS.BaseURL, ttsclient.DefaultBaseURL),
			"output_path":    filepath.ToSlash(outputPath),
			"run_dir":        filepath.ToSlash(runDir),
			"text_sha256":    textSHA256(text),
			"model_metadata": ttsMetadata,
		},
	})
	ttsCtx, cancel := context.WithTimeout(ctx, client.TimeoutOrDefault())
	defer cancel()
	playStatusf(cfg, "\nTTS MiMo: synthesizing turn %d in %d chunk(s)...", turnNumber, len(chunks))
	var chunkAudios [][]byte
	var responses []map[string]any
	totalBase64Chars := 0
	for index, chunk := range chunks {
		chunkClient := client
		chunkClient.Format = requestFormat
		chunkRequest := chunkClient.RequestPayload(chunk)
		chunkDir := filepath.Join(runDir, fmt.Sprintf("chunk-%03d", index+1))
		if err := os.MkdirAll(chunkDir, 0o755); err != nil {
			return "", err
		}
		if err := writeJSONFileAtomic(filepath.Join(chunkDir, "request.json"), chunkRequest); err != nil {
			return "", err
		}
		playStatusf(cfg, "TTS MiMo chunk %d/%d...", index+1, len(chunks))
		result, err := chunkClient.Synthesize(ttsCtx, chunk)
		responsePath := filepath.Join(chunkDir, "response.json")
		if writeErr := writeJSONFileAtomic(responsePath, result.Response); writeErr != nil {
			return "", writeErr
		}
		if err != nil {
			failed := map[string]any{
				"chunk":              index + 1,
				"chars":              len([]rune(chunk)),
				"text_sha256":        textSHA256(chunk),
				"request_path":       filepath.ToSlash(filepath.Join(chunkDir, "request.json")),
				"response_path":      filepath.ToSlash(responsePath),
				"audio_path":         "",
				"audio_bytes":        0,
				"audio_base64_chars": result.AudioBase64Chars,
				"error":              err.Error(),
			}
			responses = append(responses, failed)
			_ = writeJSONFileAtomic(filepath.Join(runDir, "response.json"), map[string]any{
				"chunk_count":  len(chunks),
				"failed_chunk": index + 1,
				"chunks":       responses,
			})
			logTTSFailure(cfg, rollout, turnNumber, runDir, outputPath, err, map[string]any{
				"request":       chunkRequest,
				"chunk":         index + 1,
				"chunk_count":   len(chunks),
				"response_path": filepath.ToSlash(responsePath),
			})
			return "", err
		}
		chunkAudioPath := filepath.Join(chunkDir, "audio."+requestFormat)
		if err := writeBinaryAtomic(chunkAudioPath, result.Audio); err != nil {
			logTTSFailure(cfg, rollout, turnNumber, runDir, outputPath, err, map[string]any{
				"request":       chunkRequest,
				"chunk":         index + 1,
				"chunk_count":   len(chunks),
				"response_path": filepath.ToSlash(responsePath),
				"phase":         "write_chunk_audio",
			})
			return "", err
		}
		chunkAudios = append(chunkAudios, result.Audio)
		totalBase64Chars += result.AudioBase64Chars
		responses = append(responses, map[string]any{
			"chunk":              index + 1,
			"chars":              len([]rune(chunk)),
			"text_sha256":        textSHA256(chunk),
			"request_path":       filepath.ToSlash(filepath.Join(chunkDir, "request.json")),
			"response_path":      filepath.ToSlash(responsePath),
			"audio_path":         filepath.ToSlash(chunkAudioPath),
			"audio_bytes":        len(result.Audio),
			"audio_base64_chars": result.AudioBase64Chars,
		})
		chunkDetail := responses[len(responses)-1]
		chunkDetail["model_metadata"] = ttsMetadata
		logEvent(cfg.Logger, gamelog.Event{
			Category: "model",
			Event:    "tts.mimo.chunk.completed",
			Message:  "mimo turn tts chunk written",
			Turn:     turnNumber,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: modelmeta.MergeFields(map[string]any{
				"chunk":       index + 1,
				"chunk_count": len(chunks),
				"chars":       len([]rune(chunk)),
				"audio_bytes": len(result.Audio),
			}, ttsMetadata),
			Detail: chunkDetail,
		})
	}
	if err := writeJSONFileAtomic(filepath.Join(runDir, "response.json"), map[string]any{
		"chunk_count": len(chunks),
		"chunks":      responses,
	}); err != nil {
		return "", err
	}
	audio := bytes.Join(chunkAudios, nil)
	if format == "wav" && requestFormat == "pcm16" {
		audio = wavFromPCM16(audio, 24000, 1)
	}
	if err := writeBinaryAtomic(outputPath, audio); err != nil {
		logTTSFailure(cfg, rollout, turnNumber, runDir, outputPath, err, map[string]any{"request": requestSummary, "response_path": filepath.ToSlash(filepath.Join(runDir, "response.json")), "phase": "write_audio_file"})
		return "", err
	}
	runMetadata := map[string]any{
		"model":              requestSummary["model"],
		"voice":              requestSummary["voice"],
		"format":             format,
		"request_format":     requestFormat,
		"chunk_count":        len(chunks),
		"text_chars":         len([]rune(text)),
		"text_sha256":        textSHA256(text),
		"output_path":        filepath.ToSlash(outputPath),
		"audio_bytes":        len(audio),
		"audio_base64_chars": totalBase64Chars,
		"response_path":      filepath.ToSlash(filepath.Join(runDir, "response.json")),
		"created_at":         story.NowString(),
	}
	runMetadata["model_metadata"] = ttsMetadata
	if err := writeJSONFileAtomic(filepath.Join(runDir, "metadata.json"), runMetadata); err != nil {
		logTTSFailure(cfg, rollout, turnNumber, runDir, outputPath, err, map[string]any{"request": requestSummary, "metadata": runMetadata, "phase": "write_metadata"})
		return "", err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "model",
		Event:    "tts.mimo.completed",
		Message:  "mimo turn tts audio written",
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"model":       requestSummary["model"],
			"voice":       requestSummary["voice"],
			"format":      format,
			"chunk_count": len(chunks),
			"audio_bytes": len(audio),
			"output_path": filepath.ToSlash(outputPath),
			"run_dir":     filepath.ToSlash(runDir),
		}, ttsMetadata),
		Detail: map[string]any{
			"request":        requestSummary,
			"response_path":  filepath.ToSlash(filepath.Join(runDir, "response.json")),
			"metadata":       runMetadata,
			"model_metadata": ttsMetadata,
		},
	})
	return outputPath, nil
}

func playStatusf(cfg playConfig, format string, args ...any) {
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		return
	}
	if cfg.Status != nil {
		cfg.Status(message)
		return
	}
	fmt.Println(message)
}

func ttsOutputPath(cfg playConfig, state *story.StoryState, loopTurn int) string {
	outputDir := strings.TrimSpace(cfg.TTS.OutputDir)
	if outputDir == "" && cfg.Workspace != nil {
		outputDir = filepath.Join(cfg.Workspace.Dir, "audio")
	}
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "tts")
	}
	return filepath.Join(outputDir, fmt.Sprintf("turn-%06d.%s", currentStoryTurnNumber(state, loopTurn), normalizeTTSFormat(cfg.TTS.Format)))
}

func ttsRunDir(cfg playConfig, state *story.StoryState, loopTurn int) string {
	segment := fmt.Sprintf("turn-%06d", currentStoryTurnNumber(state, loopTurn))
	if cfg.Workspace != nil {
		return filepath.Join(cfg.Workspace.ModelRunsDir(), "tts", segment)
	}
	outputDir := strings.TrimSpace(cfg.TTS.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "tts")
	}
	return filepath.Join(outputDir, "model-runs", "tts", segment)
}

func logTTSFailure(cfg playConfig, rollout narrator.Rollout, turnNumber int, runDir string, outputPath string, err error, detail map[string]any) {
	_ = os.MkdirAll(runDir, 0o755)
	_ = writeTextAtomic(filepath.Join(runDir, "error.txt"), err.Error()+"\n")
	if detail == nil {
		detail = map[string]any{}
	}
	detail["error"] = err.Error()
	detail["output_path"] = filepath.ToSlash(outputPath)
	detail["run_dir"] = filepath.ToSlash(runDir)
	metadata := ttsModelMetadata(cfg.TTS, "")
	detail["model_metadata"] = metadata
	logEvent(cfg.Logger, gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "model",
		Event:    "tts.mimo.failed",
		Message:  err.Error(),
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"output_path": filepath.ToSlash(outputPath),
			"run_dir":     filepath.ToSlash(runDir),
		}, metadata),
		Detail: detail,
	})
}

func normalizeTTSFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "wav":
		return "wav"
	case "pcm16":
		return "pcm16"
	default:
		return ""
	}
}

const defaultTTSChunkChars = 4500

func textSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func ttsRequestSummary(client ttsclient.Client, text string, chunks []string, outputFormat string, requestFormat string, outputPath string) map[string]any {
	template := client
	template.Format = requestFormat
	request := template.RequestPayload("")
	chunkSummaries := make([]map[string]any, 0, len(chunks))
	for index, chunk := range chunks {
		chunkSummaries = append(chunkSummaries, map[string]any{
			"chunk":       index + 1,
			"chars":       len([]rune(chunk)),
			"text_sha256": textSHA256(chunk),
		})
	}
	return map[string]any{
		"model":          request.Model,
		"voice":          request.Audio.Voice,
		"output_format":  outputFormat,
		"request_format": requestFormat,
		"instruction":    client.Instruction,
		"text_sha256":    textSHA256(text),
		"text_chars":     len([]rune(text)),
		"chunk_count":    len(chunks),
		"chunks":         chunkSummaries,
		"output_path":    filepath.ToSlash(outputPath),
		"endpoint":       strings.TrimRight(firstNonEmpty(client.BaseURL, ttsclient.DefaultBaseURL), "/") + "/chat/completions",
	}
}

func ttsModelMetadata(cfg playTTSConfig, model string) modelmeta.Metadata {
	if strings.TrimSpace(model) == "" {
		model = firstNonEmpty(cfg.Model, "mimo-v2.5-tts")
	}
	return modelmeta.Metadata{
		ProviderType:  "openai_compatible_tts",
		ModelProvider: "xiaomi_mimo",
		Model:         model,
	}
}

func splitTTSChunks(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = defaultTTSChunkChars
	}
	if len([]rune(text)) <= maxRunes {
		return []string{text}
	}
	var chunks []string
	for _, paragraph := range strings.Split(text, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if len([]rune(paragraph)) <= maxRunes {
			chunks = appendTTSChunk(chunks, paragraph, maxRunes)
			continue
		}
		for _, sentence := range splitSentences(paragraph) {
			chunks = appendTTSChunk(chunks, sentence, maxRunes)
		}
	}
	return chunks
}

func appendTTSChunk(chunks []string, part string, maxRunes int) []string {
	part = strings.TrimSpace(part)
	if part == "" {
		return chunks
	}
	if len([]rune(part)) > maxRunes {
		var current []rune
		for _, word := range strings.Fields(part) {
			wordRunes := []rune(word)
			if len(current) > 0 && len(current)+1+len(wordRunes) > maxRunes {
				chunks = appendTTSChunk(chunks, string(current), maxRunes)
				current = nil
			}
			if len(current) > 0 {
				current = append(current, ' ')
			}
			current = append(current, wordRunes...)
		}
		if len(current) > 0 {
			chunks = append(chunks, strings.TrimSpace(string(current)))
		}
		return chunks
	}
	if len(chunks) == 0 {
		return append(chunks, part)
	}
	last := chunks[len(chunks)-1]
	sep := "\n\n"
	if !strings.Contains(part, "\n") && !strings.Contains(last, "\n") {
		sep = " "
	}
	if len([]rune(last))+len([]rune(sep))+len([]rune(part)) <= maxRunes {
		chunks[len(chunks)-1] = last + sep + part
		return chunks
	}
	return append(chunks, part)
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		if r != '.' && r != '!' && r != '?' && r != '\n' {
			continue
		}
		if i+1 < len(runes) && runes[i+1] != ' ' && runes[i+1] != '\n' {
			continue
		}
		part := strings.TrimSpace(string(runes[start : i+1]))
		if part != "" {
			out = append(out, part)
		}
		start = i + 1
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

func wavFromPCM16(pcm []byte, sampleRate int, channels int) []byte {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	if channels <= 0 {
		channels = 1
	}
	byteRate := sampleRate * channels * 2
	blockAlign := channels * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

func currentStoryTurnNumber(state *story.StoryState, loopTurn int) int {
	if state != nil {
		return state.LatestTurn + 1
	}
	if loopTurn <= 0 {
		return 1
	}
	return loopTurn
}

func writeBinaryAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeTextAtomic(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONFileAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeBinaryAtomic(path, data)
}

func refreshPacket(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg playConfig) (scene.StoryPacket, error) {
	runID, runDir := writerRunInfo(cfg)
	assembler := writerRefreshContextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.StoryCollection, cfg.Logger)
	if cfg.WriterScenaristCalls != 0 {
		assembler.Options.WriterLoreK = 4
	}
	scenarioContract := scenarioContractTextForPack(cfg.GroundLorePackID)
	logLoreFidelityContext(cfg.Logger, "writer", runID, 0, packet.ID, nodeID, cfg.GroundLorePackID, scenarioContract)
	writerContext, err := assembler.AssembleWriter(ctx, contextbundle.BuildRequest{
		Packet:               packet,
		NodeID:               nodeID,
		Decision:             decision,
		Bible:                cfg.Bible,
		ScenarioContractText: scenarioContract,
	})
	if err != nil {
		return scene.StoryPacket{}, err
	}
	scope, err := prepareStoryAgentScope(cfg, "writer", runID, packet, nodeID, writerContext.RenderText())
	if err != nil {
		return scene.StoryPacket{}, err
	}
	outputDir := filepath.Join(scope.Root, "scenarists")
	runner := claudeRunner()
	loreExpertTool := loreExpertToolRunner(cfg)
	toolRunner, err := scenaristToolRunner(cfg, writerContext.RenderText(), outputDir, scope, loreExpertTool)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	if toolRunner != nil && cfg.Backend == "claude" {
		return scene.StoryPacket{}, fmt.Errorf("scenarist Writer tool is only supported for codex backend; use -model-backend codex or -writer-scenarist-calls 0")
	}
	planner := cfg.WriterPlanner
	if planner == nil {
		planner = writerPlannerWithMaterialAccess(cfg.Backend, runner, scope, toolRunner, loreExpertTool, cfg.ScenaristRounds, cfg.Logger, runID)
	}
	if toolRunner != nil {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return scene.StoryPacket{}, err
		}
	}
	job, err := writer.Service{Planner: planner}.RefreshAsync(ctx, writer.Input{
		Current:        packet,
		LatestDecision: decisionText(decision.ChoiceID, decision.FreeText),
		ContextText:    writerContext.RenderText(),
		RunID:          runID,
		ArtifactDir:    runDir,
	})
	if err != nil {
		return scene.StoryPacket{}, writerFailure(err, runID, runDir)
	}
	waitCtx, cancel := context.WithTimeout(ctx, writer.DefaultAwaitTimeout())
	defer cancel()
	result := job.Await(waitCtx)
	if result.Err != nil {
		return result.Packet, writerFailure(result.Err, runID, runDir)
	}
	nextPacket := result.Packet
	if registry, err := actorRegistry(cfg.CardStore); err == nil {
		var removedNeeds []actors.UnsupportedActorNeed
		nextPacket, removedNeeds = actors.PruneUnsupportedPacketNeeds(nextPacket, registry)
		logUnsupportedActorNeeds(cfg.Logger, "writer.packet.actor_needs.filtered", "unsupported actor_needs filtered from Writer packet", 0, nextPacket.ID, "", removedNeeds)
	}
	return nextPacket, nil
}

func persistStoryTurn(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, nextNodeID string, nextDecision scene.PlayerDecision, committed bool, audioPath ...string) error {
	if cfg.Workspace == nil || state == nil || cfg.Bible == nil {
		return nil
	}
	turnAudioPath := ""
	if len(audioPath) > 0 {
		turnAudioPath = audioPath[0]
	}
	pendingRecord := story.TurnRecord{}
	if state.PendingTurnPath != "" {
		if record, err := cfg.Workspace.LoadPendingTurn(*state); err == nil {
			pendingRecord = record
			if turnAudioPath == "" {
				turnAudioPath = record.AudioPath
			}
		}
	}
	if story.PlayerDecisionEmpty(nextDecision) && state.HasPendingDecision() {
		nextDecision = state.PendingDecision
	}
	record := story.TurnRecord{
		Turn:         pendingRecord.Turn,
		StoryID:      state.StoryID,
		PacketID:     rollout.Commit.PacketID,
		NodeID:       rollout.Commit.NodeID,
		Decision:     rollout.Commit.PlayerDecision,
		NextDecision: nextDecision,
		Prose:        rollout.Draft.Prose,
		Choices:      rollout.Node.Choices,
		Commit:       rollout.Commit,
		Committed:    committed,
		NextNodeID:   nextNodeID,
		AudioPath:    filepath.ToSlash(turnAudioPath),
	}
	nextState, err := cfg.Workspace.RecordTurn(*state, packet, record)
	if err != nil {
		return err
	}
	*state = nextState
	logEvent(cfg.Logger, gamelog.Event{
		Category: "memory",
		Event:    "story.turn.persisted",
		Message:  "turn persisted to local story workspace",
		Turn:     nextState.LatestTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"committed":          committed,
			"next_node_id":       nextNodeID,
			"choice_id":          rollout.Commit.PlayerDecision.ChoiceID,
			"has_free_text":      rollout.Commit.PlayerDecision.FreeText != "",
			"next_choice_id":     nextDecision.ChoiceID,
			"next_has_free_text": nextDecision.FreeText != "",
		},
		Detail: map[string]any{
			"turn_record":   record,
			"next_decision": nextDecision,
		},
	})
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
	state.Status = story.StatusActive
	state.BlockedReason = ""
	state.BlockedAt = ""
	state.BlockedWriterRunID = ""
	state.BlockedWriterRunPath = ""
	if err := cfg.Workspace.SaveState(*state); err != nil {
		return err
	}
	logEvent(cfg.Logger, gamelog.Event{Category: "story", Event: "story.packet.saved", Message: "current story packet saved", PacketID: packet.ID, NodeID: nodeID})
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
	logEvent(cfg.Logger, gamelog.Event{
		Category: "memory",
		Event:    "story.index.rebuilt",
		Message:  "local story index rebuilt",
		Fields:   map[string]any{"document_count": len(docs), "collection": state.StoryCollection},
		Detail: map[string]any{
			"documents": docs,
		},
	})
	if cfg.Backend == "static" {
		return nil
	}
	client := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
	ingestReq := bridgeclient.StoryIndexIngestRequest{
		Collection: state.StoryCollection,
		Documents:  bridgeStoryDocs(docs),
		Replace:    false,
	}
	ingestResp, err := client.IngestStoryIndex(ctx, ingestReq)
	if err != nil {
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "qdrant",
			Event:    "story.index.ingest.failed",
			Message:  err.Error(),
			Fields:   map[string]any{"collection": state.StoryCollection},
			Detail: map[string]any{
				"request": ingestReq,
				"error":   err.Error(),
			},
		})
		return fmt.Errorf("refresh story collection %q: %w", state.StoryCollection, err)
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "qdrant",
		Event:    "story.index.ingested",
		Message:  "story index ingested",
		Fields:   map[string]any{"collection": state.StoryCollection, "document_count": len(docs), "response_count": ingestResp.Count},
		Detail: map[string]any{
			"request":  ingestReq,
			"response": ingestResp,
		},
	})
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

func sourceLoreCollection(state *story.StoryState, fallback string) string {
	if state != nil {
		if collection := state.SourceLoreCollection(); collection != "" {
			return collection
		}
	}
	return firstNonEmpty(fallback, story.DefaultGroundLoreCollection)
}

func sourceLorePackID(state *story.StoryState, fallback string) string {
	if state != nil {
		if packID := state.SourceLorePackID(); packID != "" {
			return packID
		}
	}
	return firstNonEmpty(fallback, story.DefaultGroundLorePackID)
}

func decisionForWriterRefresh(state *story.StoryState) scene.PlayerDecision {
	if state != nil && state.HasPendingDecision() {
		return state.PendingDecision
	}
	if state != nil {
		return state.LastDecision
	}
	return scene.PlayerDecision{}
}

func savePendingDecision(cfg playConfig, state *story.StoryState, decision scene.PlayerDecision, runID string, runDir string) error {
	if cfg.Workspace == nil || state == nil || story.PlayerDecisionEmpty(decision) {
		return nil
	}
	state.PendingDecision = decision
	state.PendingDecisionAt = story.NowString()
	state.PendingWriterRunID = runID
	state.PendingWriterRunPath = filepath.ToSlash(runDir)
	state.Status = story.StatusActive
	state.BlockedReason = ""
	state.BlockedAt = ""
	state.BlockedWriterRunID = ""
	state.BlockedWriterRunPath = ""
	if err := cfg.Workspace.SaveState(*state); err != nil {
		return err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Category: "memory",
		Event:    "story.pending_decision.saved",
		Message:  "player decision saved before Writer refresh",
		RunID:    runID,
		Fields: map[string]any{
			"choice_id":     decision.ChoiceID,
			"has_free_text": decision.FreeText != "",
			"artifact_dir":  filepath.ToSlash(runDir),
		},
	})
	return nil
}

func markStoryBlocked(cfg playConfig, state *story.StoryState, failure writerRefreshFailure) error {
	if cfg.Workspace == nil || state == nil {
		return nil
	}
	state.Status = story.StatusBlockedWriterRefresh
	state.BlockedReason = failure.Err.Error()
	state.BlockedAt = story.NowString()
	state.BlockedWriterRunID = failure.RunID
	state.BlockedWriterRunPath = filepath.ToSlash(failure.RunDir)
	if err := cfg.Workspace.SaveState(*state); err != nil {
		return err
	}
	logEvent(cfg.Logger, gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "story",
		Event:    "story.blocked",
		Message:  "story blocked by Writer refresh failure",
		RunID:    failure.RunID,
		Fields: map[string]any{
			"reason":       failure.Err.Error(),
			"artifact_dir": filepath.ToSlash(failure.RunDir),
		},
	})
	return nil
}

func writerFailure(err error, runID string, runDir string) writerRefreshFailure {
	if failure, ok := asWriterFailure(err); ok {
		if failure.RunID == "" {
			failure.RunID = runID
		}
		if failure.RunDir == "" {
			failure.RunDir = runDir
		}
		return failure
	}
	return writerRefreshFailure{Err: err, RunID: runID, RunDir: runDir}
}

func writerFailureDetails(failure writerRefreshFailure) map[string]any {
	detail := map[string]any{
		"error":        failure.Err.Error(),
		"run_id":       failure.RunID,
		"artifact_dir": filepath.ToSlash(failure.RunDir),
	}
	var missing *scene.MissingChoiceTargetError
	if errors.As(failure.Err, &missing) {
		detail["failure_kind"] = "missing_choice_target"
		detail["source_node_id"] = missing.NodeID
		detail["choice_id"] = missing.ChoiceID
		detail["missing_target_node_id"] = missing.TargetNodeID
		detail["known_node_ids"] = missing.KnownNodeIDs
	}
	return detail
}

func asWriterFailure(err error) (writerRefreshFailure, bool) {
	var failure writerRefreshFailure
	if errors.As(err, &failure) {
		return failure, true
	}
	return writerRefreshFailure{}, false
}

func storyLogger(workspace *story.Workspace, state *story.StoryState) *gamelog.Writer {
	if workspace == nil || state == nil {
		return nil
	}
	return gamelog.NewWriter(workspace.Dir, state.StoryID)
}

func logEvent(logger *gamelog.Writer, event gamelog.Event) {
	if logger == nil {
		return
	}
	_ = logger.Append(event)
}

func logLoreFidelityContext(logger *gamelog.Writer, role string, runID string, turn int, packetID string, nodeID string, packID string, scenarioContract string) {
	if logger == nil || strings.TrimSpace(scenarioContract) == "" {
		return
	}
	_ = logger.Append(gamelog.Event{
		Category: "lore",
		Event:    "lore.fidelity.context",
		Message:  "lore fidelity contract attached to " + role + " context",
		RunID:    runID,
		Turn:     turn,
		PacketID: packetID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"role":                role,
			"ground_lore_pack_id": packID,
			"context_chars":       len([]rune(scenarioContract)),
		},
		Detail: map[string]any{
			"role":                role,
			"ground_lore_pack_id": packID,
			"scenario_contract":   scenarioContract,
		},
	})
}

func logUnsupportedActorNeeds(logger *gamelog.Writer, event string, message string, turn int, packetID string, nodeID string, removed []actors.UnsupportedActorNeed) {
	if logger == nil || len(removed) == 0 {
		return
	}
	_ = logger.Append(gamelog.Event{
		Level:    gamelog.LevelWarn,
		Category: "actor",
		Event:    event,
		Message:  message,
		Turn:     turn,
		PacketID: packetID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"removed_count": len(removed),
		},
		Detail: map[string]any{
			"removed_actor_needs": removed,
		},
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
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

func resolvedChoiceTarget(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision) (string, bool) {
	if strings.TrimSpace(decision.ChoiceID) == "" {
		return "", false
	}
	choice, err := node.Choice(decision.ChoiceID)
	if err != nil || strings.TrimSpace(choice.TargetNodeID) == "" {
		return "", false
	}
	if _, err := packet.Node(choice.TargetNodeID); err != nil {
		return "", false
	}
	return choice.TargetNodeID, true
}

func isCosmeticIntroChoice(packet scene.StoryPacket, node scene.SceneNode) bool {
	entryID := entryNodeID(packet)
	if node.ID != entryID || len(node.Choices) < 2 || len(node.Choices) > 3 {
		return false
	}
	target := strings.TrimSpace(node.Choices[0].TargetNodeID)
	if target == "" || target == node.ID {
		return false
	}
	for _, choice := range node.Choices[1:] {
		if strings.TrimSpace(choice.TargetNodeID) != target {
			return false
		}
	}
	return true
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

func contextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore, globalCollection string, storyCollection string, logger *gamelog.Writer) contextbundle.Assembler {
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
			globalCollection = story.DefaultGroundLoreCollection
		}
		retrievers = append(retrievers, lore.BridgeRetriever{Client: client, Collection: globalCollection, Telemetry: logger})
		if storyCollection != "" && storyCollection != globalCollection {
			retrievers = append(retrievers, lore.BridgeRetriever{Client: client, Collection: storyCollection, Telemetry: logger})
		}
		if len(retrievers) == 1 {
			assembler.Lore = retrievers[0]
		} else {
			assembler.Lore = lore.MultiRetriever{Retrievers: retrievers}
		}
	}
	return assembler
}

func writerRefreshContextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore, storyCollection string, logger *gamelog.Writer) contextbundle.Assembler {
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
		if storyCollection != "" {
			assembler.Lore = lore.BridgeRetriever{Client: client, Collection: storyCollection, Telemetry: logger}
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

func passedFlagNames() map[string]struct{} {
	names := map[string]struct{}{}
	flag.Visit(func(f *flag.Flag) {
		names[f.Name] = struct{}{}
	})
	return names
}

func shouldRunLauncher(noArgs bool, passed map[string]struct{}) bool {
	if noArgs {
		return true
	}
	flowFlags := map[string]struct{}{
		"admin":            {},
		"context-preview":  {},
		"choice":           {},
		"lore-search":      {},
		"no-play":          {},
		"node":             {},
		"packet":           {},
		"play-trace-jsonl": {},
		"read-story":       {},
		"replace-story":    {},
		"seed-output":      {},
		"story-id":         {},
		"text":             {},
		"turns":            {},
	}
	for name := range passed {
		if _, ok := flowFlags[name]; ok {
			return false
		}
	}
	return true
}

func shouldUsePlayTUI(turns int, choiceID string, freeText string, traceJSONL bool) bool {
	if turns > 0 || strings.TrimSpace(choiceID) != "" || strings.TrimSpace(freeText) != "" || traceJSONL {
		return false
	}
	return interactiveStdin()
}

func interactiveStdin() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func isRecoverableDisconnect(err error) bool {
	if prompt.IsRetryableCodexError(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "/v1/scene/commit") &&
		(strings.Contains(message, "context deadline exceeded") ||
			strings.Contains(message, "client.timeout exceeded while awaiting headers"))
}

func waitForDisconnectRetry(ctx context.Context, stage string, err error, interactive bool) bool {
	if !interactive || !isRecoverableDisconnect(err) {
		return false
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "model call"
	}
	fmt.Fprintf(os.Stderr, "\nDisconnected during %s:\n%s\n\nPress Enter to continue, or Ctrl+C to exit.", stage, err.Error())
	done := make(chan struct{})
	go func() {
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		close(done)
	}()
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr)
		return false
	case <-done:
		fmt.Fprintln(os.Stderr)
		return true
	}
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

func narratorModelMetadata(cfg playConfig) modelmeta.Metadata {
	switch cfg.Backend {
	case "static":
		return narrator.StaticRenderer{}.ModelMetadata()
	case "claude":
		return claudeRunner().ModelMetadata()
	default:
		return codexRunner("NarratorDraft", prompt.NarratorDraftSchema()).ModelMetadata()
	}
}

func writerRefreshModelMetadata(cfg playConfig) modelmeta.Metadata {
	if cfg.WriterPlanner != nil {
		if provider, ok := cfg.WriterPlanner.(interface {
			ModelMetadata() modelmeta.Metadata
		}); ok {
			return provider.ModelMetadata()
		}
	}
	switch cfg.Backend {
	case "static":
		return prompt.StaticRunner{}.ModelMetadata()
	case "claude":
		return claudeRunner().ModelMetadata()
	default:
		return codexRunner("StoryPacket", prompt.StoryPacketSchema()).ModelMetadata()
	}
}

func writerPlannerWithMaterialAccess(backend string, claude prompt.ClaudeRunner, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, maxToolRounds int, logger *gamelog.Writer, runID string) writer.Planner {
	switch backend {
	case "static":
		return writer.StaticPlanner{}
	case "claude":
		if scope.Root != "" {
			claude.PermissionMode = "dontAsk"
			claude.AllowedTools = []string{"Read", "LS", "Grep"}
			claude.WorkDir = scope.Root
			claude.AddDirs = append(claude.AddDirs, scope.Root)
		}
		return writer.ClaudePlanner{Runner: claude}
	default:
		runner := codexRunnerWithMaterialAccess("StoryPacket", prompt.StoryPacketSchema(), scope, scenaristTool, loreExpertTool, maxToolRounds)
		runner = configureCodexToolLogging(runner, logger, "writer", "writer", "Writer", runID, 0, "", "", "")
		return writer.ClaudePlanner{Runner: runner}
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

func codexRunnerWithMaterialAccess(schemaName string, schema map[string]any, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, maxToolRounds int) prompt.CodexRunner {
	runner := codexRunner(schemaName, schema)
	if strings.TrimSpace(scope.Root) != "" || scenaristTool != nil || loreExpertTool != nil {
		if strings.TrimSpace(scope.Root) == "" {
			scope = agentfs.New(filepath.Join(os.TempDir(), "sans-nom-agent", "scratch"), "model").WithVisiblePaths(".")
		}
		_ = scope.Ensure()
		runner.LocalTools = true
		runner.ToolRoot = scope.Root
		runner.ToolVisiblePaths = append([]string(nil), scope.VisiblePaths...)
		if maxToolRounds <= 0 {
			maxToolRounds = 24
		}
		runner.MaxToolRounds = maxToolRounds
		var tools []codexoauth.LocalTool
		if scenaristTool != nil {
			tools = append(tools, scenaristTool.LocalTool())
		}
		if loreExpertTool != nil {
			tools = append(tools, loreExpertTool.LocalTool())
		}
		runner.ExtraTools = tools
	}
	return runner
}

func codexRunnerWithReadTools(schemaName string, schema map[string]any, scope agentfs.Scope, logger *gamelog.Writer, category string, role string, speaker string, turn int, packetID string, nodeID string) prompt.CodexRunner {
	runner := codexRunner(schemaName, schema)
	if strings.TrimSpace(scope.Root) == "" {
		scope = agentfs.New(filepath.Join(os.TempDir(), "sans-nom-agent", "read-tools"), role).WithVisiblePaths(".")
	}
	_ = scope.Ensure()
	runner.LocalTools = true
	runner.ToolRoot = scope.Root
	runner.ToolVisiblePaths = append([]string(nil), scope.VisiblePaths...)
	runner.MaxToolRounds = 8
	return configureCodexToolLogging(runner, logger, category, role, speaker, "", turn, packetID, nodeID, "")
}

func configureCodexToolLogging(runner prompt.CodexRunner, logger *gamelog.Writer, category string, role string, speaker string, runID string, turn int, packetID string, nodeID string, storyID string) prompt.CodexRunner {
	runner.Logger = logger
	runner.ToolCategory = category
	runner.CallerRole = role
	runner.CallerSpeaker = speaker
	runner.RunID = runID
	runner.Turn = turn
	runner.PacketID = packetID
	runner.NodeID = nodeID
	runner.StoryID = storyID
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

func scenaristToolRunner(cfg playConfig, contextText string, outputDir string, scope agentfs.Scope, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	if cfg.WriterScenaristCalls == 0 || strings.TrimSpace(outputDir) == "" {
		return nil, nil
	}
	runners := scenaristProviderRunners(cfg, outputDir, scope.Root)
	if len(runners) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(scope.Root) == "" {
		scope = agentfs.New(filepath.Dir(outputDir), "scenarist").WithVisiblePaths(".")
	}
	return &scenarist.ToolRunner{
		Runner:        &scenarist.ProviderPoolRunner{Runners: runners},
		OutputDir:     outputDir,
		RepoRoot:      repoRoot(),
		WorkspaceRoot: scope.Root,
		VisiblePaths:  scope.VisiblePaths,
		ContextText:   contextText,
		Tools:         cfg.ScenaristTools,
		LoreExpert:    loreExpertTool,
		MaxCalls:      cfg.WriterScenaristCalls,
		Logger:        cfg.Logger,
		CallerRole:    "writer",
		CallerName:    "Writer",
	}, nil
}

func loreExpertToolRunner(cfg playConfig) *loreexpert.ToolRunner {
	if cfg.Backend == "static" || cfg.Backend == "claude" {
		return nil
	}
	packID := firstNonEmpty(cfg.GroundLorePackID, story.DefaultGroundLorePackID)
	pack, err := lorepack.GetPack(repoRoot(), packID)
	if err != nil {
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "lore",
			Event:    "lore.expert.pack_unavailable",
			Message:  err.Error(),
			Fields:   map[string]any{"pack_id": packID},
		})
		return nil
	}
	collection := firstNonEmpty(cfg.GlobalCollection, pack.Collection, story.DefaultGroundLoreCollection)
	packDir := filepath.Dir(pack.Path)
	return &loreexpert.ToolRunner{
		Runner:            loreExpertModelRunner(),
		Retriever:         seedLoreRetriever(cfg.BridgeURL, collection),
		CacheRoot:         filepath.Join(repoRoot(), ".cache", "sans-nom", "lore-expert"),
		RepoRoot:          repoRoot(),
		PackID:            pack.ID,
		PackLabel:         pack.Label,
		PackDescription:   pack.Description,
		Collection:        collection,
		PackManifestPath:  pack.Path,
		IndexManifestPath: filepath.Join(packDir, "index", "manifest.json"),
		MaxSources:        6,
		Logger:            cfg.Logger,
		CallerRole:        "writer",
		CallerSpeaker:     "Writer",
	}
}

func loreExpertModelRunner() prompt.CodexRunner {
	runner := codexRunner("", nil)
	if model := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_MODEL")); model != "" {
		runner.Model = model
	} else {
		runner.Model = "gpt-5.3-codex-spark"
	}
	if effort := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_EFFORT")); effort != "" {
		runner.Effort = codexLoreExpertEffort(effort)
	} else {
		runner.Effort = "low"
	}
	if tier := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_SERVICE_TIER")); tier != "" {
		runner.ServiceTier = tier
	}
	return runner
}

func codexLoreExpertEffort(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "minimal" {
		return "low"
	}
	return value
}

func scenaristProviderRunners(cfg playConfig, outputDir string, workspaceRoot string) []scenarist.ProviderRunner {
	names := scenarist.ProviderNames(cfg.ScenaristProvider)
	names = scenarist.UsableProviders(names, cfg.ScenaristModel, scenarist.DefaultEnvFiles("."), os.Environ())
	runners := make([]scenarist.ProviderRunner, 0, len(names))
	workDir := scenaristWorkDir(outputDir, workspaceRoot)
	for _, name := range names {
		runners = append(runners, scenarist.ProviderRunner{
			Provider: name,
			Model:    cfg.ScenaristModel,
			WorkDir:  workDir,
			EnvFiles: scenarist.DefaultEnvFiles("."),
			Tools:    cfg.ScenaristTools,
			AddDirs:  scenaristAddDirs(workDir),
			Timeout:  15 * time.Minute,
			Logger:   cfg.Logger,
		})
	}
	return runners
}

func scenaristWorkDir(outputDir string, workspaceRoot string) string {
	if strings.TrimSpace(workspaceRoot) != "" {
		return absOrClean(workspaceRoot)
	}
	if strings.TrimSpace(outputDir) == "" {
		return "."
	}
	return absOrClean(filepath.Dir(outputDir))
}

func scenaristAddDirs(workDir string) []string {
	dirs := []string{}
	if strings.TrimSpace(workDir) != "" {
		dirs = append(dirs, absOrClean(workDir))
	}
	return dirs
}

func absOrClean(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func writerRunInfo(cfg playConfig) (string, string) {
	segment := scenaristTurnSegment(cfg)
	if cfg.Workspace == nil {
		return segment, ""
	}
	return segment, filepath.Join(cfg.Workspace.ModelRunsDir(), "writer", segment)
}

func prepareStoryAgentScope(cfg playConfig, role string, runID string, packet scene.StoryPacket, nodeID string, contextText string) (agentfs.Scope, error) {
	root := ""
	storyID := ""
	if cfg.Workspace != nil {
		root = filepath.Join(cfg.Workspace.Dir, "agent", story.SanitizeStoryID(role), story.SanitizeStoryID(runID))
		storyID = cfg.Workspace.StoryID
	}
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(os.TempDir(), "sans-nom-agent", story.SanitizeStoryID(role), story.SanitizeStoryID(runID))
	}
	scope := agentfs.New(root, role).WithStoryID(storyID).WithVisiblePaths("story", "scenarists")
	if err := scope.Ensure(); err != nil {
		return scope, err
	}
	packID := firstNonEmpty(cfg.GroundLorePackID, lorepack.DefaultPackID())
	if cfg.State != nil {
		packID = firstNonEmpty(cfg.State.SourceLorePackID(), packID)
	}
	if stagedScope, err := stageLorePackForAgentByID(scope, packID); err == nil {
		scope = stagedScope
	} else {
		logEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "lore",
			Event:    "lorepack.agent_stage_failed",
			Message:  err.Error(),
			Fields: map[string]any{
				"ground_lore_pack_id":  packID,
				"agent_workspace_root": filepath.ToSlash(scope.Root),
			},
		})
	}
	if cfg.State != nil {
		if _, err := scope.StageJSON(filepath.Join("story", "state.json"), cfg.State); err != nil {
			return scope, err
		}
	}
	if cfg.Bible != nil {
		if _, err := scope.StageJSON(filepath.Join("story", "bible.json"), cfg.Bible); err != nil {
			return scope, err
		}
	}
	if _, err := scope.StageJSON(filepath.Join("story", "current-packet.json"), packet); err != nil {
		return scope, err
	}
	if node, err := packet.Node(nodeID); err == nil {
		if _, err := scope.StageJSON(filepath.Join("story", "current-node.json"), node); err != nil {
			return scope, err
		}
	}
	if strings.TrimSpace(contextText) != "" {
		if _, err := scope.StageText(filepath.Join("story", "context.txt"), contextText); err != nil {
			return scope, err
		}
	}
	if cfg.Workspace != nil {
		if err := stageNarrationFiles(scope, cfg.Workspace.NarrationDir()); err != nil {
			return scope, err
		}
	}
	return scope, nil
}

func stageNarrationFiles(scope agentfs.Scope, narrationDir string) error {
	entries, err := os.ReadDir(narrationDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		source := filepath.Join(narrationDir, entry.Name())
		if _, err := scope.StageFile(source, filepath.Join("story", "narration", entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func scenaristTurnSegment(cfg playConfig) string {
	turn := 1
	if cfg.State != nil && cfg.State.LatestTurn >= 0 {
		turn = cfg.State.LatestTurn + 1
	}
	return fmt.Sprintf("turn-%06d", turn)
}

func resolveWriterScenaristCallCount(value int, backend string, writerRefreshEnabled bool) int {
	if value >= 0 {
		if value > scenarist.MaxToolCalls {
			return scenarist.MaxToolCalls
		}
		return value
	}
	if writerRefreshEnabled && backend != "static" {
		return scenarist.MaxToolCalls
	}
	return 0
}

func playTTSConfigFromFlags(enabled bool, baseURL string, model string, voice string, format string, instruction string, outputDir string) playTTSConfig {
	return playTTSConfig{
		Enabled:     enabled,
		BaseURL:     firstNonEmpty(baseURL, ttsclient.DefaultBaseURL),
		Model:       firstNonEmpty(model, ttsclient.DefaultModel),
		Voice:       firstNonEmpty(voice, ttsclient.DefaultVoice),
		Format:      firstNonEmpty(format, ttsclient.DefaultAudioFormat),
		Instruction: firstNonEmpty(instruction, ttsclient.DefaultNarratorInstruction),
		OutputDir:   outputDir,
		Timeout:     time.Duration(envInt("SANS_NOM_TTS_TIMEOUT_MS", int(ttsclient.DefaultTimeout/time.Millisecond))) * time.Millisecond,
	}
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envBoolDefault(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
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
	for dir := wd; dir != ""; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
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
		"",
		filepath.Join("lore", "packs", "metabarons", "comics", "peer-cards", "cards", "accepted"),
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
