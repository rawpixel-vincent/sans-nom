package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/adminapp"
	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/bridgeproc"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/director"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lore"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelrun"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/playrun"
	"github.com/rawpixel-vincent/sans-nom/internal/playui"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seedrun"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
	"github.com/rawpixel-vincent/sans-nom/internal/ttsrun"
)

type AdminConfig = adminapp.Config
type SeedRuntimeConfig = seedrun.Config
type PlayConfig = playrun.Config
type PlayTTSConfig = ttsrun.Config
type FlagBridgeConfig = bridgeproc.Config

type OneShotRolloutConfig struct {
	Packet           scene.StoryPacket
	NodeID           string
	Decision         scene.PlayerDecision
	Backend          string
	BridgeURL        string
	PacketPath       string
	CardStore        *contextbundle.LocalCardStore
	State            *story.StoryState
	Bible            *story.StoryBible
	GlobalCollection string
	StoryCollection  string
}

func LoadDotEnv(path string) {
	runutil.LoadDotEnv(path)
}

func RunLoreSearch(ctx context.Context, bridgeURL string, collection string, query string, k int) error {
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
		if text := runutil.Preview(result.Text, 240); text != "" {
			fmt.Printf("  %s\n", text)
		}
	}
	return nil
}

func PrintStoryNarration(workspace story.Workspace) error {
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

func PrintSeedBundle(bundle story.SeedBundle, outputPath string) error {
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

func NewStorySession(ctx context.Context, opts storyapp.StoryActionOptions, cfg SeedRuntimeConfig) (storyapp.StorySession, error) {
	return seedrun.NewStorySession(ctx, opts, cfg)
}

func NewStorySessionWithDisconnectRetry(ctx context.Context, opts storyapp.StoryActionOptions, cfg SeedRuntimeConfig, interactive bool) (storyapp.StorySession, error) {
	return seedrun.NewStorySessionWithDisconnectRetry(ctx, opts, cfg, interactive)
}

func RunAdmin(ctx context.Context, cfg AdminConfig) error {
	return adminapp.Run(ctx, cfg)
}

func EnsureFlagBridge(ctx context.Context, cfg FlagBridgeConfig) (func(), error) {
	return bridgeproc.EnsureFlagBridge(ctx, cfg)
}

func EnsureBridge(ctx context.Context, backend string, bridgeURL string) (func(), error) {
	return bridgeproc.Ensure(ctx, backend, bridgeURL)
}

func RunContextPreview(ctx context.Context, role string, asText bool, bridgeURL string, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, bible *story.StoryBible, globalCollection string, storyCollection string, cardStore *contextbundle.LocalCardStore) error {
	req := contextbundle.BuildRequest{Packet: packet, NodeID: nodeID, Decision: decision, Bible: bible}
	switch role {
	case "narrator":
		assembler := modelrun.ContextAssembler(bridgeURL, bridgeURL != "", cardStore, globalCollection, storyCollection, nil)
		context, err := assembler.AssembleNarrator(ctx, req)
		if err != nil {
			return err
		}
		if asText {
			fmt.Println(context.RenderText())
			return nil
		}
		return runutil.PrintPrettyJSON(context)
	case "writer":
		assembler := modelrun.WriterRefreshContextAssembler(bridgeURL, bridgeURL != "", cardStore, storyCollection, nil)
		context, err := assembler.AssembleWriter(ctx, req)
		if err != nil {
			return err
		}
		if asText {
			fmt.Println(context.RenderText())
			return nil
		}
		return runutil.PrintPrettyJSON(context)
	default:
		return fmt.Errorf("unknown context preview role %q", role)
	}
}

func RunPlay(ctx context.Context, cfg PlayConfig) error {
	return playui.Run(ctx, cfg)
}

func RunOneShotRollout(ctx context.Context, cfg OneShotRolloutConfig) error {
	packID := playrun.SourceLorePackID(cfg.State, lorepack.DefaultPackID())
	runner := modelrun.ClaudeRunner()
	runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPackID(packID, runner.SystemPrompt)
	registry, err := runutil.ActorRegistry(cfg.CardStore)
	if err != nil {
		return err
	}
	assembler := modelrun.ContextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection, nil)
	narratorContext, err := assembler.AssembleNarrator(ctx, contextbundle.BuildRequest{
		Packet:   cfg.Packet,
		NodeID:   cfg.NodeID,
		Decision: cfg.Decision,
		Bible:    cfg.Bible,
	})
	if err != nil {
		return err
	}
	var actorInvoker actors.Invoker = actors.StaticInvoker{}
	var renderer narrator.Renderer = narrator.StaticRenderer{}
	switch cfg.Backend {
	case "claude":
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	case "codex":
		actorInvoker = actors.ClaudeInvoker{Runner: modelrun.DirectorCodexRunner("ActorOutput", prompt.ActorOutputSchema(), packID)}
		scope := agentfs.New(filepath.Dir(runutil.AbsOrClean(cfg.PacketPath)), "narrator").WithVisiblePaths(".")
		renderer = narrator.ClaudeRenderer{Runner: modelrun.CodexRunnerWithReadTools("NarratorDraft", prompt.NarratorDraftSchema(), scope, nil, "narrator", "narrator", "Narrator", 0, cfg.Packet.ID, cfg.NodeID)}
	}
	selected, err := cfg.Packet.EntryNode()
	if strings.TrimSpace(cfg.NodeID) != "" {
		selected, err = cfg.Packet.Node(cfg.NodeID)
	}
	if err != nil {
		return err
	}
	rehearsal, err := director.Service{
		Actors:           &actors.Service{Registry: registry, Invoker: actorInvoker},
		ActorContextText: narratorContext.RenderActorText(),
	}.Rehearse(ctx, cfg.Packet, selected, narratorContext.RenderText())
	if err != nil {
		return err
	}
	rollout, err := narrator.Service{Renderer: renderer}.Rollout(ctx, cfg.Packet, cfg.NodeID, cfg.Decision, narratorContext.RenderText(), rehearsal)
	if err != nil {
		return err
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
	return nil
}

func SourceLorePackID(state *story.StoryState, fallback string) string {
	return playrun.SourceLorePackID(state, fallback)
}

func SourceLoreCollection(state *story.StoryState, fallback string) string {
	return playrun.SourceLoreCollection(state, fallback)
}

func ResolvedStoryCollection(state *story.StoryState, fallback string) string {
	return playrun.ResolvedStoryCollection(state, fallback)
}

func StoryLogger(workspace *story.Workspace, state *story.StoryState) *gamelog.Writer {
	return runutil.StoryLogger(workspace, state)
}

func FirstNonEmpty(values ...string) string {
	return runutil.FirstNonEmpty(values...)
}

func ValidatePacketSourceForBackend(backend string, packetExplicit bool, packetPath string) error {
	if packetPath == "" {
		return errors.New("packet path is required")
	}
	if backend == "static" || packetExplicit {
		return nil
	}
	return errors.New("live packet run requires an explicit -packet; testdata is never selected by default")
}

func FlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func PassedFlagNames() map[string]struct{} {
	names := map[string]struct{}{}
	flag.Visit(func(f *flag.Flag) {
		names[f.Name] = struct{}{}
	})
	return names
}

func ShouldRunLauncher(noArgs bool, passed map[string]struct{}) bool {
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

func ShouldUsePlayTUI(turns int, choiceID string, freeText string, traceJSONL bool) bool {
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

func ShouldCreateStory(storyID string, packetPath string, storyRoot string, replaceStory bool) bool {
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

func ResolveBackend(value string, useClaude bool) string {
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

func RuntimeBridgeURL(backend string, bridgeURL string) string {
	if strings.TrimSpace(bridgeURL) != "" {
		return bridgeURL
	}
	if backend == "static" {
		return ""
	}
	return bridgeclient.NewClient("", "").BaseURL
}

func ResolveWriterScenaristCallCount(value int, backend string, writerRefreshEnabled bool) int {
	return modelrun.ResolveWriterScenaristCallCount(value, backend, writerRefreshEnabled)
}

func PlayTTSConfigFromFlags(enabled bool, baseURL string, model string, voice string, format string, instruction string, outputDir string) PlayTTSConfig {
	return ttsrun.FromFlags(enabled, baseURL, model, voice, format, instruction, outputDir)
}

func EnvBool(name string) bool {
	return runutil.EnvBool(name)
}

func EnvBoolDefault(name string, fallback bool) bool {
	return runutil.EnvBoolDefault(name, fallback)
}

func EnvInt(name string, fallback int) int {
	return runutil.EnvInt(name, fallback)
}

func DefaultScenaristProviderPool() string {
	return modelrun.DefaultScenaristProviderPool()
}

func RepoRoot() string {
	return runutil.RepoRoot()
}

func OpenPacket(path string) (scene.StoryPacket, error) {
	return runutil.OpenPacket(path)
}

func LoadCardStore() (*contextbundle.LocalCardStore, error) {
	return runutil.LoadCardStore()
}
