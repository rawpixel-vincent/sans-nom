package modelrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/actortool"
	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelcap"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

type Config struct {
	Backend              string
	BridgeURL            string
	CardStore            *contextbundle.LocalCardStore
	Workspace            *story.Workspace
	State                *story.StoryState
	Bible                *story.StoryBible
	GroundLorePackID     string
	GlobalCollection     string
	StoryCollection      string
	WriterScenaristCalls int
	ScenaristProvider    string
	ScenaristModel       string
	ScenaristTools       bool
	ScenaristOutputRoot  string
	ScenaristRounds      int
	Logger               *gamelog.Writer
	WriterPlanner        writer.Planner
}

func RunnerHasLocalTools(runner prompt.Runner) bool {
	codex, ok := runner.(prompt.CodexRunner)
	return ok && codex.LocalTools
}

func seedRunner(backend string, seedBundle bool) prompt.Runner {
	pack, err := lorepack.GetPack(runutil.RepoRoot(), lorepack.DefaultPackID())
	if err != nil {
		pack = lorepack.Pack{}
	}
	return seedRunnerForPack(backend, seedBundle, pack)
}

func SeedRunner(backend string, seedBundle bool) prompt.Runner {
	return seedRunner(backend, seedBundle)
}

func seedRunnerForPack(backend string, seedBundle bool, pack lorepack.Pack) prompt.Runner {
	switch backend {
	case "static":
		return nil
	case "claude":
		runner := ClaudeRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPack(pack, runner.SystemPrompt)
		return runner
	default:
		if seedBundle {
			runner := CodexRunner("SeedBundle", prompt.SeedBundleSchema())
			runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPack(pack, runner.SystemPrompt)
			return runner
		}
		runner := SeedVerifierCodexRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPack(pack, runner.SystemPrompt)
		return runner
	}
}

func SeedRunnerForPack(backend string, seedBundle bool, pack lorepack.Pack) prompt.Runner {
	return seedRunnerForPack(backend, seedBundle, pack)
}

func SeedVerifierCodexRunner() prompt.CodexRunner {
	runner := CodexRunner("SeedReview", prompt.SeedReviewSchema())
	runner.Timeout = VerifierCodexModelTimeout()
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
	serviceTier := os.Getenv("SANS_NOM_VERIFIER_CODEX_OAUTH_SERVICE_TIER")
	if serviceTier == "" {
		serviceTier = "fast"
	}
	runner.ServiceTier = serviceTier
	return runner
}

