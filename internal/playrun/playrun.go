package playrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	bridgeclient "github.com/rawpixel-vincent/sans-nom/internal/bridge"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/director"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/lorestage"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/modelrun"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	"github.com/rawpixel-vincent/sans-nom/internal/ttsrun"
	"github.com/rawpixel-vincent/sans-nom/internal/writer"
)

type Config struct {
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
	TTS                  ttsrun.Config
	Trace                func(any) error
	LocalTurnNumbers     bool
}

type Status struct {
	Message string
	Level   string
}

type ChoiceView struct {
	ID    string
	Label string
}

type TurnView struct {
	Turn            int
	PacketID        string
	NodeID          string
	Node            scene.SceneNode
	Prose           string
	Choices         []ChoiceView
	TerminalHandoff bool
	CommitLabel     string
	AudioPath       string
	Status          []Status
}

type SubmittedDecision struct {
	ChoiceID string
	FreeText string
	RawInput string
	Exit     bool
}

type WriterRefreshFailure struct {
	Err    error
	RunID  string
	RunDir string
}

func (e WriterRefreshFailure) Error() string {
	if e.RunDir != "" {
		return fmt.Sprintf("writer refresh failed: %v (artifacts: %s)", e.Err, filepath.ToSlash(e.RunDir))
	}
	return fmt.Sprintf("writer refresh failed: %v", e.Err)
}

func (e WriterRefreshFailure) Unwrap() error {
	return e.Err
}

func RetryBlockedWriterRefreshIfNeeded(ctx context.Context, cfg Config) (Config, error) {
	state := cfg.State
	if state == nil || state.Status != story.StatusBlockedWriterRefresh {
		return cfg, nil
	}
	if !cfg.RefreshWrite {
		return cfg, fmt.Errorf("story is blocked by Writer refresh: %s (artifacts: %s)", state.BlockedReason, state.BlockedWriterRunPath)
	}
	if cfg.Workspace == nil {
		return cfg, fmt.Errorf("story is blocked by Writer refresh and has no workspace to save retry results: %s", state.BlockedReason)
	}
	if cfg.Logger == nil {
		cfg.Logger = runutil.StoryLogger(cfg.Workspace, state)
	}
	refreshCfg := cfg
	refreshCfg.State = state
	runID, runDir := modelrun.WriterRunInfo(modelConfig(refreshCfg))
	metadata := modelrun.WriterRefreshModelMetadata(modelConfig(refreshCfg))
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "writer",
		Event:    "writer.refresh.started",
		Message:  "retrying blocked Writer refresh",
		PacketID: cfg.Packet.ID,
		NodeID:   cfg.NodeID,
		RunID:    runID,
		Fields: modelmeta.MergeFields(map[string]any{
			"artifact_dir": filepath.ToSlash(runDir),
			"retry":        true,
			"blocked_run":  state.BlockedWriterRunID,
		}, metadata),
		Detail: map[string]any{
			"artifact_dir":   filepath.ToSlash(runDir),
			"blocked_reason": state.BlockedReason,
			"blocked_run":    state.BlockedWriterRunID,
			"model_metadata": metadata,
		},
	})
	next, err := RefreshPacket(ctx, cfg.Packet, cfg.NodeID, DecisionForWriterRefresh(state), refreshCfg)
	if err != nil {
		failure := writerFailure(err, runID, runDir)
		detail := writerFailureDetails(failure)
		detail["model_metadata"] = metadata
		detail["retry"] = true
		runutil.LogEvent(cfg.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "writer",
			Event:    "writer.refresh.failed",
			Message:  failure.Err.Error(),
			PacketID: cfg.Packet.ID,
			NodeID:   cfg.NodeID,
			RunID:    failure.RunID,
			Fields: modelmeta.MergeFields(map[string]any{
				"artifact_dir": filepath.ToSlash(failure.RunDir),
				"retry":        true,
			}, metadata),
			Detail: detail,
		})
		if blockErr := MarkStoryBlocked(refreshCfg, state, failure); blockErr != nil {
			return cfg, blockErr
		}
		return cfg, failure
	}
	nextNodeID := EntryNodeID(next)
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "writer",
		Event:    "writer.refresh.completed",
		Message:  "blocked Writer refresh retry completed",
		PacketID: next.ID,
		NodeID:   nextNodeID,
		RunID:    runID,
		Fields: modelmeta.MergeFields(map[string]any{
			"artifact_dir": filepath.ToSlash(runDir),
			"retry":        true,
		}, metadata),
		Detail: map[string]any{
			"artifact_dir":   filepath.ToSlash(runDir),
			"model_metadata": metadata,
			"retry":          true,
		},
	})
	if err := SaveCurrentStoryPacket(ctx, refreshCfg, state, next, nextNodeID); err != nil {
		return cfg, err
	}
	cfg.Packet = next
	cfg.NodeID = nextNodeID
	cfg.State = state
	return cfg, nil
}

