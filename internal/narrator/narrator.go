package narrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Renderer interface {
	Render(ctx context.Context, node scene.SceneNode, actorOutputs []actors.Output, context string) (Draft, error)
}

type Draft struct {
	Prose                 string               `json:"prose"`
	SurfacedActorMessages []scene.ActorMessage `json:"surfaced_actor_messages,omitempty"`
	Choices               []scene.Choice       `json:"choices,omitempty"`
}

type Service struct {
	Actors           *actors.Service
	Renderer         Renderer
	Logger           *gamelog.Writer
	Turn             int
	ActorContextText string
}

type Rollout struct {
	Node         scene.SceneNode
	ActorRaw     []actors.Output
	Draft        Draft
	Commit       scene.Commit
	NextChoiceID string
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

	var actorRaw []actors.Output
	if s.Actors != nil {
		actorService := *s.Actors
		actorService.PacketID = packet.ID
		actorService.Turn = s.Turn
		actorContext := context
		if strings.TrimSpace(s.ActorContextText) != "" {
			actorContext = s.ActorContextText
		}
		actorRaw, err = actorService.InvokeForNode(ctx, node, actorContext)
		if err != nil {
			return Rollout{}, err
		}
	}
	rawActors, _ := json.Marshal(actorRaw)
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
		}, metadata),
		Detail: map[string]any{
			"prompt":         promptText,
			"context_text":   context,
			"node":           node,
			"actor_outputs":  actorRaw,
			"decision":       decision,
			"model_metadata": metadata,
		},
	})
	draft, err := s.Renderer.Render(ctx, node, actorRaw, context)
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
				"actor_outputs":  actorRaw,
				"decision":       decision,
				"error":          err.Error(),
				"model_metadata": metadata,
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
	commit := buildCommit(packet, node, decision, draft)
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
				"actor_outputs":  actorRaw,
				"draft":          draft,
				"decision":       decision,
				"error":          err.Error(),
				"model_metadata": metadata,
			},
		})
		return Rollout{}, err
	}
	return Rollout{Node: node, ActorRaw: actorRaw, Draft: draft, Commit: commit}, nil
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

func buildCommit(packet scene.StoryPacket, node scene.SceneNode, decision scene.PlayerDecision, draft Draft) scene.Commit {
	witnesses := newSet()
	witnesses.add("sans-nom")
	for _, actor := range packet.ContextSnapshot.PresentActors {
		witnesses.add(actor)
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
		WorkspaceID:    packet.ContextSnapshot.WorkspaceID,
		SessionID:      scene.SceneSessionID(packet.ContextSnapshot.SessionID, packet.ID, node.ID),
		PacketID:       packet.ID,
		NodeID:         node.ID,
		PlayerDecision: decision,
		FinalProse:     draft.Prose,
		ActorMessages:  surfaced,
		Events:         events,
		Witnesses:      witnesses.values(),
	}
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

type StaticRenderer struct{}

func (StaticRenderer) ModelMetadata() modelmeta.Metadata {
	return modelmeta.Metadata{
		ProviderType:  "static",
		ModelProvider: "static",
		Model:         "static",
	}
}

func (StaticRenderer) Render(_ context.Context, node scene.SceneNode, actorOutputs []actors.Output, _ string) (Draft, error) {
	var prose strings.Builder
	for _, beat := range node.Beats {
		prose.WriteString(beat.Text)
		prose.WriteString("\n\n")
	}
	var surfaced []scene.ActorMessage
	for _, output := range actorOutputs {
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

func (r ClaudeRenderer) Render(ctx context.Context, node scene.SceneNode, actorOutputs []actors.Output, context string) (Draft, error) {
	if r.Runner == nil {
		return Draft{}, errors.New("claude renderer needs runner")
	}
	rawActors, _ := json.Marshal(actorOutputs)
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
