package seedrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelrun"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/seed"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

type Config struct {
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

func NewStorySession(ctx context.Context, opts storyapp.StoryActionOptions, cfg Config) (storyapp.StorySession, error) {
	globalLogger := gamelog.NewGlobalWriter(opts.StoryRoot)
	runutil.LogEvent(globalLogger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.started",
		Message:  "story seed generation started",
		Fields: map[string]any{
			"backend":                cfg.Backend,
			"ground_lore_pack_id":    opts.GroundLorePackID,
			"ground_lore_collection": runutil.FirstNonEmpty(opts.GroundLoreCollection, opts.GlobalLoreCollection, story.DefaultGroundLoreCollection),
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
		return GenerateBundle(ctx, cfg, action.StoryRoot, action.GroundLorePackID, runutil.FirstNonEmpty(action.GroundLoreCollection, action.GlobalLoreCollection, story.DefaultGroundLoreCollection), globalLogger)
	}
	opts.SeedExternal = func(ctx context.Context, session storyapp.StorySession, bundle story.SeedBundle) error {
		logger := runutil.StoryLogger(&session.Workspace, &session.State)
		err := MaybeSeedExternalStory(ctx, cfg.Backend, cfg.BridgeURL, session.Workspace, session.State, bundle, logger)
		if err != nil {
			runutil.LogEvent(globalLogger, gamelog.Event{
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
		runutil.LogEvent(globalLogger, gamelog.Event{
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
	logger := runutil.StoryLogger(&session.Workspace, &session.State)
	runutil.LogEvent(globalLogger, gamelog.Event{
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
	runutil.LogEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.generated",
		Message:  "story bible and opening packet generated",
		PacketID: session.Packet.ID,
		NodeID:   session.NodeID,
		Fields: map[string]any{
			"title":               session.Bible.Title,
			"backend":             cfg.Backend,
			"story_context_count": len(session.Bible.StoryContextGuidance()),
			"node_count":          len(session.Packet.Nodes),
		},
	})
	runutil.LogEvent(logger, gamelog.Event{
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

func NewStorySessionWithDisconnectRetry(ctx context.Context, opts storyapp.StoryActionOptions, cfg Config, interactive bool) (storyapp.StorySession, error) {
	for {
		session, err := NewStorySession(ctx, opts, cfg)
		if err == nil {
			return session, nil
		}
		if !runutil.WaitForDisconnectRetry(ctx, "story seed", err, interactive) {
			return storyapp.StorySession{}, err
		}
	}
}

func GenerateBundle(ctx context.Context, cfg Config, storyRoot string, groundPackID string, globalCollection string, logger *gamelog.Writer) (story.SeedBundle, error) {
	pack, err := lorepack.GetPack(runutil.RepoRoot(), runutil.FirstNonEmpty(groundPackID, cfg.GroundLorePackID, lorepack.DefaultPackID()))
	if err != nil {
		return story.SeedBundle{}, err
	}
	if strings.TrimSpace(globalCollection) == "" {
		globalCollection = pack.Collection
	}
	runutil.LogEvent(logger, gamelog.Event{
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
	contextText := BuildContextForPack(pack)
	scenaristOutputDir := seedScenaristOutputDir(storyRoot, cfg.ScenaristOutputRoot)
	agentScope := seedAgentScopeFromScenaristOutputDir(scenaristOutputDir)
	agentScope, err = lorestage.StageLorePackForAgent(agentScope, pack)
	if err != nil {
		return story.SeedBundle{}, err
	}
	loreExpertTool := seedLoreExpertToolRunner(cfg, pack, globalCollection, logger)
	var scenaristTool *scenarist.ToolRunner
	var workshopRunner prompt.Runner
	workshopOutputDir := ""
	if !lorestage.ScenarioContractSeedMode(pack) {
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
	verifierRunner, err := seedVerifierRunner(cfg, pack, agentScope, loreExpertTool, logger)
	if err != nil {
		return story.SeedBundle{}, err
	}
	runutil.LogEvent(logger, gamelog.Event{
		Category: "story",
		Event:    "story.seed.context.completed",
		Message:  "seed context ready",
		Fields: map[string]any{
			"backend":                     cfg.Backend,
			"collection":                  globalCollection,
			"ground_lore_pack_id":         pack.ID,
			"context_chars":               len(contextText),
			"writer_scenarist_calls":      cfg.WriterScenaristCalls,
			"scenarist_output_dir":        filepath.ToSlash(scenaristOutputDir),
			"workshop_output_dir":         filepath.ToSlash(workshopOutputDir),
			"scenario_contract_seed_mode": lorestage.ScenarioContractSeedMode(pack),
			"agent_workspace_root":        filepath.ToSlash(agentScope.Root),
			"writer_tools_attached":       modelrun.RunnerHasLocalTools(writerRunner),
			"verifier_tools_attached":     modelrun.RunnerHasLocalTools(verifierRunner),
			"scenarist_tool_attached":     scenaristTool != nil,
			"lore_expert_tool_attached":   loreExpertTool != nil,
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
		Verifier:        verifierRunner,
		MaxIterations:   3,
		Logger:          logger,
		WorkshopDir:     workshopOutputDir,
		AgentRoot:       agentScope.Root,
		RecoverRunError: seedRunErrorRecoverer(ctx, cfg),
	}.Generate(ctx, contextText)
}

func seedRunErrorRecoverer(ctx context.Context, cfg Config) func(string, error) bool {
	if !cfg.InteractiveRetry {
		return nil
	}
	return func(stage string, err error) bool {
		return runutil.WaitForDisconnectRetry(ctx, stage, err, true)
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

func BuildContext(ctx context.Context, retriever lore.Retriever) string {
	_ = ctx
	_ = retriever
	pack, err := lorepack.GetPack(runutil.RepoRoot(), lorepack.DefaultPackID())
	if err != nil {
		return BuildContextForPack(lorepack.Pack{
			ID:          story.DefaultGroundLorePackID,
			Label:       "Metabarons",
			Description: "Default lorepack for Sans-Nom stories.",
		})
	}
	return BuildContextForPack(pack)
}

func BuildContextForPack(pack lorepack.Pack) string {
	var b strings.Builder
	b.WriteString(seed.ContextText())
	if scenario := pack.ScenarioOrientationText(); scenario != "" {
		b.WriteString("\n\n")
		b.WriteString(scenario)
		b.WriteString("\n\nSeed fidelity instructions:\n")
		b.WriteString("- Treat the lorepack scenario notes as the source of truth for premise logic, protagonist pressure, opening pressure, and choice logic.\n")
		b.WriteString("- The seed runs inside a game loop: the StoryBible provides durable story context, the StoryPacket is a bounded node graph, choices target included nodes, memory intents decide what can become shared story state, and terminal handoffs preserve player-facing momentum for later Writers.\n")
		b.WriteString("- Keep the player character central, reach agency quickly, and keep the pace active; do not explain game mechanics to the player unless the scene itself makes them visible.\n")
		b.WriteString("- Before drafting, orient in the staged lorepack: read lorepack/navigation.md, then lorepack/lore-pack.json, then inspect reference files enough to understand major characters, premise pressures, decisive events, and choice texture, then stage that understanding into playable opening turns.\n")
		b.WriteString("- Follow the lorepack/navigation.md minimum orientation: contract, wiki overview, comics manifest, relevant volume summaries, and matching character records before drafting, unless ask_lore_expert resolves a narrow missing fact.\n")
		b.WriteString("- Search with the Grep tool or rg-style commands; do not rely on shell grep.\n")
		b.WriteString("- Keep the contract's pressure alive through scene choices without copying its synopsis wording into every node.\n")
		b.WriteString("- Use ask_lore_expert as lore reference support when fidelity is uncertain.\n")
		b.WriteString("- Treat legacy scenario-note fields as context; use avoid-drift notes and canon anchors as source-fidelity boundaries.\n")
		return b.String()
	}
	b.WriteString("\n\nLegacy ground-lore influence summary:\n")
	if pack.ID != "" || pack.Label != "" {
		fmt.Fprintf(&b, "- selected lore pack: %s (%s)\n", runutil.FirstNonEmpty(pack.Label, pack.ID), runutil.FirstNonEmpty(pack.ID, "unknown"))
	}
	b.WriteString("- pack note: legacy pack selected for story pressure; concrete source analysis comes through tools when needed.\n")
	b.WriteString("- source style: infer setting-specific texture from staged lore files; keep scenes concrete, playable, and consequential.\n")
	b.WriteString("- game-loop context: build a compact node graph with playable choices, valid targets, memory intents, and clear terminal handoffs; keep mechanics out of player-facing prose unless the scene makes them visible.\n")
	b.WriteString("- story context: include only the guidance the opening actually needs; let rules or customs emerge from scene pressure when the source supports them.\n")
	b.WriteString("- reader plausibility: strange material needs internal cause, social practice, emotional credibility, material consequence, and follow-through.\n")
	b.WriteString("- material imagination: choose the story's dominant condition from its own premise, such as infrastructure, ecology, transport, weather, resource scarcity, labor, workplace tools, maintenance routines, money pressure, family pressure, or local trade.\n")
	b.WriteString("- creative workshop: use ask_scenarist as the preferred room for story heat, suggestions, critique, story seed risks, character contradictions, scene engines, choice pressure, reversals, and reader criticism.\n")
	b.WriteString("- scenarist sequence: ask one clear advisory question, read the returned file, then ask any dependent follow-up with that path in relevant_paths.\n")
	b.WriteString("- source reference: use ask_lore_expert when a question depends on concrete patterns from the selected lorepack, such as source references, tone, relationship pressure, or contradiction checks.\n")
	b.WriteString("- practical sequence: ask Scenarists first when exploring what this generated story could become; ask LoreExpert when a specific source-pack pattern needs grounding.\n")
	b.WriteString("- concrete source analysis comes through cached ask_lore_expert calls when needed.\n")
	return b.String()
}

func seedWriterRunner(cfg Config, scope agentfs.Scope, _ *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedCreativeRunner(cfg, "SeedBundle", prompt.SeedBundleSchema(), scope, nil, loreExpertTool, logger)
}

func SeedWriterRunner(cfg Config, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedWriterRunner(cfg, scope, scenaristTool, loreExpertTool, logger)
}

func seedVerifierRunner(cfg Config, pack lorepack.Pack, scope agentfs.Scope, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	switch cfg.Backend {
	case "static":
		return nil, nil
	case "claude":
		runner := modelrun.ClaudeRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, runner.SystemPrompt)
		return runner, nil
	default:
		runner := modelrun.SeedVerifierCodexRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, runner.SystemPrompt)
		if strings.TrimSpace(scope.Root) == "" {
			runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPack(pack, runner.SystemPrompt)
		} else {
			if err := scope.Ensure(); err != nil {
				return nil, err
			}
			runner.LocalTools = true
			runner.ToolRoot = scope.Root
			runner.ToolVisiblePaths = append([]string(nil), scope.VisiblePaths...)
			if cfg.ScenaristRounds > 0 {
				runner.MaxToolRounds = cfg.ScenaristRounds
			} else {
				runner.MaxToolRounds = 24
			}
			if loreExpertTool != nil {
				runner.ExtraTools = []codexoauth.LocalTool{loreExpertTool.LocalTool()}
			}
		}
		return modelrun.ConfigureCodexToolLogging(runner, logger, "model", "nerd-verifier", "Nerd Verifier", "", 0, "", "", ""), nil
	}
}

func SeedVerifierRunner(cfg Config, pack lorepack.Pack, scope agentfs.Scope, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedVerifierRunner(cfg, pack, scope, loreExpertTool, logger)
}

func seedWorkshopRunner(cfg Config, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedCreativeRunner(cfg, "", nil, scope, toolRunner, loreExpertTool, logger)
}

func SeedWorkshopRunner(cfg Config, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedWorkshopRunner(cfg, scope, toolRunner, loreExpertTool, logger)
}

func seedCreativeRunner(cfg Config, schemaName string, schema map[string]any, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	if cfg.Backend == "static" {
		return nil, nil
	}
	if (toolRunner != nil || loreExpertTool != nil) && cfg.Backend == "claude" {
		return nil, fmt.Errorf("Writer local tools are only supported for codex backend; use -model-backend codex or disable writer tools")
	}
	switch cfg.Backend {
	case "claude":
		runner := modelrun.ClaudeRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, runner.SystemPrompt)
		return runner, nil
	default:
		if strings.TrimSpace(scope.Root) != "" {
			if err := scope.Ensure(); err != nil {
				return nil, err
			}
		}
		runner := modelrun.CodexRunnerWithMaterialAccess(schemaName, schema, scope, toolRunner, loreExpertTool, nil, cfg.ScenaristRounds)
		return modelrun.ConfigureCodexToolLogging(runner, logger, "writer", "creative-writer", "Creative Writer", "", 0, "", "", ""), nil
	}
}

func SeedCreativeRunner(cfg Config, schemaName string, schema map[string]any, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedCreativeRunner(cfg, schemaName, schema, scope, toolRunner, loreExpertTool, logger)
}

func seedScenaristToolRunner(cfg Config, contextText string, outputDir string, scope agentfs.Scope, logger *gamelog.Writer, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	if cfg.WriterScenaristCalls == 0 || cfg.Backend == "static" {
		return nil, nil
	}
	tool, err := modelrun.ScenaristToolRunner(modelrun.Config{
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

func SeedScenaristToolRunner(cfg Config, contextText string, outputDir string, scope agentfs.Scope, logger *gamelog.Writer, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	return seedScenaristToolRunner(cfg, contextText, outputDir, scope, logger, loreExpertTool)
}

func seedLoreExpertToolRunner(cfg Config, pack lorepack.Pack, collection string, logger *gamelog.Writer) *loreexpert.ToolRunner {
	if cfg.Backend == "static" || cfg.Backend == "claude" {
		return nil
	}
	if strings.TrimSpace(collection) == "" {
		collection = pack.Collection
	}
	tool := modelrun.LoreExpertToolRunner(modelrun.Config{
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

func SeedLoreExpertToolRunner(cfg Config, pack lorepack.Pack, collection string, logger *gamelog.Writer) *loreexpert.ToolRunner {
	return seedLoreExpertToolRunner(cfg, pack, collection, logger)
}

func seedScenaristOutputDir(storyRoot string, outputRoot string) string {
	segment := "seed-" + strings.NewReplacer(".", "", ":", "", "-", "").Replace(time.Now().UTC().Format(time.RFC3339Nano))
	root := strings.TrimSpace(outputRoot)
	if root != "" {
		return runutil.AbsOrClean(filepath.Join(root, "seed-runs", segment, "agent", "scenarists"))
	}
	if strings.TrimSpace(storyRoot) == "" {
		storyRoot = story.DefaultRoot
	}
	return runutil.AbsOrClean(filepath.Join(filepath.Dir(filepath.Clean(storyRoot)), "seed-runs", segment, "agent", "scenarists"))
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

func SeedAgentScopeFromScenaristOutputDir(scenaristOutputDir string) agentfs.Scope {
	return seedAgentScopeFromScenaristOutputDir(scenaristOutputDir)
}

func MaybeSeedExternalStory(ctx context.Context, backend string, bridgeURL string, ws story.Workspace, state story.StoryState, bundle story.SeedBundle, logger *gamelog.Writer) error {
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
		Documents:  runutil.BridgeStoryDocs(docs),
		Replace:    true,
	}
	runutil.LogEvent(logger, gamelog.Event{
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
		runutil.LogEvent(logger, gamelog.Event{
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
	runutil.LogEvent(logger, gamelog.Event{
		Category: "qdrant",
		Event:    "story.seed.index.ingested",
		Message:  "seed story index ingested",
		Fields:   map[string]any{"collection": state.StoryCollection, "document_count": len(docs), "response_count": ingestResp.Count},
		Detail: map[string]any{
			"request":  ingestReq,
			"response": ingestResp,
		},
	})
	for peerID, facts := range SeedPeerCards(bundle.Bible) {
		patch := bridgeclient.PeerCardPatch{Facts: facts, Replace: true}
		runutil.LogEvent(logger, gamelog.Event{
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
			runutil.LogEvent(logger, gamelog.Event{
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
		runutil.LogEvent(logger, gamelog.Event{
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

func SeedPeerCards(bible story.StoryBible) map[string][]string {
	var guidanceFacts []string
	for _, note := range bible.StoryContextGuidance() {
		guidanceFacts = append(guidanceFacts, "CONTEXT: "+note)
	}
	for _, note := range bible.ToneGuidanceLines() {
		guidanceFacts = append(guidanceFacts, "TONE: "+note)
	}
	baseBoundaries := []string{
		"BOUNDARY: Lorepack references support fidelity and background; unused lore is not automatic scene canon.",
	}
	narrator := []string{
		"Name: narrator",
		"ROLE: Player-facing narrator for " + bible.Title + ".",
		"INSTRUCTION: Render the selected node into scene-first prose grounded in current story context.",
		"INSTRUCTION: Keep private actor interiors and Writer planning out of player-facing prose.",
	}
	narrator = append(narrator, guidanceFacts...)
	narrator = append(narrator, baseBoundaries...)

	writer := []string{
		"Name: creative-writer",
		"ROLE: Background planner for bounded StoryPacket revisions in " + bible.Title + ".",
		"INSTRUCTION: Use the Story Bible and lorepack notes as background support for playable scene pressure.",
		"INSTRUCTION: Preserve rolled-out canon boundaries.",
	}
	writer = append(writer, guidanceFacts...)
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
	player = append(player, guidanceFacts...)
	return map[string][]string{
		"narrator":                   TrimCardFacts(narrator, 40),
		"creative-writer":            TrimCardFacts(writer, 40),
		bible.PlayerCharacter.PeerID: TrimCardFacts(player, 40),
	}
}

func TrimCardFacts(facts []string, max int) []string {
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
