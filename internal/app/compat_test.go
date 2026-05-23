package app

import (
	"context"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/actortool"
	"github.com/rawpixel-vincent/sans-nom/internal/adminapp"
	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/bridgeproc"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/loreexpert"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/modelrun"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/playrun"
	"github.com/rawpixel-vincent/sans-nom/internal/playui"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scenarist"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seedrun"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
	"github.com/rawpixel-vincent/sans-nom/internal/ttsrun"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

type playConfig = playrun.Config
type seedRuntimeConfig = seedrun.Config
type playTTSConfig = ttsrun.Config

func parseDecision(line string, node scene.SceneNode) scene.PlayerDecision {
	return playrun.ParseDecision(line, node)
}

func resolveWriterScenaristCallCount(value int, backend string, writerRefreshEnabled bool) int {
	return modelrun.ResolveWriterScenaristCallCount(value, backend, writerRefreshEnabled)
}

func envBoolDefault(name string, fallback bool) bool {
	return runutil.EnvBoolDefault(name, fallback)
}

func sharedModelTimeout() time.Duration {
	return modelrun.SharedModelTimeout()
}

func verifierCodexModelTimeout() time.Duration {
	return modelrun.VerifierCodexModelTimeout()
}

func codexRunner(schemaName string, schema map[string]any) prompt.CodexRunner {
	return modelrun.CodexRunner(schemaName, schema)
}

func claudeRunner() prompt.ClaudeRunner {
	return modelrun.ClaudeRunner()
}

func scenaristModelTimeout() time.Duration {
	return modelrun.ScenaristModelTimeout()
}

func writerAwaitTimeout() time.Duration {
	return modelrun.WriterAwaitTimeout()
}

func shouldUsePlayTUI(turns int, choiceID string, freeText string, traceJSONL bool) bool {
	return ShouldUsePlayTUI(turns, choiceID, freeText, traceJSONL)
}

func seedWriterRunner(cfg seedRuntimeConfig, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedrun.SeedWriterRunner(cfg, scope, scenaristTool, loreExpertTool, logger)
}

func codexRunnerWithMaterialAccess(schemaName string, schema map[string]any, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, actorTool *actortool.ToolRunner, maxToolRounds int) prompt.CodexRunner {
	return modelrun.CodexRunnerWithMaterialAccess(schemaName, schema, scope, scenaristTool, loreExpertTool, actorTool, maxToolRounds)
}

func seedVerifierRunner(cfg seedRuntimeConfig, pack lorepack.Pack, scope agentfs.Scope, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedrun.SeedVerifierRunner(cfg, pack, scope, loreExpertTool, logger)
}

func seedVerifierCodexRunner() prompt.CodexRunner {
	return modelrun.SeedVerifierCodexRunner()
}

func seedWorkshopRunner(cfg seedRuntimeConfig, scope agentfs.Scope, toolRunner *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, logger *gamelog.Writer) (prompt.Runner, error) {
	return seedrun.SeedWorkshopRunner(cfg, scope, toolRunner, loreExpertTool, logger)
}

func narratorCodexRunner(schemaName string, schema map[string]any) prompt.CodexRunner {
	return modelrun.NarratorCodexRunner(schemaName, schema)
}

func directorCodexRunner(schemaName string, schema map[string]any, packID string) prompt.CodexRunner {
	return modelrun.DirectorCodexRunner(schemaName, schema, packID)
}

func codexRunnerWithPackLoreID(schemaName string, schema map[string]any, packID string) prompt.CodexRunner {
	return modelrun.CodexRunnerWithPackLoreID(schemaName, schema, packID)
}

func codexRunnerWithReadTools(schemaName string, schema map[string]any, scope agentfs.Scope, logger *gamelog.Writer, category string, role string, speaker string, turn int, packetID string, nodeID string) prompt.CodexRunner {
	return modelrun.CodexRunnerWithReadTools(schemaName, schema, scope, logger, category, role, speaker, turn, packetID, nodeID)
}

func adminActorInvoker(backend string, packID string) actors.Invoker {
	return adminapp.ActorInvoker(backend, packID)
}

func scenaristProviderRunners(cfg playConfig, outputDir string, workspaceRoot string) []scenarist.ProviderRunner {
	return modelrun.ScenaristProviderRunners(modelConfig(cfg), outputDir, workspaceRoot)
}

func stageLorePackForAgent(scope agentfs.Scope, pack lorepack.Pack) (agentfs.Scope, error) {
	return lorestage.StageLorePackForAgent(scope, pack)
}

func prepareStoryAgentScope(cfg playConfig, role string, runID string, packet scene.StoryPacket, nodeID string, contextText string) (agentfs.Scope, error) {
	return modelrun.PrepareStoryAgentScope(modelConfig(cfg), role, runID, packet, nodeID, contextText)
}

func loreExpertModelRunner() prompt.CodexRunner {
	return modelrun.LoreExpertModelRunner()
}

func codexLoreExpertEffort(value string) string {
	return modelrun.CodexLoreExpertEffort(value)
}

func loreExpertToolRunner(cfg playConfig) *loreexpert.ToolRunner {
	return modelrun.LoreExpertToolRunner(modelConfig(cfg))
}

func bridgeDialFailure(err error) bool {
	return bridgeproc.DialFailure(err)
}

func bridgeListenAddress(rawURL string) (string, string, error) {
	return bridgeproc.ListenAddress(rawURL)
}

func synthesizeTurnAudio(ctx context.Context, cfg playConfig, state *story.StoryState, loopTurn int, rollout narrator.Rollout) (string, error) {
	return ttsrun.SynthesizeTurnAudio(ctx, cfg.TTS, cfg.Workspace, state, cfg.Logger, loopTurn, rollout)
}

func ttsOutputPath(cfg playConfig, state *story.StoryState, loopTurn int) string {
	return ttsrun.OutputPath(cfg.TTS, cfg.Workspace, state, loopTurn)
}

func buildSeedContext(ctx context.Context, retriever lore.Retriever) string {
	return seedrun.BuildContext(ctx, retriever)
}

func validatePacketSourceForBackend(backend string, packetExplicit bool, packetPath string) error {
	return ValidatePacketSourceForBackend(backend, packetExplicit, packetPath)
}

func shouldCreateStory(storyID string, packetPath string, storyRoot string, replaceStory bool) bool {
	return ShouldCreateStory(storyID, packetPath, storyRoot, replaceStory)
}

func shouldRunLauncher(noArgs bool, passed map[string]struct{}) bool {
	return ShouldRunLauncher(noArgs, passed)
}

func newStorySession(ctx context.Context, opts storyapp.StoryActionOptions, cfg seedRuntimeConfig) (storyapp.StorySession, error) {
	return seedrun.NewStorySession(ctx, opts, cfg)
}

func runPlay(ctx context.Context, cfg playConfig) error {
	return playui.Run(ctx, cfg)
}

func decisionForWriterRefreshAfterSubmit(submitted scene.PlayerDecision, displayed scene.PlayerDecision, state *story.StoryState) scene.PlayerDecision {
	return playrun.DecisionForWriterRefreshAfterSubmit(submitted, displayed, state)
}

func applyCardUpdatesAfterCommit(ctx context.Context, cfg playConfig, state *story.StoryState, client bridgeclient.Client, updates []scene.CardUpdate, displayTurn int, packetID string, nodeID string) error {
	return playrun.ApplyCardUpdatesAfterCommit(ctx, cfg, state, client, updates, displayTurn, packetID, nodeID)
}

func refreshStoryIndex(ctx context.Context, cfg playConfig, state *story.StoryState, packet scene.StoryPacket) error {
	return playrun.RefreshStoryIndex(ctx, cfg, state, packet)
}

func retryBlockedWriterRefreshIfNeeded(ctx context.Context, cfg playConfig) (playConfig, error) {
	return playrun.RetryBlockedWriterRefreshIfNeeded(ctx, cfg)
}

func savePendingDisplayedTurn(cfg playConfig, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, committed bool, audioPath string, loopTurn int) error {
	return playrun.SavePendingDisplayedTurn(cfg, state, packet, rollout, committed, audioPath, loopTurn)
}

func terminalHandoffBridge(node scene.SceneNode) string {
	return playrun.TerminalHandoffBridge(node)
}

func seedRunner(backend string, seedBundle bool) prompt.Runner {
	return modelrun.SeedRunner(backend, seedBundle)
}

func seedScenaristToolRunner(cfg seedRuntimeConfig, contextText string, outputDir string, scope agentfs.Scope, logger *gamelog.Writer, loreExpertTool *loreexpert.ToolRunner) (*scenarist.ToolRunner, error) {
	return seedrun.SeedScenaristToolRunner(cfg, contextText, outputDir, scope, logger, loreExpertTool)
}

func seedAgentScopeFromScenaristOutputDir(scenaristOutputDir string) agentfs.Scope {
	return seedrun.SeedAgentScopeFromScenaristOutputDir(scenaristOutputDir)
}

func writerPlannerWithMaterialAccess(backend string, claude prompt.ClaudeRunner, scope agentfs.Scope, scenaristTool *scenarist.ToolRunner, loreExpertTool *loreexpert.ToolRunner, actorTool *actortool.ToolRunner, maxToolRounds int, logger *gamelog.Writer, runID string, contextBudget map[string]any) writer.Planner {
	return modelrun.WriterPlannerWithMaterialAccess(backend, claude, scope, scenaristTool, loreExpertTool, actorTool, maxToolRounds, logger, runID, contextBudget)
}

func narratorModelMetadata(cfg playConfig) modelmeta.Metadata {
	return modelrun.NarratorModelMetadata(modelConfig(cfg))
}

func modelConfig(cfg playConfig) modelrun.Config {
	return modelrun.Config{
		Backend:              cfg.Backend,
		BridgeURL:            cfg.BridgeURL,
		CardStore:            cfg.CardStore,
		Workspace:            cfg.Workspace,
		State:                cfg.State,
		Bible:                cfg.Bible,
		GroundLorePackID:     cfg.GroundLorePackID,
		GlobalCollection:     cfg.GlobalCollection,
		StoryCollection:      cfg.StoryCollection,
		WriterScenaristCalls: cfg.WriterScenaristCalls,
		ScenaristProvider:    cfg.ScenaristProvider,
		ScenaristModel:       cfg.ScenaristModel,
		ScenaristTools:       cfg.ScenaristTools,
		ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
		ScenaristRounds:      cfg.ScenaristRounds,
		Logger:               cfg.Logger,
		WriterPlanner:        cfg.WriterPlanner,
	}
}
