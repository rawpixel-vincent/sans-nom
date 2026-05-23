package narrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Renderer interface {
	Render(ctx context.Context, node scene.SceneNode, rehearsal RehearsalResult, context string) (Draft, error)
}

type Draft struct {
	Prose                 string               `json:"prose"`
	SurfacedActorMessages []scene.ActorMessage `json:"surfaced_actor_messages,omitempty"`
	Choices               []scene.Choice       `json:"choices,omitempty"`
	CuratedCardUpdates    []scene.CardUpdate   `json:"curated_card_updates,omitempty"`
}

type Service struct {
	Actors               *actors.Service
	Renderer             Renderer
	Logger               *gamelog.Writer
	Turn                 int
	RunID                string
	ActorContextText     string
	RehearsalArtifactDir string
	ContextBudget        any
}

type Rollout struct {
	Node         scene.SceneNode
	ActorRaw     []actors.Output
	Rehearsal    RehearsalResult
	Draft        Draft
	Commit       scene.Commit
	NextChoiceID string
}

type RehearsalResult struct {
	PacketID      string             `json:"packet_id"`
	NodeID        string             `json:"node_id"`
	RunID         string             `json:"run_id,omitempty"`
	Turn          int                `json:"turn,omitempty"`
	ActorNeeds    []scene.ActorNeed  `json:"actor_needs,omitempty"`
	ActorOutputs  []actors.Output    `json:"actor_outputs"`
	Warnings      []string           `json:"warnings,omitempty"`
	ArtifactDir   string             `json:"artifact_dir,omitempty"`
	ArtifactPaths []string           `json:"artifact_paths,omitempty"`
	ModelMetadata modelmeta.Metadata `json:"model_metadata,omitempty"`
	NonCanonScope string             `json:"non_canon_scope"`
}

func (s Service) Rollout(ctx context.Context, packet scene.StoryPacket, nodeID string, decision scene.PlayerDecision, context string) (Rollout, error) {
	if s.Renderer == nil {
		return Rollout{}, errors.New("narrator renderer is required")
	}
	node, err := selectedNode(packet, nodeID)
	if err != nil {
		return Rollout{}, err
	}
	node, removedActorNeeds := filterUnsupportedActorNeeds(node, s.Actors)
	if len(removedActorNeeds) > 0 {
		logEvent(s.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "actor",
			Event:    "actor.needs.filtered",
			Message:  "unsupported actor_needs filtered before narrator rollout",
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			Fields: map[string]any{
				"removed_count": len(removedActorNeeds),
			},
			Detail: map[string]any{
				"removed_actor_needs": removedActorNeeds,
			},
		})
	}

	rehearsal, err := s.runRehearsal(ctx, packet, node, context)
	if err != nil {
		return Rollout{}, err
	}
	actorRaw := rehearsal.ActorOutputs
	rawActors, _ := json.Marshal(rehearsal)
	promptText := prompt.NarratorPrompt(node, string(rawActors), context)
	metadata := rendererModelMetadata(s.Renderer)
	logEvent(s.Logger, gamelog.Event{
		Category: "narrator",
		Event:    "narrator.rollout.request",
		Message:  "narrator rollout requested",
		Turn:     s.Turn,
		PacketID: packet.ID,
		NodeID:   node.ID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_output_count": len(actorRaw),
			"context_chars":      len([]rune(context)),
			"prompt_chars":       len([]rune(promptText)),
			"decision_choice_id": decision.ChoiceID,
			"decision_free_text": decision.FreeText,
			"context_budget":     s.ContextBudget,
		}, metadata),
		Detail: map[string]any{
			"prompt":         promptText,
			"context_text":   context,
			"node":           node,
			"rehearsal":      rehearsal,
			"actor_outputs":  actorRaw,
			"decision":       decision,
			"model_metadata": metadata,
			"context_budget": s.ContextBudget,
		},
	})
	draft, err := s.Renderer.Render(ctx, node, rehearsal, context)
	if err != nil {
		logEvent(s.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "narrator",
			Event:    "narrator.rollout.failed",
			Message:  err.Error(),
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			Detail: map[string]any{
				"prompt":         promptText,
				"context_text":   context,
				"node":           node,
				"rehearsal":      rehearsal,
				"actor_outputs":  actorRaw,
				"decision":       decision,
				"error":          err.Error(),
				"model_metadata": metadata,
				"context_budget": s.ContextBudget,
			},
		})
		return Rollout{}, err
	}
	draft, droppedSurfaces := filterUnsupportedActorSurfaces(draft, actorRaw)
	if len(droppedSurfaces) > 0 {
		logEvent(s.Logger, gamelog.Event{
			Level:    gamelog.LevelWarn,
			Category: "narrator",
			Event:    "narrator.actor_surface.filtered",
			Message:  "unsupported surfaced actor messages filtered before commit",
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			Fields: map[string]any{
				"removed_count": len(droppedSurfaces),
			},
			Detail: map[string]any{
				"removed_actor_messages": droppedSurfaces,
			},
		})
	}
	commit := buildCommit(packet, node, commitDecision(packet, node, decision), draft, observerPeerIDs(packet, node, actorRaw, s.Actors))
	if err := commit.ValidateCanonOnly(); err != nil {
		logEvent(s.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "narrator",
			Event:    "narrator.rollout.failed",
			Message:  err.Error(),
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			Detail: map[string]any{
				"prompt":         promptText,
				"context_text":   context,
				"node":           node,
				"rehearsal":      rehearsal,
				"actor_outputs":  actorRaw,
				"draft":          draft,
				"decision":       decision,
				"error":          err.Error(),
				"model_metadata": metadata,
			},
		})
		return Rollout{}, err
	}
	return Rollout{Node: node, ActorRaw: actorRaw, Rehearsal: rehearsal, Draft: draft, Commit: commit}, nil
}

