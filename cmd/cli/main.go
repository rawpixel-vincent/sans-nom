package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/app"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/clitui"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
	ttsclient "github.com/rawpixel-vincent/sans-nom/internal/tts"
)

func main() {
	app.LoadDotEnv(".env")
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
	scenaristProvider := flag.String("scenarist-provider", app.DefaultScenaristProviderPool(), "Claude-compatible scenarist provider pool, comma-separated, for example ZAI,MINIMAX")
	scenaristModel := flag.String("scenarist-model", os.Getenv("SANS_NOM_SCENARIST_MODEL"), "Claude-compatible scenarist model override")
	scenaristTools := flag.Bool("scenarist-tools", app.EnvBoolDefault("SANS_NOM_SCENARIST_TOOLS", true), "allow Scenarists read-only lore tools")
	scenaristOutputRoot := flag.String("scenarist-output-root", "", "optional root for Scenarist material output")
	writerScenaristRounds := flag.Int("writer-scenarist-rounds", app.EnvInt("SANS_NOM_WRITER_SCENARIST_ROUNDS", 24), "maximum Writer tool rounds while refreshing; 0 uses the 24-round safety cap")
	noPlay := flag.Bool("no-play", false, "do not enter the interactive play loop after loading or creating a story")
	noCommit := flag.Bool("no-commit", false, "do not commit witnessed scene canon to Honcho during play")
	turns := flag.Int("turns", 0, "number of automated play turns; 0 reads interactively")
	playTraceJSONL := flag.Bool("play-trace-jsonl", false, "emit machine-readable JSONL trace events during play")
	readTurnTTS := flag.Bool("tts", app.EnvBool("SANS_NOM_TTS"), "synthesize each narrator turn with Xiaomi MiMo v2.5 TTS")
	ttsBaseURL := flag.String("tts-base-url", app.FirstNonEmpty(os.Getenv("SANS_NOM_TTS_BASE_URL"), ttsclient.DefaultBaseURL), "MiMo OpenAI-compatible TTS base URL")
	ttsModel := flag.String("tts-model", app.FirstNonEmpty(os.Getenv("SANS_NOM_TTS_MODEL"), ttsclient.DefaultModel), "MiMo TTS model")
	ttsVoice := flag.String("tts-voice", app.FirstNonEmpty(os.Getenv("SANS_NOM_TTS_VOICE"), ttsclient.DefaultVoice), "MiMo TTS voice id")
	ttsFormat := flag.String("tts-format", app.FirstNonEmpty(os.Getenv("SANS_NOM_TTS_FORMAT"), ttsclient.DefaultAudioFormat), "TTS output format: wav or pcm16")
	ttsInstruction := flag.String("tts-instruction", app.FirstNonEmpty(os.Getenv("SANS_NOM_TTS_INSTRUCTION"), ttsclient.DefaultNarratorInstruction), "MiMo TTS style/director instruction; spoken text stays in the assistant message")
	ttsOutputDir := flag.String("tts-output-dir", os.Getenv("SANS_NOM_TTS_OUTPUT_DIR"), "optional TTS output directory; defaults to the story audio directory")
	loreSearch := flag.String("lore-search", "", "query the runtime lore retriever and exit")
	loreK := flag.Int("k", 5, "number of lore search results")
	bridgeURL := flag.String("bridge-url", os.Getenv("SANS_NOM_BRIDGE_URL"), "bridge sidecar URL")
	adminMode := flag.Bool("admin", false, "start the local admin web UI and do not enter the game loop")
	adminAddr := flag.String("admin-addr", "127.0.0.1:8790", "local admin web UI listen address")
	contextPreview := flag.String("context-preview", "", "preview assembled context: narrator or writer")
	contextText := flag.Bool("context-text", false, "print model-ready context text instead of pretty JSON")
	flag.Parse()
	packetExplicit := app.FlagPassed("packet")
	groundPack, err := lorepack.GetPack(app.RepoRoot(), *groundLorePackID)
	if err != nil {
		log.Fatal(err)
	}
	groundLoreCollection := groundPack.Collection
	backend := app.ResolveBackend(*modelBackend, *useClaude)
	resolvedBridgeURL := app.RuntimeBridgeURL(backend, *bridgeURL)

	if *loreSearch != "" {
		loreBridgeURL := app.FirstNonEmpty(resolvedBridgeURL, bridgeclient.NewClient("", "").BaseURL)
		stopBridge, err := app.EnsureBridge(context.Background(), "codex", loreBridgeURL)
		if err != nil {
			log.Fatal(err)
		}
		defer stopBridge()
		if err := app.RunLoreSearch(context.Background(), loreBridgeURL, groundLoreCollection, *loreSearch, *loreK); err != nil {
			log.Fatal(err)
		}
		return
	}

	cardStore, err := app.LoadCardStore()
	if err != nil {
		log.Fatal(err)
	}
	if *adminMode {
		if err := app.RunAdmin(context.Background(), app.AdminConfig{
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
			WriterScenaristCalls: app.ResolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
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
		if err := app.PrintStoryNarration(workspace); err != nil {
			log.Fatal(err)
		}
		return
	}
	playMode := !*noPlay && *contextPreview == ""
	commitMode := !*noCommit
	playRefreshWriter := !*noWriterRefresh
	if app.ShouldRunLauncher(noArgs, app.PassedFlagNames()) {
		ttsCfg := app.PlayTTSConfigFromFlags(*readTurnTTS, *ttsBaseURL, *ttsModel, *ttsVoice, *ttsFormat, *ttsInstruction, *ttsOutputDir)
		if err := runLauncher(context.Background(), launcherRuntime{
			backend:              backend,
			bridgeURL:            resolvedBridgeURL,
			cardStore:            cardStore,
			storyRoot:            *storyRoot,
			groundLorePackID:     groundPack.ID,
			globalCollection:     groundLoreCollection,
			storyCollection:      *storyCollection,
			writerScenaristCalls: app.ResolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
			scenaristProvider:    *scenaristProvider,
			scenaristModel:       *scenaristModel,
			scenaristTools:       *scenaristTools,
			scenaristOutputRoot:  *scenaristOutputRoot,
			scenaristRounds:      *writerScenaristRounds,
			refreshWrite:         playRefreshWriter,
			commit:               commitMode,
			tts:                  ttsCfg,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	createStory := app.ShouldCreateStory(*storyID, *packetPath, *storyRoot, *replaceStory)
	stopBridge, err := app.EnsureFlagBridge(context.Background(), app.FlagBridgeConfig{
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
		session, err := app.NewStorySessionWithDisconnectRetry(context.Background(), storyapp.StoryActionOptions{
			StoryID:              *storyID,
			StoryRoot:            *storyRoot,
			GroundLorePackID:     groundPack.ID,
			GroundLoreCollection: groundLoreCollection,
			StoryCollection:      *storyCollection,
			ReplaceStory:         *replaceStory,
		}, app.SeedRuntimeConfig{
			Backend:              backend,
			BridgeURL:            resolvedBridgeURL,
			GroundLorePackID:     groundPack.ID,
			WriterScenaristCalls: app.ResolveWriterScenaristCallCount(*writerScenaristCalls, backend, true),
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			InteractiveRetry:     app.ShouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL),
		}, app.ShouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL))
		if err != nil {
			log.Fatal(err)
		}
		workspace = &session.Workspace
		state = &session.State
		bible = &session.Bible
		packet = session.Packet
		bundle = &session.Bundle
		if *seedOutput != "" {
			if err := app.PrintSeedBundle(*bundle, *seedOutput); err != nil {
				log.Fatal(err)
			}
		}
		if *nodeID == "" {
			*nodeID = session.NodeID
		}
		if !playMode && *contextPreview == "" {
			if *seedOutput == "" {
				if err := app.PrintSeedBundle(*bundle, ""); err != nil {
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
		if err := app.ValidatePacketSourceForBackend(backend, packetExplicit, *packetPath); err != nil {
			log.Fatal(err)
		}
		packet, err = app.OpenPacket(*packetPath)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("no story source: run without flags for the live menu, or pass -story-id or -packet")
	}
	if workspace != nil {
		cardStore, err = contextbundle.LoadStoryCardStore(cardStore, workspace.Dir)
		if err != nil {
			log.Fatal(err)
		}
	}
	decision := scene.PlayerDecision{
		ChoiceID: *choiceID,
		FreeText: *freeText,
	}
	if playMode {
		ttsCfg := app.PlayTTSConfigFromFlags(*readTurnTTS, *ttsBaseURL, *ttsModel, *ttsVoice, *ttsFormat, *ttsInstruction, *ttsOutputDir)
		if err := app.RunPlay(context.Background(), app.PlayConfig{
			Packet:               packet,
			NodeID:               *nodeID,
			Backend:              backend,
			BridgeURL:            resolvedBridgeURL,
			CardStore:            cardStore,
			Workspace:            workspace,
			State:                state,
			Bible:                bible,
			GroundLorePackID:     app.SourceLorePackID(state, groundPack.ID),
			GlobalCollection:     app.SourceLoreCollection(state, groundLoreCollection),
			StoryCollection:      app.ResolvedStoryCollection(state, *storyCollection),
			InitialDecision:      decision,
			Turns:                *turns,
			Commit:               commitMode,
			RefreshWrite:         playRefreshWriter,
			TraceJSONL:           *playTraceJSONL,
			UseTUI:               app.ShouldUsePlayTUI(*turns, *choiceID, *freeText, *playTraceJSONL),
			WriterScenaristCalls: app.ResolveWriterScenaristCallCount(*writerScenaristCalls, backend, playRefreshWriter),
			ScenaristProvider:    *scenaristProvider,
			ScenaristModel:       *scenaristModel,
			ScenaristTools:       *scenaristTools,
			ScenaristOutputRoot:  *scenaristOutputRoot,
			ScenaristRounds:      *writerScenaristRounds,
			Logger:               app.StoryLogger(workspace, state),
			TTS:                  ttsCfg,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *contextPreview != "" {
		if err := app.RunContextPreview(context.Background(), *contextPreview, *contextText, resolvedBridgeURL, packet, *nodeID, decision, bible, app.SourceLoreCollection(state, groundLoreCollection), app.ResolvedStoryCollection(state, *storyCollection), cardStore); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := app.RunOneShotRollout(context.Background(), app.OneShotRolloutConfig{
		Packet:           packet,
		NodeID:           *nodeID,
		Decision:         decision,
		Backend:          backend,
		BridgeURL:        resolvedBridgeURL,
		PacketPath:       *packetPath,
		CardStore:        cardStore,
		State:            state,
		Bible:            bible,
		GlobalCollection: app.SourceLoreCollection(state, groundLoreCollection),
		StoryCollection:  app.ResolvedStoryCollection(state, *storyCollection),
	}); err != nil {
		log.Fatal(err)
	}
}

type launcherRuntime struct {
	backend              string
	bridgeURL            string
	cardStore            *contextbundle.LocalCardStore
	storyRoot            string
	groundLorePackID     string
	globalCollection     string
	storyCollection      string
	writerScenaristCalls int
	scenaristProvider    string
	scenaristModel       string
	scenaristTools       bool
	scenaristOutputRoot  string
	scenaristRounds      int
	refreshWrite         bool
	commit               bool
	tts                  app.PlayTTSConfig
}

func runLauncher(ctx context.Context, cfg launcherRuntime) error {
	stopBridge, err := app.EnsureBridge(ctx, cfg.backend, cfg.bridgeURL)
	if err != nil {
		return err
	}
	defer stopBridge()

	action, err := clitui.Run(ctx, cfg.storyRoot, lorepack.ListPacks)
	if err != nil {
		return err
	}
	switch action.Kind {
	case clitui.ActionNewStory:
		packID := app.FirstNonEmpty(action.GroundLorePackID, cfg.groundLorePackID, lorepack.DefaultPackID())
		pack, err := lorepack.GetPack(app.RepoRoot(), packID)
		if err != nil {
			return err
		}
		groundCollection := cfg.globalCollection
		if groundCollection == "" || pack.ID != cfg.groundLorePackID {
			groundCollection = pack.Collection
		}
		session, err := app.NewStorySessionWithDisconnectRetry(ctx, storyapp.StoryActionOptions{
			StoryRoot:            cfg.storyRoot,
			GroundLorePackID:     pack.ID,
			GroundLoreCollection: groundCollection,
			StoryCollection:      cfg.storyCollection,
		}, app.SeedRuntimeConfig{
			Backend:              cfg.backend,
			BridgeURL:            cfg.bridgeURL,
			GroundLorePackID:     pack.ID,
			WriterScenaristCalls: cfg.writerScenaristCalls,
			ScenaristProvider:    cfg.scenaristProvider,
			ScenaristModel:       cfg.scenaristModel,
			ScenaristTools:       cfg.scenaristTools,
			ScenaristOutputRoot:  cfg.scenaristOutputRoot,
			ScenaristRounds:      cfg.scenaristRounds,
			InteractiveRetry:     true,
		}, true)
		if err != nil {
			return err
		}
		return app.RunPlay(ctx, playConfigFromSession(session, cfg))
	case clitui.ActionResumeStory:
		session, err := storyapp.ResumeStory(ctx, action.StoryID, storyapp.StoryActionOptions{StoryRoot: cfg.storyRoot})
		if err != nil {
			return err
		}
		return app.RunPlay(ctx, playConfigFromSession(session, cfg))
	case clitui.ActionExit, clitui.ActionNone:
		return nil
	default:
		return nil
	}
}

func playConfigFromSession(session storyapp.StorySession, cfg launcherRuntime) app.PlayConfig {
	workspace := session.Workspace
	state := session.State
	bible := session.Bible
	return app.PlayConfig{
		Packet:               session.Packet,
		NodeID:               session.NodeID,
		Backend:              cfg.backend,
		BridgeURL:            cfg.bridgeURL,
		CardStore:            cfg.cardStore,
		Workspace:            &workspace,
		State:                &state,
		Bible:                &bible,
		GroundLorePackID:     state.SourceLorePackID(),
		GlobalCollection:     app.SourceLoreCollection(&state, cfg.globalCollection),
		StoryCollection:      app.ResolvedStoryCollection(&state, cfg.storyCollection),
		Commit:               cfg.commit,
		RefreshWrite:         cfg.refreshWrite,
		UseTUI:               true,
		WriterScenaristCalls: cfg.writerScenaristCalls,
		ScenaristProvider:    cfg.scenaristProvider,
		ScenaristModel:       cfg.scenaristModel,
		ScenaristTools:       cfg.scenaristTools,
		ScenaristOutputRoot:  cfg.scenaristOutputRoot,
		ScenaristRounds:      cfg.scenaristRounds,
		Logger:               app.StoryLogger(&workspace, &state),
		TTS:                  cfg.tts,
	}
}