func LogLoreFidelityContext(logger *gamelog.Writer, role string, runID string, turn int, packetID string, nodeID string, packID string, scenarioContract string) {
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

func LogUnsupportedActorNeeds(logger *gamelog.Writer, event string, message string, turn int, packetID string, nodeID string, removed []actors.UnsupportedActorNeed) {
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

func ContextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore, globalCollection string, storyCollection string, logger *gamelog.Writer) contextbundle.Assembler {
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

func WriterRefreshContextAssembler(bridgeURL string, strict bool, cardStore *contextbundle.LocalCardStore, storyCollection string, logger *gamelog.Writer) contextbundle.Assembler {
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

func NarratorModelMetadata(cfg Config) modelmeta.Metadata {
	switch cfg.Backend {
	case "static":
		return narrator.StaticRenderer{}.ModelMetadata()
	case "claude":
		return ClaudeRunner().ModelMetadata()
	default:
		return NarratorCodexRunner("NarratorDraft", prompt.NarratorDraftSchema()).ModelMetadata()
	}
}

func WriterRefreshModelMetadata(cfg Config) modelmeta.Metadata {
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
		return ClaudeRunner().ModelMetadata()
	default:
		return CodexRunner("StoryPacket", prompt.StoryPacketSchema()).ModelMetadata()
	}
}

func ContextBudgetFor(metadata modelmeta.Metadata, role string, hasTools bool, hasJSONSchema bool, maxToolRounds int) modelcap.Budget {
	return modelcap.BudgetFor(modelcap.Request{
		Metadata:      metadata,
		Role:          role,
		HasTools:      hasTools,
		HasJSONSchema: hasJSONSchema,
		MaxToolRounds: maxToolRounds,
	})
}

func BudgetReportMap(report contextbundle.RenderBudgetReport) map[string]any {
	data, err := json.Marshal(report)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func StageContextBudgetArtifacts(scope agentfs.Scope, report *contextbundle.RenderBudgetReport, inlineContext string) error {
	if report == nil || strings.TrimSpace(scope.Root) == "" {
		return nil
	}
	artifacts := map[string]string{
		"full_context": "story/context.txt",
	}
	inlineRel, err := scope.StageText(filepath.Join("story", "context-inline.txt"), inlineContext)
	if err != nil {
		return err
	}
	artifacts["inline_context"] = inlineRel
	artifacts["budget"] = "story/context-budget.json"
	report.Artifacts = artifacts
	_, err = scope.StageJSON(filepath.Join("story", "context-budget.json"), report)
	return err
}

func WriterToolRoundBudget(rounds int) int {
	if rounds <= 0 {
		return 24
	}
	return rounds
}

func WriterPlannerWithMaterialAccess(backend string, claude prompt.ClaudeRunner, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, actorTool *actortool.ToolRunner, maxToolRounds int, logger *gamelog.Writer, runID string, contextBudget map[string]any) writer.Planner {
	switch backend {
	case "static":
		return writer.StaticPlanner{}
	case "claude":
		claude.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, claude.SystemPrompt)
		if scope.Root != "" {
			claude.PermissionMode = "dontAsk"
			claude.AllowedTools = []string{"Read", "LS", "Grep"}
			claude.WorkDir = scope.Root
			claude.AddDirs = append(claude.AddDirs, scope.Root)
		}
		return writer.ClaudePlanner{Runner: claude}
	default:
		runner := CodexRunnerWithMaterialAccess("StoryPacket", prompt.StoryPacketSchema(), scope, scenaristTool, loreExpertTool, actorTool, maxToolRounds)
		runner = ConfigureCodexToolLogging(runner, logger, "writer", "writer", "Writer", runID, 0, "", "", "")
		runner.ContextBudget = contextBudget
		if runner.LocalTools {
			runner.RequireToolUse = true
			runner.ToolUseReason = "Writer refreshes must perform one narrow grounding read before drafting so the central story planner proves it inspected staged story state."
		}
		return writer.ClaudePlanner{Runner: runner}
	}
}

func CodexRunner(schemaName string, schema map[string]any) prompt.CodexRunner {
	model := os.Getenv("SANS_NOM_CODEX_MODEL")
	if model == "" {
		model = "gpt-5.4"
	}
	effort := os.Getenv("SANS_NOM_CODEX_EFFORT")
	if effort == "" {
		effort = "high"
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
		Timeout:     CodexModelTimeout(),
	}
}

func NarratorCodexRunner(schemaName string, schema map[string]any) prompt.CodexRunner {
	runner := CodexRunner(schemaName, schema)
	if model := strings.TrimSpace(os.Getenv("SANS_NOM_NARRATOR_CODEX_MODEL")); model != "" {
		runner.Model = model
	}
	if effort := runutil.FirstNonEmpty(os.Getenv("SANS_NOM_NARRATOR_CODEX_REASONING"), os.Getenv("SANS_NOM_NARRATOR_CODEX_EFFORT")); strings.TrimSpace(effort) != "" {
		runner.Effort = strings.TrimSpace(effort)
	}
	if tier := runutil.FirstNonEmpty(os.Getenv("SANS_NOM_NARRATOR_CODEX_SERVICE_TIER"), os.Getenv("SANS_NOM_NARRATOR_CODEX_OAUTH_SERVICE_TIER")); strings.TrimSpace(tier) != "" {
		runner.ServiceTier = strings.TrimSpace(tier)
	}
	return runner
}

func DirectorCodexRunner(schemaName string, schema map[string]any, packID string) prompt.CodexRunner {
	runner := CodexRunnerWithPackLoreID(schemaName, schema, packID)
	if model := strings.TrimSpace(os.Getenv("SANS_NOM_DIRECTOR_CODEX_MODEL")); model != "" {
		runner.Model = model
	}
	if effort := runutil.FirstNonEmpty(os.Getenv("SANS_NOM_DIRECTOR_CODEX_REASONING"), os.Getenv("SANS_NOM_DIRECTOR_CODEX_EFFORT")); strings.TrimSpace(effort) != "" {
		runner.Effort = strings.TrimSpace(effort)
	}
	if tier := runutil.FirstNonEmpty(os.Getenv("SANS_NOM_DIRECTOR_CODEX_SERVICE_TIER"), os.Getenv("SANS_NOM_DIRECTOR_CODEX_OAUTH_SERVICE_TIER")); strings.TrimSpace(tier) != "" {
		runner.ServiceTier = strings.TrimSpace(tier)
	}
	return runner
}

func CodexRunnerWithPackLoreID(schemaName string, schema map[string]any, packID string) prompt.CodexRunner {
	runner := CodexRunner(schemaName, schema)
	runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPackID(packID, runner.SystemPrompt)
	return runner
}

func CodexRunnerWithMaterialAccess(schemaName string, schema map[string]any, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, actorTool *actortool.ToolRunner, maxToolRounds int) prompt.CodexRunner {
	runner := CodexRunner(schemaName, schema)
	runner.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, runner.SystemPrompt)
	if strings.TrimSpace(scope.Root) != "" || scenaristTool != nil || loreExpertTool != nil || actorTool != nil {
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
		if actorTool != nil {
			tools = append(tools, actorTool.LocalTool())
		}
		runner.ExtraTools = tools
	}
	return runner
}

func CodexRunnerWithReadTools(schemaName string, schema map[string]any, scope agentfs.Scope, logger *gamelog.Writer, category string, role string, speaker string, turn int, packetID string, nodeID string) prompt.CodexRunner {
	runner := CodexRunner(schemaName, schema)
	if role == "narrator" || schemaName == "NarratorDraft" {
		runner = NarratorCodexRunner(schemaName, schema)
	}
	runner.SystemPrompt = lorestage.MasterLoreSystemPromptFromScope(scope, runner.SystemPrompt)
	if strings.TrimSpace(scope.Root) == "" {
		scope = agentfs.New(filepath.Join(os.TempDir(), "sans-nom-agent", "read-tools"), role).WithVisiblePaths(".")
	}
	_ = scope.Ensure()
	runner.LocalTools = true
	runner.ToolRoot = scope.Root
	runner.ToolVisiblePaths = append([]string(nil), scope.VisiblePaths...)
	runner.MaxToolRounds = 8
	return ConfigureCodexToolLogging(runner, logger, category, role, speaker, "", turn, packetID, nodeID, "")
}

func ConfigureCodexToolLogging(runner prompt.CodexRunner, logger *gamelog.Writer, category string, role string, speaker string, runID string, turn int, packetID string, nodeID string, storyID string) prompt.CodexRunner {
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

func ClaudeRunner() prompt.ClaudeRunner {
	return prompt.ClaudeRunner{
		Model:   "sonnet",
		Effort:  "medium",
		WorkDir: ".",
		Timeout: ClaudeModelTimeout(),
	}
}

func ScenaristToolRunner(cfg Config, contextText string, outputDir string, scope agentfs.Scope, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	if cfg.WriterScenaristCalls == 0 || strings.TrimSpace(outputDir) == "" {
		return nil, nil
	}
	runners := ScenaristProviderRunners(cfg, outputDir, scope.Root)
	if len(runners) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(scope.Root) == "" {
		scope = agentfs.New(filepath.Dir(outputDir), "scenarist").WithVisiblePaths(".")
	}
	return &scenarist.ToolRunner{
		Runner:        &scenarist.ProviderPoolRunner{Runners: runners},
		OutputDir:     outputDir,
		RepoRoot:      runutil.RepoRoot(),
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

func WriterActorToolRunner(_ context.Context, cfg Config, writerContext contextbundle.WriterContext, scope agentfs.Scope, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, runID string) (*actortool.ToolRunner, error) {
	if cfg.Backend != "codex" || cfg.CardStore == nil {
		return nil, nil
	}
	registry, err := runutil.ActorRegistry(cfg.CardStore)
	if err != nil {
		return nil, err
	}
	if len(registry.IDs()) == 0 {
		return nil, nil
	}
	assembler := WriterRefreshContextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.StoryCollection, cfg.Logger)
	assembler.Options.SkipMissingActorCards = true
	scenarioContract := lorestage.ScenarioContractTextForPack(cfg.GroundLorePackID)
	outputDir := filepath.Join(scope.Root, "actors")
	return &actortool.ToolRunner{
		Registry:             registry,
		Invoker:              actors.ClaudeInvoker{Runner: CodexRunnerWithPackLoreID("ActorOutput", prompt.ActorOutputSchema(), cfg.GroundLorePackID)},
		Assembler:            assembler,
		Packet:               packet,
		NodeID:               nodeID,
		Decision:             decision,
		Bible:                cfg.Bible,
		ScenarioContractText: runutil.FirstNonEmpty(scenarioContract, writerContext.ScenarioContract),
		OutputDir:            outputDir,
		WorkspaceRoot:        scope.Root,
		VisiblePaths:         append([]string(nil), scope.VisiblePaths...),
		Logger:               cfg.Logger,
		CallerRole:           "writer",
		CallerName:           "Writer",
		RunID:                runID,
		MaxCalls:             actors.Phase0ActorCap,
	}, nil
}

func LoreExpertToolRunner(cfg Config) *loreexpert.ToolRunner {
	if cfg.Backend == "static" || cfg.Backend == "claude" {
		return nil
	}
	packID := runutil.FirstNonEmpty(cfg.GroundLorePackID, story.DefaultGroundLorePackID)
	pack, err := lorepack.GetPack(runutil.RepoRoot(), packID)
	if err != nil {
		runutil.LogEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "lore",
			Event:    "lore.expert.pack_unavailable",
			Message:  err.Error(),
			Fields:   map[string]any{"pack_id": packID},
		})
		return nil
	}
	collection := runutil.FirstNonEmpty(cfg.GlobalCollection, pack.Collection, story.DefaultGroundLoreCollection)
	packDir := filepath.Dir(pack.Path)
	runner := LoreExpertModelRunner()
	runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPack(pack, runner.SystemPrompt)
	return &loreexpert.ToolRunner{
		Runner:            runner,
		Retriever:         lore.BridgeRetriever{Client: bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN")), Collection: collection},
		CacheRoot:         filepath.Join(runutil.RepoRoot(), ".cache", "sans-nom", "lore-expert"),
		RepoRoot:          runutil.RepoRoot(),
		PackID:            pack.ID,
		PackLabel:         pack.Label,
		PackDescription:   pack.Description,
		Collection:        collection,
		PackManifestPath:  pack.Path,
		IndexManifestPath: filepath.Join(packDir, "index", "manifest.json"),
		MasterLorePath:    pack.MasterLorePath(),
		MaxSources:        6,
		Logger:            cfg.Logger,
		CallerRole:        "writer",
		CallerSpeaker:     "Writer",
	}
}

func LoreExpertModelRunner() prompt.CodexRunner {
	runner := CodexRunner("", nil)
	if model := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_MODEL")); model != "" {
		runner.Model = model
	} else {
		runner.Model = "gpt-5.3-codex-spark"
	}
	if effort := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_EFFORT")); effort != "" {
		runner.Effort = CodexLoreExpertEffort(effort)
	} else {
		runner.Effort = "medium"
	}
	if tier := strings.TrimSpace(os.Getenv("SANS_NOM_LORE_EXPERT_SERVICE_TIER")); tier != "" {
		runner.ServiceTier = tier
	}
	return runner
}