func (s Service) runRehearsal(ctx context.Context, packet scene.StoryPacket, node scene.SceneNode, narratorContext string) (RehearsalResult, error) {
	result := RehearsalResult{
		PacketID:      packet.ID,
		NodeID:        node.ID,
		RunID:         s.RunID,
		Turn:          s.Turn,
		ActorNeeds:    append([]scene.ActorNeed(nil), node.ActorNeeds...),
		ActorOutputs:  []actors.Output{},
		ArtifactDir:   filepath.ToSlash(s.RehearsalArtifactDir),
		NonCanonScope: "Actor rehearsal is backstage planning/performance material. Only Narrator surfaced_actor_messages and curated_card_updates can become witnessed actor memory.",
	}
	if s.Actors == nil || len(node.ActorNeeds) == 0 {
		return result, nil
	}

	actorService := *s.Actors
	actorService.PacketID = packet.ID
	actorService.Turn = s.Turn
	actorContext := narratorContext
	if strings.TrimSpace(s.ActorContextText) != "" {
		actorContext = s.ActorContextText
	}
	metadata := actorInvokerModelMetadata(actorService.Invoker)
	result.ModelMetadata = metadata

	logEvent(s.Logger, gamelog.Event{
		Category: "rehearsal",
		Event:    "rehearsal.started",
		Message:  "actor rehearsal started",
		Turn:     s.Turn,
		PacketID: packet.ID,
		NodeID:   node.ID,
		RunID:    s.RunID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_need_count": len(node.ActorNeeds),
			"artifact_dir":     filepath.ToSlash(s.RehearsalArtifactDir),
		}, metadata),
		Detail: map[string]any{
			"node":            node,
			"actor_needs":     node.ActorNeeds,
			"actor_context":   actorContext,
			"non_canon_scope": result.NonCanonScope,
			"artifact_dir":    filepath.ToSlash(s.RehearsalArtifactDir),
			"model_metadata":  metadata,
		},
	})
	for _, need := range node.ActorNeeds {
		logEvent(s.Logger, gamelog.Event{
			Category: "rehearsal",
			Event:    "rehearsal.actor.request",
			Message:  "actor rehearsal requested",
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			RunID:    s.RunID,
			Fields: modelmeta.MergeFields(map[string]any{
				"actor_id": need.ActorID,
				"mode":     need.Mode,
				"reason":   need.Reason,
			}, metadata),
			Detail: map[string]any{
				"need":            need,
				"node":            node,
				"non_canon_scope": result.NonCanonScope,
				"model_metadata":  metadata,
			},
		})
	}

	outputs, err := actorService.InvokeForNode(ctx, node, actorContext)
	if err != nil {
		logEvent(s.Logger, gamelog.Event{
			Level:    gamelog.LevelError,
			Category: "rehearsal",
			Event:    "rehearsal.failed",
			Message:  err.Error(),
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			RunID:    s.RunID,
			Fields: modelmeta.MergeFields(map[string]any{
				"actor_need_count": len(node.ActorNeeds),
			}, metadata),
			Detail: map[string]any{
				"node":            node,
				"actor_needs":     node.ActorNeeds,
				"error":           err.Error(),
				"non_canon_scope": result.NonCanonScope,
				"model_metadata":  metadata,
			},
		})
		return result, err
	}
	result.ActorOutputs = outputs
	for _, output := range outputs {
		logEvent(s.Logger, gamelog.Event{
			Category: "rehearsal",
			Event:    "rehearsal.actor.completed",
			Message:  "actor rehearsal completed",
			Turn:     s.Turn,
			PacketID: packet.ID,
			NodeID:   node.ID,
			RunID:    s.RunID,
			Fields: modelmeta.MergeFields(map[string]any{
				"actor_id":     output.ActorID,
				"has_dialogue": strings.TrimSpace(output.Dialogue) != "",
				"has_action":   strings.TrimSpace(output.Action) != "",
				"has_interior": strings.TrimSpace(output.Interior) != "",
			}, metadata),
			Detail: map[string]any{
				"output":          output,
				"non_canon_scope": result.NonCanonScope,
				"model_metadata":  metadata,
			},
		})
	}
	if err := s.writeRehearsalArtifacts(&result, node); err != nil {
		result.Warnings = append(result.Warnings, "write rehearsal artifacts: "+err.Error())
	}
	logEvent(s.Logger, gamelog.Event{
		Category: "rehearsal",
		Event:    "rehearsal.completed",
		Message:  "actor rehearsal completed",
		Turn:     s.Turn,
		PacketID: packet.ID,
		NodeID:   node.ID,
		RunID:    s.RunID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_output_count": len(result.ActorOutputs),
			"artifact_dir":       result.ArtifactDir,
			"artifact_count":     len(result.ArtifactPaths),
		}, metadata),
		Detail: map[string]any{
			"rehearsal":      result,
			"artifact_dir":   result.ArtifactDir,
			"artifact_paths": result.ArtifactPaths,
			"model_metadata": metadata,
		},
	})
	return result, nil
}

