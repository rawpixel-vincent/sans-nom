package adminapp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/admin"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/bridgeproc"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/lorepack"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/modelrun"
	"github.com/rawpixel-vincent/sans-nom/internal/playrun"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/seedrun"
	"github.com/rawpixel-vincent/sans-nom/internal/storyapp"
)

type Config struct {
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

func Run(ctx context.Context, cfg Config) error {
	stopBridge, err := bridgeproc.Ensure(ctx, cfg.Backend, cfg.BridgeURL)
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
			refreshCfg := playrun.Config{
				Packet:               session.Packet,
				NodeID:               session.NodeID,
				Backend:              cfg.Backend,
				BridgeURL:            cfg.BridgeURL,
				CardStore:            cfg.CardStore,
				Workspace:            &workspace,
				State:                &state,
				Bible:                &bible,
				GroundLorePackID:     state.SourceLorePackID(),
				GlobalCollection:     playrun.SourceLoreCollection(&state, cfg.GlobalCollection),
				StoryCollection:      playrun.ResolvedStoryCollection(&state, cfg.StoryCollection),
				InitialDecision:      playrun.DecisionForWriterRefresh(&state),
				RefreshWrite:         true,
				WriterScenaristCalls: cfg.WriterScenaristCalls,
				ScenaristProvider:    cfg.ScenaristProvider,
				ScenaristModel:       cfg.ScenaristModel,
				ScenaristTools:       cfg.ScenaristTools,
				ScenaristOutputRoot:  cfg.ScenaristOutputRoot,
				ScenaristRounds:      cfg.ScenaristRounds,
				Logger:               runutil.StoryLogger(&workspace, &state),
			}
			next, err := playrun.RefreshPacket(ctx, session.Packet, session.NodeID, playrun.DecisionForWriterRefresh(&state), refreshCfg)
			if err != nil {
				if failure, ok := playrun.AsWriterFailure(err); ok {
					_ = playrun.MarkStoryBlocked(refreshCfg, &state, failure)
				}
				return err
			}
			return playrun.SaveCurrentStoryPacket(ctx, refreshCfg, &state, next, playrun.EntryNodeID(next))
		},
		ListActors: func(ctx context.Context, storyID string) ([]admin.ActorSummary, error) {
			return StoryActors(ctx, cfg, storyID)
		},
		ActorChat: func(ctx context.Context, storyID string, req admin.ActorChatRequest) (admin.ActorChatResponse, error) {
			return ActorChat(ctx, cfg, storyID, req)
		},
		NewStory: func(ctx context.Context, packID string) (storyapp.StorySession, error) {
			pack, err := lorepack.GetPack(runutil.RepoRoot(), runutil.FirstNonEmpty(packID, cfg.GroundLorePackID, lorepack.DefaultPackID()))
			if err != nil {
				return storyapp.StorySession{}, err
			}
			return seedrun.NewStorySession(ctx, storyapp.StoryActionOptions{
				StoryRoot:            cfg.StoryRoot,
				GroundLorePackID:     pack.ID,
				GroundLoreCollection: runutil.FirstNonEmpty(pack.Collection, cfg.GlobalCollection),
				StoryCollection:      cfg.StoryCollection,
			}, seedrun.Config{
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

func StoryActors(ctx context.Context, cfg Config, storyID string) ([]admin.ActorSummary, error) {
	session, cardStore, err := StorySessionCards(ctx, cfg, storyID)
	if err != nil {
		return nil, err
	}
	_ = session
	ids := cardStore.ActorIDs()
	out := make([]admin.ActorSummary, 0, len(ids))
	for _, id := range ids {
		card, ok := cardStore.LocalCard(id)
		if !ok {
			continue
		}
		out = append(out, admin.ActorSummary{
			ActorID:     id,
			DisplayName: runutil.FirstNonEmpty(card.DisplayName, id),
			CardSource:  filepath.ToSlash(card.Source),
			CardPreview: cardPreview(card.Text, 360),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].ActorID < out[j].ActorID
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out, nil
}

func ActorChat(ctx context.Context, cfg Config, storyID string, req admin.ActorChatRequest) (admin.ActorChatResponse, error) {
	session, cardStore, err := StorySessionCards(ctx, cfg, storyID)
	if err != nil {
		return admin.ActorChatResponse{}, err
	}
	registry, err := runutil.ActorRegistry(cardStore)
	if err != nil {
		return admin.ActorChatResponse{}, err
	}
	localActor, ok := registry.Actor(req.ActorID)
	if !ok {
		return admin.ActorChatResponse{}, fmt.Errorf("actor %q is not available for story %q", req.ActorID, storyID)
	}
	node, err := session.Packet.Node(session.NodeID)
	if err != nil {
		node, err = session.Packet.EntryNode()
	}
	if err != nil {
		return admin.ActorChatResponse{}, err
	}
	node.ActorNeeds = nil
	node.MemoryIntents = nil
	node.Beats = append(node.Beats, scene.Beat{
		ID:   "admin_actor_chat",
		Text: ActorChatPromptText(req),
	})
	need := scene.ActorNeed{
		ActorID: req.ActorID,
		Mode:    scene.ActorModeContext,
		Reason:  "Admin transient actor chat: " + strings.TrimSpace(req.Message),
	}
	assembler := modelrun.ContextAssembler(
		cfg.BridgeURL,
		false,
		cardStore,
		playrun.SourceLoreCollection(&session.State, cfg.GlobalCollection),
		playrun.ResolvedStoryCollection(&session.State, cfg.StoryCollection),
		nil,
	)
	assembler.Options.DisableCardSeeding = true
	actorContext, contextText, warnings, err := assembler.AssembleActorCall(ctx, contextbundle.ActorCallBuildRequest{
		Packet:               session.Packet,
		Node:                 node,
		Need:                 need,
		Decision:             playrun.DecisionForWriterRefresh(&session.State),
		Bible:                &session.Bible,
		ScenarioContractText: lorestage.ScenarioContractTextForPack(session.State.SourceLorePackID()),
	})
	if err != nil {
		return admin.ActorChatResponse{}, err
	}
	invoker := actorInvoker(cfg.Backend, session.State.SourceLorePackID())
	actor := localActor
	actor.Card = actorContext.Card
	output, err := invoker.Invoke(ctx, actor, node, need, contextText)
	if err != nil {
		return admin.ActorChatResponse{}, err
	}
	return admin.ActorChatResponse{
		ActorID:  actor.ID,
		Interior: output.Interior,
		Dialogue: output.Dialogue,
		Action:   output.Action,
		Warnings: warnings,
		Model:    actorInvokerMetadata(invoker),
	}, nil
}

func StorySessionCards(ctx context.Context, cfg Config, storyID string) (storyapp.StorySession, *contextbundle.LocalCardStore, error) {
	session, err := storyapp.ResumeStory(ctx, storyID, storyapp.StoryActionOptions{StoryRoot: cfg.StoryRoot})
	if err != nil {
		return storyapp.StorySession{}, nil, err
	}
	store, err := contextbundle.LoadStoryCardStore(cfg.CardStore, session.Workspace.Dir)
	if err != nil {
		return storyapp.StorySession{}, nil, err
	}
	return session, store, nil
}

func actorInvoker(backend string, packID string) actors.Invoker {
	switch backend {
	case "claude":
		runner := modelrun.ClaudeRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPackID(packID, runner.SystemPrompt)
		return actors.ClaudeInvoker{Runner: runner}
	case "codex":
		return actors.ClaudeInvoker{Runner: modelrun.CodexRunnerWithPackLoreID("ActorOutput", prompt.ActorOutputSchema(), packID)}
	default:
		return actors.StaticInvoker{}
	}
}

func ActorInvoker(backend string, packID string) actors.Invoker {
	return actorInvoker(backend, packID)
}

func actorInvokerMetadata(invoker actors.Invoker) modelmeta.Metadata {
	if provider, ok := invoker.(interface {
		ModelMetadata() modelmeta.Metadata
	}); ok {
		return provider.ModelMetadata()
	}
	return modelmeta.Metadata{}
}

func ActorChatPromptText(req admin.ActorChatRequest) string {
	var lines []string
	history := req.History
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	for _, item := range history {
		role := strings.TrimSpace(item.Role)
		text := strings.TrimSpace(item.Text)
		if role == "" || text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, runutil.TruncateRunes(text, 600)))
	}
	lines = append(lines, "user: "+strings.TrimSpace(req.Message))
	return "Admin transient actor chat. This exchange is not canon and must not be committed to memory.\n" + strings.Join(lines, "\n")
}

func cardPreview(text string, max int) string {
	var parts []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return runutil.TruncateRunes(strings.Join(parts, " "), max)
}