func commitDisplayedTurn(ctx context.Context, cfg Config, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, audioPath string, displayTurn int) error {
	client := bridgeclient.NewClient(cfg.BridgeURL, os.Getenv("SIDECAR_TOKEN"))
	commitStarted := time.Now()
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.commit.started",
		Message:  "witnessed canon commit started",
		Turn:     displayTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"session_id":                rollout.Commit.SessionID,
			"event_count":               len(rollout.Commit.Events),
			"witness_count":             len(rollout.Commit.Witnesses),
			"observer_peer_count":       len(rollout.Commit.ObserverPeerIDs),
			"curated_card_update_count": len(rollout.Draft.CuratedCardUpdates),
		},
		Detail: map[string]any{
			"request": rollout.Commit,
		},
	})
	if err := client.CommitScene(ctx, rollout.Commit); err != nil {
		if saveErr := savePendingDisplayedTurn(cfg, state, packet, rollout, false, audioPath, displayTurn); saveErr != nil {
			err = fmt.Errorf("%w; additionally failed to save pending displayed turn: %v", err, saveErr)
		}
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.commit.completed",
		Message:  "witnessed canon committed",
		Turn:     displayTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"session_id":                rollout.Commit.SessionID,
			"elapsed_ms":                time.Since(commitStarted).Milliseconds(),
			"event_count":               len(rollout.Commit.Events),
			"witness_count":             len(rollout.Commit.Witnesses),
			"observer_peer_count":       len(rollout.Commit.ObserverPeerIDs),
			"curated_card_update_count": len(rollout.Draft.CuratedCardUpdates),
		},
		Detail: map[string]any{
			"request": rollout.Commit,
		},
	})
	return applyCardUpdatesAfterCommit(ctx, cfg, state, client, rollout.Draft.CuratedCardUpdates, displayTurn, rollout.Commit.PacketID, rollout.Commit.NodeID)
}

func applyCardUpdatesAfterCommit(ctx context.Context, cfg Config, state *story.StoryState, client bridgeclient.Client, updates []scene.CardUpdate, displayTurn int, packetID string, nodeID string) error {
	allUpdates := normalizeCardUpdates(updates)
	if state != nil && len(state.PendingCardUpdates) > 0 {
		allUpdates = append(normalizeCardUpdates(state.PendingCardUpdates), allUpdates...)
	}
	allUpdates = normalizeCardUpdates(allUpdates)
	if len(allUpdates) == 0 {
		if state != nil && len(state.PendingCardUpdates) > 0 {
			state.PendingCardUpdates = nil
			state.PendingCardUpdatesAt = ""
			if cfg.Workspace != nil {
				return cfg.Workspace.SaveState(*state)
			}
		}
		return nil
	}
	workspaceID := ""
	if state != nil {
		workspaceID = state.WorkspaceID
	}
	if workspaceID == "" {
		for _, update := range allUpdates {
			logCardUpdateFailed(cfg.Logger, displayTurn, packetID, nodeID, update, errors.New("workspace_id is required"))
		}
		return savePendingCardUpdates(cfg, state, allUpdates)
	}
	var failed []scene.CardUpdate
	for _, update := range allUpdates {
		if err := applyCardUpdate(ctx, cfg.Logger, client, workspaceID, displayTurn, packetID, nodeID, update); err != nil {
			failed = append(failed, update)
		}
	}
	return savePendingCardUpdates(cfg, state, failed)
}

func ApplyCardUpdatesAfterCommit(ctx context.Context, cfg Config, state *story.StoryState, client bridgeclient.Client, updates []scene.CardUpdate, displayTurn int, packetID string, nodeID string) error {
	return applyCardUpdatesAfterCommit(ctx, cfg, state, client, updates, displayTurn, packetID, nodeID)
}

func applyCardUpdate(ctx context.Context, logger *gamelog.Writer, client bridgeclient.Client, workspaceID string, displayTurn int, packetID string, nodeID string, update scene.CardUpdate) error {
	runutil.LogEvent(logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.card_update.request",
		Message:  "curated actor card update requested",
		Turn:     displayTurn,
		PacketID: packetID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"peer_id":        update.PeerID,
			"target_peer_id": update.TargetPeerID,
			"fact_count":     len(update.Facts),
		},
		Detail: map[string]any{
			"workspace_id": workspaceID,
			"update":       update,
		},
	})
	current, err := client.GetCard(ctx, workspaceID, update.PeerID, update.TargetPeerID)
	if err != nil {
		logCardUpdateFailed(logger, displayTurn, packetID, nodeID, update, err)
		return err
	}
	merged, added := mergeCardFacts(current, update.Facts, 40)
	if added == 0 {
		runutil.LogEvent(logger, gamelog.Event{
			Category: "honcho",
			Event:    "honcho.card_update.skipped",
			Message:  "curated actor card update already present",
			Turn:     displayTurn,
			PacketID: packetID,
			NodeID:   nodeID,
			Fields: map[string]any{
				"peer_id":        update.PeerID,
				"target_peer_id": update.TargetPeerID,
				"fact_count":     len(update.Facts),
			},
			Detail: map[string]any{
				"workspace_id": workspaceID,
				"update":       update,
				"current":      current,
			},
		})
		return nil
	}
	resp, err := client.PatchCard(ctx, workspaceID, update.PeerID, bridgeclient.PeerCardPatch{
		Facts:   merged,
		Replace: true,
		Target:  update.TargetPeerID,
	})
	if err != nil {
		logCardUpdateFailed(logger, displayTurn, packetID, nodeID, update, err)
		return err
	}
	runutil.LogEvent(logger, gamelog.Event{
		Category: "honcho",
		Event:    "honcho.card_update.completed",
		Message:  "curated actor card update applied",
		Turn:     displayTurn,
		PacketID: packetID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"peer_id":             update.PeerID,
			"target_peer_id":      update.TargetPeerID,
			"added_fact_count":    added,
			"response_fact_count": len(resp),
		},
		Detail: map[string]any{
			"workspace_id": workspaceID,
			"update":       update,
			"response":     resp,
		},
	})
	return nil
}