func (s Service) writeRehearsalArtifacts(result *RehearsalResult, node scene.SceneNode) error {
	if strings.TrimSpace(s.RehearsalArtifactDir) == "" {
		return nil
	}
	if err := os.MkdirAll(s.RehearsalArtifactDir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name  string
		value any
	}{
		{"selected-node.json", node},
		{"actor-outputs.json", result.ActorOutputs},
		{"manifest.json", result},
	}
	for _, file := range files {
		path := filepath.Join(s.RehearsalArtifactDir, file.name)
		if err := writeJSONFile(path, file.value); err != nil {
			return err
		}
		result.ArtifactPaths = append(result.ArtifactPaths, filepath.ToSlash(path))
	}
	result.ArtifactDir = filepath.ToSlash(s.RehearsalArtifactDir)
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func filterUnsupportedActorNeeds(node scene.SceneNode, service *actors.Service) (scene.SceneNode, []actors.UnsupportedActorNeed) {
	if service == nil || service.Registry == nil {
		return node, nil
	}
	return actors.FilterUnsupportedNeeds(node, service.Registry)
}

func filterUnsupportedActorSurfaces(draft Draft, actorRaw []actors.Output) (Draft, []scene.ActorMessage) {
	if len(draft.SurfacedActorMessages) == 0 {
		return draft, nil
	}
	supported := map[string]struct{}{}
	for _, output := range actorRaw {
		if strings.TrimSpace(output.ActorID) != "" {
			supported[output.ActorID] = struct{}{}
		}
	}
	filtered := draft
	filtered.SurfacedActorMessages = make([]scene.ActorMessage, 0, len(draft.SurfacedActorMessages))
	var dropped []scene.ActorMessage
	for _, msg := range draft.SurfacedActorMessages {
		if _, ok := supported[msg.ActorID]; ok {
			filtered.SurfacedActorMessages = append(filtered.SurfacedActorMessages, msg)
			continue
		}
		dropped = append(dropped, msg)
	}
	if len(dropped) == 0 {
		return draft, nil
	}
	return filtered, dropped
}

func selectedNode(packet scene.StoryPacket, nodeID string) (scene.SceneNode, error) {
	if nodeID == "" {
		return packet.EntryNode()
	}
	return packet.Node(nodeID)
}

func buildCommit(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision, draft Draft, baseObserverPeerIDs []string) scene.Commit {
	witnesses := newSet()
	witnesses.add("sans-nom")
	for _, actor := range packet.ContextSnapshot.PresentActors {
		witnesses.add(actor)
	}
	observers := newSet()
	observers.add("sans-nom")
	observers.add("narrator")
	for _, peerID := range baseObserverPeerIDs {
		observers.add(peerID)
	}

	events := []scene.CanonEvent{{
		PeerID:    "sans-nom",
		Content:   decisionText(decision),
		Source:    scene.CommitSourcePlayer,
		Witnesses: witnesses.values(),
	}, {
		PeerID:    "narrator",
		Content:   draft.Prose,
		Source:    scene.CommitSourceNarrator,
		Witnesses: witnesses.values(),
	}}

	var surfaced []scene.ActorMessage
	for _, msg := range draft.SurfacedActorMessages {
		if msg.Surface == "" {
			continue
		}
		if len(msg.Witnesses) == 0 {
			msg.Witnesses = witnesses.values()
		}
		msg.Interior = ""
		surfaced = append(surfaced, msg)
		observers.add(msg.ActorID)
		events = append(events, scene.CanonEvent{
			PeerID:    msg.ActorID,
			Content:   msg.Surface,
			Source:    scene.CommitSourceActorSurface,
			Witnesses: append([]string(nil), msg.Witnesses...),
		})
		for _, witness := range msg.Witnesses {
			witnesses.add(witness)
		}
	}

	for _, intent := range node.CanonMemoryIntents() {
		events = append(events, scene.CanonEvent{
			PeerID:    "narrator",
			Content:   intent.Summary,
			Source:    scene.CommitSourceNarrator,
			Witnesses: append([]string(nil), intent.Witnesses...),
		})
		for _, witness := range intent.Witnesses {
			witnesses.add(witness)
		}
	}

	return scene.Commit{
		WorkspaceID:     packet.ContextSnapshot.WorkspaceID,
		SessionID:       scene.SceneSessionID(packet.ContextSnapshot.SessionID, packet.ID, node.ID),
		PacketID:        packet.ID,
		NodeID:          node.ID,
		PlayerDecision:  decision,
		FinalProse:      draft.Prose,
		ActorMessages:   surfaced,
		Events:          events,
		Witnesses:       witnesses.values(),
		ObserverPeerIDs: observers.values(),
	}
}

func observerPeerIDs(packet scene.StoryPacket, node scene.SceneNode, actorRaw []actors.Output, service *actors.Service) []string {
	ids := newSet()
	if service != nil && service.Registry != nil {
		for _, actorID := range packet.ContextSnapshot.PresentActors {
			if _, ok := service.Registry.Actor(actorID); ok {
				ids.add(actorID)
			}
		}
	}
	for _, need := range node.ActorNeeds {
		ids.add(need.ActorID)
	}
	for _, output := range actorRaw {
		ids.add(output.ActorID)
	}
	return ids.values()
}

func commitDecision(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision) scene.PlayerDecision {
	if strings.TrimSpace(decision.ChoiceID) == "" {
		return decision
	}
	entry, err := packet.EntryNode()
	if err != nil || entry.ID == node.ID || len(entry.Choices) < 2 {
		return decision
	}
	target := strings.TrimSpace(entry.Choices[0].TargetNodeID)
	if target == "" || target != node.ID {
		return decision
	}
	found := false
	for _, choice := range entry.Choices {
		if strings.TrimSpace(choice.TargetNodeID) != target {
			return decision
		}
		if choice.ID == decision.ChoiceID {
			found = true
		}
	}
	if found {
		return scene.PlayerDecision{}
	}
	return decision
}

func decisionText(decision scene.PlayerDecision) string {
	if decision.FreeText != "" {
		return "Player free-text: " + decision.FreeText
	}
	if decision.ChoiceID != "" {
		return "Player selected choice: " + decision.ChoiceID
	}
	return "Player observed."
}

func logEvent(logger *gamelog.Writer, event gamelog.Event) {
	if logger == nil {
		return
	}
	_ = logger.Append(event)
}

func rendererModelMetadata(renderer Renderer) modelmeta.Metadata {
	if renderer == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := renderer.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

func actorInvokerModelMetadata(invoker actors.Invoker) modelmeta.Metadata {
	if invoker == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := invoker.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

type StaticRenderer struct{}

func (StaticRenderer) ModelMetadata() modelmeta.Metadata {
	return modelmeta.Metadata{
		ProviderType:  "static",
		ModelProvider: "static",
		Model:         "static",
	}
}

func (StaticRenderer) Render(_ context.Context, node scene.SceneNode, rehearsal RehearsalResult, _ string) (Draft, error) {
	var prose strings.Builder
	for _, beat := range node.Beats {
		prose.WriteString(beat.Text)
		prose.WriteString("\n\n")
	}
	var surfaced []scene.ActorMessage
	for _, output := range rehearsal.ActorOutputs {
		surface := strings.TrimSpace(strings.Join([]string{output.Dialogue, output.Action}, " "))
		if surface == "" {
			continue
		}
		surfaced = append(surfaced, scene.ActorMessage{
			ActorID:   output.ActorID,
			Surface:   surface,
			Witnesses: []string{"sans-nom", output.ActorID},
		})
		prose.WriteString(surface)
		prose.WriteString("\n\n")
	}
	return Draft{
		Prose:                 strings.TrimSpace(prose.String()),
		SurfacedActorMessages: surfaced,
		Choices:               node.Choices,
	}, nil
}

type ClaudeRenderer struct {
	Runner prompt.Runner
}

func (r ClaudeRenderer) ModelMetadata() modelmeta.Metadata {
	if r.Runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := r.Runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

func (r ClaudeRenderer) Render(ctx context.Context, node scene.SceneNode, rehearsal RehearsalResult, context string) (Draft, error) {
	if r.Runner == nil {
		return Draft{}, errors.New("claude renderer needs runner")
	}
	rawActors, _ := json.Marshal(rehearsal)
	text := prompt.NarratorPrompt(node, string(rawActors), context)
	raw, err := r.Runner.Run(ctx, text)
	if err != nil {
		return Draft{}, err
	}
	var draft Draft
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		return Draft{}, fmt.Errorf("parse narrator output: %w", err)
	}
	if strings.TrimSpace(draft.Prose) == "" {
		return Draft{}, errors.New("narrator returned empty prose")
	}
	return draft, nil
}

type stringSet map[string]struct{}

func newSet() stringSet {
	return stringSet{}
}

func (s stringSet) add(v string) {
	v = strings.TrimSpace(v)
	if v != "" {
		s[v] = struct{}{}
	}
}

func (s stringSet) values() []string {
	values := make([]string, 0, len(s))
	for v := range s {
		values = append(values, v)
	}
	return values
}