func CodexLoreExpertEffort(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "minimal" {
		return "low"
	}
	return value
}

func ScenaristProviderRunners(cfg Config, outputDir string, workspaceRoot string) []scenarist.ProviderRunner {
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
			Timeout:  ScenaristModelTimeout(),
			Logger:   cfg.Logger,
		})
	}
	return runners
}

func scenaristWorkDir(outputDir string, workspaceRoot string) string {
	if strings.TrimSpace(workspaceRoot) != "" {
		return runutil.AbsOrClean(workspaceRoot)
	}
	if strings.TrimSpace(outputDir) == "" {
		return "."
	}
	return runutil.AbsOrClean(filepath.Dir(outputDir))
}

func scenaristAddDirs(workDir string) []string {
	dirs := []string{}
	if strings.TrimSpace(workDir) != "" {
		dirs = append(dirs, runutil.AbsOrClean(workDir))
	}
	return dirs
}

func WriterRunInfo(cfg Config) (string, string) {
	segment := ScenaristTurnSegment(cfg)
	if cfg.Workspace == nil {
		return segment, ""
	}
	return segment, filepath.Join(cfg.Workspace.ModelRunsDir(), "writer", segment)
}

func DirectorRunDir(cfg Config, turn int) string {
	if cfg.Workspace == nil {
		return ""
	}
	return filepath.Join(cfg.Workspace.ModelRunsDir(), "director", fmt.Sprintf("turn-%06d", turn))
}