func logCardUpdateFailed(logger *gamelog.Writer, displayTurn int, packetID string, nodeID string, update scene.CardUpdate, err error) {
	runutil.LogEvent(logger, gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "honcho",
		Event:    "honcho.card_update.failed",
		Message:  err.Error(),
		Turn:     displayTurn,
		PacketID: packetID,
		NodeID:   nodeID,
		Fields: map[string]any{
			"peer_id":        update.PeerID,
			"target_peer_id": update.TargetPeerID,
			"fact_count":     len(update.Facts),
		},
		Detail: map[string]any{
			"update": update,
			"error":  err.Error(),
		},
	})
}

func savePendingCardUpdates(cfg Config, state *story.StoryState, failed []scene.CardUpdate) error {
	if state == nil {
		return nil
	}
	if len(failed) == 0 {
		state.PendingCardUpdates = nil
		state.PendingCardUpdatesAt = ""
	} else {
		state.PendingCardUpdates = normalizeCardUpdates(failed)
		state.PendingCardUpdatesAt = story.NowString()
	}
	if cfg.Workspace == nil {
		return nil
	}
	return cfg.Workspace.SaveState(*state)
}

func normalizeCardUpdates(updates []scene.CardUpdate) []scene.CardUpdate {
	var out []scene.CardUpdate
	seen := map[string]struct{}{}
	for _, update := range updates {
		peerID := strings.TrimSpace(update.PeerID)
		if peerID == "" {
			continue
		}
		facts := cleanFactList(update.Facts)
		if len(facts) == 0 {
			continue
		}
		target := strings.TrimSpace(update.TargetPeerID)
		key := peerID + "\x00" + target + "\x00" + strings.Join(facts, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, scene.CardUpdate{
			PeerID:       peerID,
			TargetPeerID: target,
			Facts:        facts,
			Reason:       strings.TrimSpace(update.Reason),
		})
	}
	return out
}

func mergeCardFacts(current []string, added []string, max int) ([]string, int) {
	merged := cleanFactList(current)
	seen := map[string]struct{}{}
	for _, fact := range merged {
		seen[fact] = struct{}{}
	}
	addedCount := 0
	for _, fact := range cleanFactList(added) {
		if _, ok := seen[fact]; ok {
			continue
		}
		if max > 0 && len(merged) >= max {
			break
		}
		seen[fact] = struct{}{}
		merged = append(merged, fact)
		addedCount++
	}
	return merged, addedCount
}

