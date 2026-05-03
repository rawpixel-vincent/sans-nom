package narrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
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
	Actors   *actors.Service
	Renderer Renderer
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

	var actorRaw []actors.Output
	if s.Actors != nil {
		actorRaw, err = s.Actors.InvokeForNode(ctx, node, context)
		if err != nil {
			return Rollout{}, err
		}
	}
	draft, err := s.Renderer.Render(ctx, node, actorRaw, context)
	if err != nil {
		return Rollout{}, err
	}
	commit := buildCommit(packet, node, decision, draft)
	if err := commit.ValidateCanonOnly(); err != nil {
		return Rollout{}, err
	}
	return Rollout{Node: node, ActorRaw: actorRaw, Draft: draft, Commit: commit}, nil
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
		SessionID:      packet.ContextSnapshot.SessionID,
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

type StaticRenderer struct{}

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