func WriterActorIDs(manifest []contextbundle.ActorManifestItem) []string {
	ids := make([]string, 0, len(manifest))
	for _, actor := range manifest {
		if strings.TrimSpace(actor.PeerID) != "" {
			ids = append(ids, strings.TrimSpace(actor.PeerID))
		}
	}
	return uniqueWriterActorIDs(ids)
}

func WriterActorManifestSummary(manifest []contextbundle.ActorManifestItem) string {
	if len(manifest) == 0 {
		return ""
	}
	var parts []string
	for _, actor := range manifest {
		part := strings.TrimSpace(actor.PeerID)
		if actor.DisplayName != "" {
			part += " (" + actor.DisplayName + ")"
		}
		if actor.Reason != "" {
			part += ": " + actor.Reason
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

func uniqueWriterActorIDs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func PrepareStoryAgentScope(cfg Config, role string, runID string, packet scene.StoryPacket, nodeID string, contextText string) (agentfs.Scope, error) {
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
	packID := runutil.FirstNonEmpty(cfg.GroundLorePackID, lorepack.DefaultPackID())
	if cfg.State != nil {
		packID = runutil.FirstNonEmpty(cfg.State.SourceLorePackID(), packID)
	}
	if stagedScope, err := lorestage.StageLorePackForRoleByID(scope, role, packID); err == nil {
		scope = stagedScope
	} else {
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
		if err := StageNarrationFiles(scope, cfg.Workspace.NarrationDir()); err != nil {
			return scope, err
		}
	}
	return scope, nil
}

func StageNarrationFiles(scope agentfs.Scope, narrationDir string) error {
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

func ScenaristTurnSegment(cfg Config) string {
	turn := 1
	if cfg.State != nil && cfg.State.LatestTurn >= 0 {
		turn = cfg.State.LatestTurn + 1
	}
	return fmt.Sprintf("turn-%06d", turn)
}

func ResolveWriterScenaristCallCount(value int, backend string, writerRefreshEnabled bool) int {
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

const (
	defaultAgentModelTimeout    = 45 * time.Minute
	defaultVerifierCodexTimeout = 5 * time.Minute
)

func SharedModelTimeout() time.Duration {
	return runutil.EnvDurationMS("SANS_NOM_MODEL_TIMEOUT_MS", defaultAgentModelTimeout)
}

func CodexModelTimeout() time.Duration {
	return runutil.EnvDurationMS("SANS_NOM_CODEX_TIMEOUT_MS", SharedModelTimeout())
}

func VerifierCodexModelTimeout() time.Duration {
	if strings.TrimSpace(os.Getenv("SANS_NOM_VERIFIER_CODEX_TIMEOUT_MS")) != "" {
		return runutil.EnvDurationMS("SANS_NOM_VERIFIER_CODEX_TIMEOUT_MS", CodexModelTimeout())
	}
	if strings.TrimSpace(os.Getenv("SANS_NOM_CODEX_TIMEOUT_MS")) != "" {
		return CodexModelTimeout()
	}
	if strings.TrimSpace(os.Getenv("SANS_NOM_MODEL_TIMEOUT_MS")) != "" {
		return SharedModelTimeout()
	}
	return defaultVerifierCodexTimeout
}

func ClaudeModelTimeout() time.Duration {
	return runutil.EnvDurationMS("SANS_NOM_CLAUDE_TIMEOUT_MS", SharedModelTimeout())
}

func ScenaristModelTimeout() time.Duration {
	return runutil.EnvDurationMS("SANS_NOM_SCENARIST_TIMEOUT_MS", SharedModelTimeout())
}

func WriterAwaitTimeout() time.Duration {
	return runutil.EnvDurationMS("SANS_NOM_WRITER_AWAIT_TIMEOUT_MS", SharedModelTimeout())
}

func DefaultScenaristProviderPool() string {
	if value := strings.TrimSpace(os.Getenv("SANS_NOM_SCENARIST_PROVIDERS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("SANS_NOM_SCENARIST_PROVIDER")); value != "" {
		return value
	}
	return strings.Join(scenarist.DefaultProviderNames(), ",")
}