func cleanFactList(values []string) []string {
	var out []string
	seen := map[string]struct{}{}
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

type LoopController struct {
	cfg        Config
	packet     scene.StoryPacket
	nodeID     string
	decision   scene.PlayerDecision
	state      *story.StoryState
	turn       int
	current    *controllerTurn
	statusMu   sync.Mutex
	status     []Status
	statusSink func(Status)
}

type controllerTurn struct {
	displayTurn int
	rollout     narrator.Rollout
	audioPath   string
	committed   bool
}

func NewLoopController(cfg Config) (*LoopController, error) {
	packet := cfg.Packet
	nodeID := cfg.NodeID
	decision := cfg.InitialDecision
	var state *story.StoryState
	if cfg.State != nil {
		copied := *cfg.State
		state = &copied
	}
	if cfg.Logger == nil {
		cfg.Logger = runutil.StoryLogger(cfg.Workspace, state)
	}
	if state != nil && state.Status == story.StatusBlockedWriterRefresh {
		return nil, fmt.Errorf("story is blocked by Writer refresh: %s (artifacts: %s)", state.BlockedReason, state.BlockedWriterRunPath)
	}
	if err := recoverPendingDisplayedTurnFromLogs(cfg, state, packet, nodeID); err != nil {
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
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
	return &LoopController{
		cfg:      cfg,
		packet:   packet,
		nodeID:   nodeID,
		decision: decision,
		state:    state,
	}, nil
}

func (c *LoopController) SetStatusSink(sink func(Status)) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	c.statusSink = sink
}

func (c *LoopController) IsRecoverableError(err error) bool {
	return runutil.IsRecoverableDisconnect(err)
}

func (c *LoopController) NextTurn(ctx context.Context) (TurnView, error) {
	c.turn++
	displayTurn := c.displayTurnNumber(c.turn)
	c.resetStatus("Rendering narrator turn...")
	cfg := c.config()
	turnAudioPath := ""
	committed := false
	rollout, pendingRecord, resumedPending, err := loadPendingDisplayedRollout(cfg, c.state, c.packet, c.nodeID, c.decision, displayTurn)
	if err != nil {
		return TurnView{}, err
	}
	if resumedPending {
		if pendingRecord.Turn > 0 {
			displayTurn = pendingRecord.Turn
		}
		turnAudioPath = pendingRecord.AudioPath
		committed = pendingRecord.Committed
		c.addStatus("", "Resumed a displayed turn awaiting player input.")
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
				return TurnView{}, err
			}
			committed = true
			if err := savePendingDisplayedTurn(cfg, c.state, c.packet, rollout, true, turnAudioPath, displayTurn); err != nil {
				return TurnView{}, err
			}
			c.addStatus("", "Pending Honcho commit completed.")
		}
	} else {
		rollout, err = rolloutOnce(ctx, c.packet, c.nodeID, c.decision, cfg, displayTurn)
		if err != nil {
			metadata := modelrun.NarratorModelMetadata(modelConfig(cfg))
			runutil.LogEvent(cfg.Logger, gamelog.Event{
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
			return TurnView{}, err
		}
		metadata := modelrun.NarratorModelMetadata(modelConfig(cfg))
		runutil.LogEvent(cfg.Logger, gamelog.Event{
			Category: "narrator",
			Event:    "narrator.rollout.completed",
			Message:  "narrator rollout completed",
			Turn:     displayTurn,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: modelmeta.MergeFields(map[string]any{
				"actor_raw_count":              len(rollout.ActorRaw),
				"choice_count":                 len(rollout.Draft.Choices),
				"prose_chars":                  len([]rune(rollout.Draft.Prose)),
				"surfaced_actor_message_count": len(rollout.Draft.SurfacedActorMessages),
				"curated_card_update_count":    len(rollout.Draft.CuratedCardUpdates),
				"commit_event_count":           len(rollout.Commit.Events),
				"witness_count":                len(rollout.Commit.Witnesses),
				"observer_peer_count":          len(rollout.Commit.ObserverPeerIDs),
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
			return TurnView{}, err
		}
		if cfg.Commit {
			c.addStatus("", "Committing witnessed canon...")
			if err := commitDisplayedTurn(ctx, cfg, c.state, c.packet, rollout, turnAudioPath, displayTurn); err != nil {
				return TurnView{}, err
			}
			committed = true
			c.addStatus("", "Honcho commit completed.")
		}
		if err := savePendingDisplayedTurn(cfg, c.state, c.packet, rollout, cfg.Commit, turnAudioPath, displayTurn); err != nil {
			return TurnView{}, err
		}
		c.addStatus("", "Turn checkpoint saved.")
	}
	if cfg.TraceJSONL {
		if err := writePlayTrace(cfg, playTraceTurn{
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
			return TurnView{}, err
		}
	}
	terminalHandoff := len(rollout.Node.Choices) == 0 && cfg.RefreshWrite
	if terminalHandoff {
		if bridge := TerminalHandoffBridge(rollout.Node); bridge != "" {
			c.addStatus("", bridge)
		}
	}
	c.current = &controllerTurn{
		displayTurn: displayTurn,
		rollout:     rollout,
		audioPath:   turnAudioPath,
		committed:   committed,
	}
	return TurnView{
		Turn:            displayTurn,
		PacketID:        rollout.Commit.PacketID,
		NodeID:          rollout.Commit.NodeID,
		Node:            rollout.Node,
		Prose:           rollout.Draft.Prose,
		Choices:         choiceViews(rollout),
		TerminalHandoff: terminalHandoff,
		CommitLabel:     CommitLabel(rollout, committed, cfg.Commit),
		AudioPath:       filepath.ToSlash(turnAudioPath),
		Status:          c.statusSnapshot(),
	}, nil
}

func (c *LoopController) displayTurnNumber(loopTurn int) int {
	if c.cfg.LocalTurnNumbers && loopTurn > 0 {
		return loopTurn
	}
	return currentStoryTurnNumber(c.state, loopTurn)
}

func (c *LoopController) Submit(ctx context.Context, submitted SubmittedDecision) (bool, error) {
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
		rawInput = runutil.FirstNonEmpty(nextDecision.ChoiceID, nextDecision.FreeText)
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
	if targetNodeID, ok := ResolvedChoiceTarget(c.packet, rollout.Node, nextDecision); ok {
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, targetNodeID, nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		c.nodeID = targetNodeID
		c.decision = nextDecision
		c.current = nil
		if cfg.TraceJSONL {
			if err := writePlayTrace(cfg, decisionTrace(rawInput, nextDecision, targetNodeID)); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if cfg.RefreshWrite {
		refreshCfg := cfg
		refreshCfg.State = c.state
		runID, runDir := modelrun.WriterRunInfo(modelConfig(refreshCfg))
		metadata := modelrun.WriterRefreshModelMetadata(modelConfig(refreshCfg))
		c.resetStatus("Refreshing Writer packet...")
		if err := savePendingDecision(cfg, c.state, nextDecision, runID, runDir); err != nil {
			return false, err
		}
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
		writerDecision := DecisionForWriterRefreshAfterSubmit(nextDecision, rollout.Commit.PlayerDecision, c.state)
		next, err := RefreshPacket(ctx, c.packet, c.nodeID, writerDecision, refreshCfg)
		if err == nil {
			nextNodeID := EntryNodeID(next)
			runutil.LogEvent(cfg.Logger, gamelog.Event{
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
			if err := SaveCurrentStoryPacket(ctx, cfg, c.state, next, nextNodeID); err != nil {
				return false, err
			}
			c.packet = next
			c.nodeID = nextNodeID
			c.decision = nextDecision
			c.current = nil
			if cfg.TraceJSONL {
				if err := writePlayTrace(cfg, decisionTrace(rawInput, nextDecision, nextNodeID)); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		if runutil.IsRecoverableDisconnect(err) {
			return false, err
		}
		failure := writerFailure(err, runID, runDir)
		detail := writerFailureDetails(failure)
		detail["model_metadata"] = metadata
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
		if err := MarkStoryBlocked(cfg, c.state, failure); err != nil {
			return false, err
		}
		if err := persistStoryTurn(ctx, cfg, c.state, c.packet, rollout, "", nextDecision, cfg.Commit, c.current.audioPath); err != nil {
			return false, err
		}
		if cfg.TraceJSONL {
			if err := writePlayTrace(cfg, decisionTrace(rawInput, nextDecision, "")); err != nil {
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
			if err := writePlayTrace(cfg, decisionTrace(rawInput, nextDecision, "")); err != nil {
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

func (c *LoopController) config() Config {
	cfg := c.cfg
	cfg.State = c.state
	cfg.Status = c.reportStatus
	return cfg
}

func (c *LoopController) resetStatus(message string) {
	c.statusMu.Lock()
	c.status = nil
	c.statusMu.Unlock()
	c.reportStatus(message)
}

func (c *LoopController) reportStatus(message string) {
	c.addStatus("", message)
}

func (c *LoopController) addStatus(level string, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	status := Status{Message: message, Level: level}
	c.statusMu.Lock()
	c.status = append(c.status, status)
	sink := c.statusSink
	c.statusMu.Unlock()
	if sink != nil {
		sink(status)
	}
}

func (c *LoopController) statusSnapshot() []Status {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	return append([]Status{}, c.status...)
}

func choiceViews(rollout narrator.Rollout) []ChoiceView {
	choices := rollout.Draft.Choices
	if len(choices) == 0 {
		choices = rollout.Node.Choices
	}
	views := make([]ChoiceView, 0, len(choices))
	for _, choice := range choices {
		views = append(views, ChoiceView{ID: choice.ID, Label: choice.Label})
	}
	return views
}

func CommitLabel(rollout narrator.Rollout, committed bool, commitEnabled bool) string {
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

func TerminalHandoffBridge(node scene.SceneNode) string {
	for _, note := range node.RevisionNotes {
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		note = firstSentence(note)
		note = strings.TrimPrefix(note, "After this branch is narrated, ")
		note = strings.TrimPrefix(note, "After this branch is narrated,")
		note = strings.TrimPrefix(note, "Next packet from this branch should ")
		note = strings.TrimPrefix(note, "Continuation should ")
		note = strings.TrimSpace(note)
		if note == "" {
			continue
		}
		if isReflectionHandoff(note) {
			return "[Reflect] " + upperFirst(firstSentence(note))
		}
		return "The scene carries forward: " + lowerFirst(note)
	}
	return ""
}

func isReflectionHandoff(note string) bool {
	note = strings.ToLower(note)
	for _, marker := range []string{"reflect", "answer", "moral question", "vow", "confession"} {
		if strings.Contains(note, marker) {
			return true
		}
	}
	return false
}

func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	for _, sep := range []string{". ", "! ", "? ", "\n"} {
		if index := strings.Index(text, sep); index >= 0 {
			end := index + 1
			if sep == "\n" {
				end = index
			}
			return strings.TrimSpace(text[:end])
		}
	}
	return text
}

func lowerFirst(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	runes[0] = []rune(strings.ToLower(string(runes[0])))[0]
	return string(runes)
}

func upperFirst(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func recoverPendingDisplayedTurnFromLogs(cfg Config, state *story.StoryState, packet scene.StoryPacket, nodeID string) error {
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
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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

func loadPendingDisplayedRollout(cfg Config, state *story.StoryState, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, loopTurn int) (narrator.Rollout, story.TurnRecord, bool, error) {
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
	resolvedNodeID := runutil.FirstNonEmpty(record.NodeID, nodeID)
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
			Prose:              record.Prose,
			Choices:            choices,
			CuratedCardUpdates: record.CardUpdates,
		},
		Commit: record.Commit,
	}
	return rollout, record, true, nil
}

func isAwaitingTurnInputStatus(status string) bool {
	return status == story.StatusAwaitingPlayerChoice || status == story.StatusAwaitingContinuation
}

func savePendingDisplayedTurn(cfg Config, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, committed bool, audioPath string, loopTurn int) error {
	if cfg.Workspace == nil || state == nil {
		return nil
	}
	awaitingContinuation := len(rollout.Node.Choices) == 0 && len(rollout.Node.RevisionNotes) > 0
	record := story.TurnRecord{
		Turn:     currentStoryTurnNumber(state, loopTurn),
		StoryID:  state.StoryID,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Decision: rollout.Commit.PlayerDecision,
		Prose:    rollout.Draft.Prose,
		Choices:  rollout.Node.Choices,
		Commit:   rollout.Commit,
		CardUpdates: append([]scene.CardUpdate(nil),
			rollout.Draft.CuratedCardUpdates...),
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
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

func SavePendingDisplayedTurn(cfg Config, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, committed bool, audioPath string, loopTurn int) error {
	return savePendingDisplayedTurn(cfg, state, packet, rollout, committed, audioPath, loopTurn)
}

func rolloutOnce(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg Config, turn int) (narrator.Rollout, error) {
	registry, err := runutil.ActorRegistry(cfg.CardStore)
	if err != nil {
		return narrator.Rollout{}, err
	}
	packet, removedNeeds := actors.PruneUnsupportedPacketNeeds(packet, registry)
	modelrun.LogUnsupportedActorNeeds(cfg.Logger, "actor.needs.filtered", "unsupported actor_needs filtered before narrator context assembly", turn, packet.ID, nodeID, removedNeeds)
	assembler := modelrun.ContextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.GlobalCollection, cfg.StoryCollection, cfg.Logger)
	assembler.Options.SkipMissingActorCards = true
	scenarioContract := lorestage.ScenarioContractTextForPack(cfg.GroundLorePackID)
	modelrun.LogLoreFidelityContext(cfg.Logger, "narrator", "", turn, packet.ID, nodeID, cfg.GroundLorePackID, scenarioContract)
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
	fullNarratorContextText := narratorContext.RenderText()
	narratorContextText := fullNarratorContextText
	var narratorBudget map[string]any
	actorInvoker := actors.Invoker(actors.StaticInvoker{})
	renderer := narrator.Renderer(narrator.StaticRenderer{})
	switch cfg.Backend {
	case "claude":
		runner := modelrun.ClaudeRunner()
		runner.SystemPrompt = lorestage.MasterLoreSystemPromptForPackID(cfg.GroundLorePackID, runner.SystemPrompt)
		inline, report := narratorContext.RenderBudgetedText(modelrun.ContextBudgetFor(runner.ModelMetadata(), "narrator", false, false, 0))
		narratorContextText = inline
		narratorBudget = modelrun.BudgetReportMap(report)
		actorInvoker = actors.ClaudeInvoker{Runner: runner}
		renderer = narrator.ClaudeRenderer{Runner: runner}
	case "codex":
		actorInvoker = actors.ClaudeInvoker{Runner: modelrun.DirectorCodexRunner("ActorOutput", prompt.ActorOutputSchema(), cfg.GroundLorePackID)}
		scope, err := modelrun.PrepareStoryAgentScope(modelConfig(cfg), "narrator", fmt.Sprintf("turn-%06d", turn), packet, nodeID, fullNarratorContextText)
		if err != nil {
			return narrator.Rollout{}, err
		}
		runner := modelrun.CodexRunnerWithReadTools("NarratorDraft", prompt.NarratorDraftSchema(), scope, cfg.Logger, "narrator", "narrator", "Narrator", turn, packet.ID, nodeID)
		inline, report := narratorContext.RenderBudgetedText(modelrun.ContextBudgetFor(runner.ModelMetadata(), "narrator", runner.LocalTools, runner.SchemaName != "" && runner.Schema != nil, runner.MaxToolRounds))
		if err := modelrun.StageContextBudgetArtifacts(scope, &report, inline); err != nil {
			return narrator.Rollout{}, err
		}
		narratorContextText = inline
		narratorBudget = modelrun.BudgetReportMap(report)
		runner.ContextBudget = narratorBudget
		renderer = narrator.ClaudeRenderer{Runner: runner}
	}
	runID := fmt.Sprintf("turn-%06d", turn)
	selectedNode, err := packet.Node(nodeID)
	if err != nil {
		return narrator.Rollout{}, err
	}
	rehearsal, err := director.Service{
		Actors:           &actors.Service{Registry: registry, Invoker: actorInvoker, Logger: cfg.Logger, PacketID: packet.ID, Turn: turn, RunID: runID, SkipMissingActors: true},
		Logger:           cfg.Logger,
		Turn:             turn,
		RunID:            runID,
		ActorContextText: narratorContext.RenderActorText(),
		ArtifactDir:      modelrun.DirectorRunDir(modelConfig(cfg), turn),
	}.Rehearse(ctx, packet, selectedNode, narratorContextText)
	if err != nil {
		return narrator.Rollout{}, err
	}
	return narrator.Service{
		Renderer:      renderer,
		Logger:        cfg.Logger,
		Turn:          turn,
		RunID:         runID,
		ContextBudget: narratorBudget,
	}.Rollout(ctx, packet, nodeID, decision, narratorContextText, rehearsal)
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

func synthesizeTurnAudio(ctx context.Context, cfg Config, state *story.StoryState, loopTurn int, rollout narrator.Rollout) (string, error) {
	return ttsrun.SynthesizeTurnAudio(ctx, cfg.TTS, cfg.Workspace, state, cfg.Logger, loopTurn, rollout)
}

func modelConfig(cfg Config) modelrun.Config {
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

func RefreshPacket(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, cfg Config) (scene.StoryPacket, error) {
	runID, runDir := modelrun.WriterRunInfo(modelConfig(cfg))
	assembler := modelrun.WriterRefreshContextAssembler(cfg.BridgeURL, false, cfg.CardStore, cfg.StoryCollection, cfg.Logger)
	if cfg.WriterScenaristCalls != 0 {
		assembler.Options.WriterLoreK = 4
	}
	scenarioContract := lorestage.ScenarioContractTextForPack(cfg.GroundLorePackID)
	modelrun.LogLoreFidelityContext(cfg.Logger, "writer", runID, 0, packet.ID, nodeID, cfg.GroundLorePackID, scenarioContract)
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
	fullWriterContextText := writerContext.RenderText()
	writerContextText, writerBudgetReport := writerContext.RenderBudgetedText(modelrun.ContextBudgetFor(modelrun.WriterRefreshModelMetadata(modelConfig(cfg)), "writer", cfg.Backend != "static", cfg.Backend == "codex", modelrun.WriterToolRoundBudget(cfg.ScenaristRounds)))
	writerBudget := modelrun.BudgetReportMap(writerBudgetReport)
	scope, err := modelrun.PrepareStoryAgentScope(modelConfig(cfg), "writer", runID, packet, nodeID, fullWriterContextText)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	if err := modelrun.StageContextBudgetArtifacts(scope, &writerBudgetReport, writerContextText); err != nil {
		return scene.StoryPacket{}, err
	}
	outputDir := filepath.Join(scope.Root, "scenarists")
	runner := modelrun.ClaudeRunner()
	loreExpertTool := modelrun.LoreExpertToolRunner(modelConfig(cfg))
	toolRunner, err := modelrun.ScenaristToolRunner(modelConfig(cfg), writerContextText, outputDir, scope, loreExpertTool)
	if err != nil {
		return scene.StoryPacket{}, err
	}
	if toolRunner != nil && cfg.Backend == "claude" {
		return scene.StoryPacket{}, fmt.Errorf("scenarist Writer tool is only supported for codex backend; use -model-backend codex or -writer-scenarist-calls 0")
	}
	planner := cfg.WriterPlanner
	if planner == nil {
		actorTool, err := modelrun.WriterActorToolRunner(ctx, modelConfig(cfg), writerContext, scope, packet, nodeID, decision, runID)
		if err != nil {
			return scene.StoryPacket{}, err
		}
		planner = modelrun.WriterPlannerWithMaterialAccess(cfg.Backend, runner, scope, toolRunner, loreExpertTool, actorTool, cfg.ScenaristRounds, cfg.Logger, runID, writerBudget)
	}
	if toolRunner != nil {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return scene.StoryPacket{}, err
		}
	}
	waitCtx, cancel := context.WithTimeout(ctx, modelrun.WriterAwaitTimeout())
	defer cancel()
	job, err := writer.Service{Planner: planner, Validator: writer.ValidateProgression}.RefreshAsync(waitCtx, writer.Input{
		Current:              packet,
		LatestDecision:       DecisionText(decision.ChoiceID, decision.FreeText),
		ContextText:          writerContextText,
		RunID:                runID,
		ArtifactDir:          runDir,
		SpentMotifs:          writerContext.Progression.SpentMotifs,
		RequiredMovement:     writerContext.Progression.RequiredMovement,
		ContextBudget:        writerBudget,
		RegisteredActorIDs:   modelrun.WriterActorIDs(writerContext.ActorManifest),
		ActorManifestSummary: modelrun.WriterActorManifestSummary(writerContext.ActorManifest),
	})
	if err != nil {
		return scene.StoryPacket{}, writerFailure(err, runID, runDir)
	}
	result := job.Await(waitCtx)
	if result.Err != nil {
		return result.Packet, writerFailure(result.Err, runID, runDir)
	}
	nextPacket := result.Packet
	if registry, err := runutil.ActorRegistry(cfg.CardStore); err == nil {
		var removedNeeds []actors.UnsupportedActorNeed
		nextPacket, removedNeeds = actors.PruneUnsupportedPacketNeeds(nextPacket, registry)
		modelrun.LogUnsupportedActorNeeds(cfg.Logger, "writer.packet.actor_needs.filtered", "unsupported actor_needs filtered from Writer packet", 0, nextPacket.ID, "", removedNeeds)
	}
	return nextPacket, nil
}

func persistStoryTurn(ctx context.Context, cfg Config, state *story.StoryState, packet scene.StoryPacket, rollout narrator.Rollout, nextNodeID string, nextDecision scene.PlayerDecision, committed bool, audioPath ...string) error {
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
		CardUpdates:  append([]scene.CardUpdate(nil), rollout.Draft.CuratedCardUpdates...),
		Committed:    committed,
		NextNodeID:   nextNodeID,
		AudioPath:    filepath.ToSlash(turnAudioPath),
	}
	nextState, err := cfg.Workspace.RecordTurn(*state, packet, record)
	if err != nil {
		return err
	}
	*state = nextState
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "memory",
		Event:    "story.turn.persisted",
		Message:  "turn persisted to local story workspace",
		Turn:     nextState.LatestTurn,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: map[string]any{
			"committed":           committed,
			"next_node_id":        nextNodeID,
			"choice_id":           rollout.Commit.PlayerDecision.ChoiceID,
			"has_free_text":       rollout.Commit.PlayerDecision.FreeText != "",
			"next_choice_id":      nextDecision.ChoiceID,
			"next_has_free_text":  nextDecision.FreeText != "",
			"card_update_count":   len(rollout.Draft.CuratedCardUpdates),
			"observer_peer_count": len(rollout.Commit.ObserverPeerIDs),
		},
		Detail: map[string]any{
			"turn_record":   record,
			"next_decision": nextDecision,
		},
	})
	return refreshStoryIndex(ctx, cfg, state, packet)
}

func SaveCurrentStoryPacket(ctx context.Context, cfg Config, state *story.StoryState, packet scene.StoryPacket, nodeID string) error {
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{Category: "story", Event: "story.packet.saved", Message: "current story packet saved", PacketID: packet.ID, NodeID: nodeID})
	return refreshStoryIndex(ctx, cfg, state, packet)
}

func refreshStoryIndex(ctx context.Context, cfg Config, state *story.StoryState, packet scene.StoryPacket) error {
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
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
		Documents:  runutil.BridgeStoryDocs(docs),
		Replace:    true,
	}
	ingestResp, err := client.IngestStoryIndex(ctx, ingestReq)
	if err != nil {
		runutil.LogEvent(cfg.Logger, gamelog.Event{
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
		Category: "qdrant",
		Event:    "story.index.ingested",
		Message:  "story index ingested",
		Fields:   map[string]any{"collection": state.StoryCollection, "document_count": len(docs), "response_count": ingestResp.Count, "replace": true},
		Detail: map[string]any{
			"request":  ingestReq,
			"response": ingestResp,
		},
	})
	return nil
}

func RefreshStoryIndex(ctx context.Context, cfg Config, state *story.StoryState, packet scene.StoryPacket) error {
	return refreshStoryIndex(ctx, cfg, state, packet)
}

func EntryNodeID(packet scene.StoryPacket) string {
	if packet.EntryNodeID != "" {
		return packet.EntryNodeID
	}
	if node, err := packet.EntryNode(); err == nil {
		return node.ID
	}
	return ""
}

func ResolvedStoryCollection(state *story.StoryState, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if state != nil && state.StoryCollection != "" {
		return state.StoryCollection
	}
	return ""
}

func SourceLoreCollection(state *story.StoryState, fallback string) string {
	if state != nil {
		if collection := state.SourceLoreCollection(); collection != "" {
			return collection
		}
	}
	return runutil.FirstNonEmpty(fallback, story.DefaultGroundLoreCollection)
}

func SourceLorePackID(state *story.StoryState, fallback string) string {
	if state != nil {
		if packID := state.SourceLorePackID(); packID != "" {
			return packID
		}
	}
	return runutil.FirstNonEmpty(fallback, story.DefaultGroundLorePackID)
}

func DecisionForWriterRefresh(state *story.StoryState) scene.PlayerDecision {
	if state != nil && state.HasPendingDecision() {
		return state.PendingDecision
	}
	if state != nil {
		return state.LastDecision
	}
	return scene.PlayerDecision{}
}

func DecisionForWriterRefreshAfterSubmit(submitted scene.PlayerDecision, displayed scene.PlayerDecision, state *story.StoryState) scene.PlayerDecision {
	if !story.PlayerDecisionEmpty(submitted) {
		return submitted
	}
	if state != nil && state.HasPendingDecision() {
		return state.PendingDecision
	}
	if !story.PlayerDecisionEmpty(displayed) {
		return displayed
	}
	return DecisionForWriterRefresh(state)
}

func savePendingDecision(cfg Config, state *story.StoryState, decision scene.PlayerDecision, runID string, runDir string) error {
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
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

func MarkStoryBlocked(cfg Config, state *story.StoryState, failure WriterRefreshFailure) error {
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
	runutil.LogEvent(cfg.Logger, gamelog.Event{
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

func writerFailure(err error, runID string, runDir string) WriterRefreshFailure {
	if failure, ok := AsWriterFailure(err); ok {
		if failure.RunID == "" {
			failure.RunID = runID
		}
		if failure.RunDir == "" {
			failure.RunDir = runDir
		}
		return failure
	}
	return WriterRefreshFailure{Err: err, RunID: runID, RunDir: runDir}
}

func writerFailureDetails(failure WriterRefreshFailure) map[string]any {
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

func AsWriterFailure(err error) (WriterRefreshFailure, bool) {
	var failure WriterRefreshFailure
	if errors.As(err, &failure) {
		return failure, true
	}
	return WriterRefreshFailure{}, false
}

func AutomatedDecision(node scene.SceneNode) scene.PlayerDecision {
	if len(node.Choices) > 0 {
		return scene.PlayerDecision{ChoiceID: node.Choices[0].ID}
	}
	return scene.PlayerDecision{FreeText: "wait"}
}

func DecisionText(choiceID string, freeText string) string {
	if freeText != "" {
		return freeText
	}
	if choiceID != "" {
		return choiceID
	}
	return "observe"
}

func ParseDecision(line string, node scene.SceneNode) scene.PlayerDecision {
	if line == "" {
		return AutomatedDecision(node)
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

func ResolvedChoiceTarget(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision) (string, bool) {
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

func writePlayTrace(cfg Config, event any) error {
	if cfg.Trace == nil {
		return nil
	}
	return cfg.Trace(event)
}
