package actors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

const Phase0ActorCap = 4

type Actor struct {
	ID          string
	DisplayName string
	Card        string
}

type Output struct {
	ActorID  string `json:"actor_id"`
	Interior string `json:"interior,omitempty"`
	Dialogue string `json:"dialogue,omitempty"`
	Action   string `json:"action,omitempty"`
}

type ActorCallRequest struct {
	ActorID    string   `json:"actor_id"`
	Mode       string   `json:"mode,omitempty"`
	SceneSetup string   `json:"scene_setup,omitempty"`
	Question   string   `json:"question,omitempty"`
	Targets    []string `json:"targets,omitempty"`
	NodeID     string   `json:"node_id,omitempty"`
}

type ActorMaterial struct {
	ActorID             string   `json:"actor_id"`
	Interior            string   `json:"interior,omitempty"`
	Dialogue            string   `json:"dialogue,omitempty"`
	Action              string   `json:"action,omitempty"`
	UsedCardSource      string   `json:"used_card_source,omitempty"`
	UsedRepresentations []string `json:"used_representations,omitempty"`
	ArtifactPath        string   `json:"artifact_path,omitempty"`
}

type Invoker interface {
	Invoke(ctx context.Context, actor Actor, node scene.SceneNode, need scene.ActorNeed, context string) (Output, error)
}

type Registry struct {
	actors map[string]Actor
}

type UnsupportedActorNeed struct {
	NodeID string          `json:"node_id"`
	Need   scene.ActorNeed `json:"need"`
}

func NewRegistry(actorList []Actor) (*Registry, error) {
	r := &Registry{actors: map[string]Actor{}}
	for _, actor := range actorList {
		if strings.TrimSpace(actor.ID) == "" {
			return nil, errors.New("actor id is required")
		}
		r.actors[actor.ID] = actor
	}
	return r, nil
}

func LoadCards(dir string) (*Registry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var loaded []Actor
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		loaded = append(loaded, Actor{
			ID:          id,
			DisplayName: id,
			Card:        string(data),
		})
	}
	return NewRegistry(loaded)
}

func (r *Registry) Actor(id string) (Actor, bool) {
	actor, ok := r.actors[id]
	return actor, ok
}

func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.actors))
	for id := range r.actors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func FilterUnsupportedNeeds(node scene.SceneNode, registry *Registry) (scene.SceneNode, []UnsupportedActorNeed) {
	if registry == nil || len(node.ActorNeeds) == 0 {
		return node, nil
	}
	filtered := node
	filtered.ActorNeeds = make([]scene.ActorNeed, 0, len(node.ActorNeeds))
	var removed []UnsupportedActorNeed
	for _, need := range node.ActorNeeds {
		if _, ok := registry.Actor(need.ActorID); ok {
			filtered.ActorNeeds = append(filtered.ActorNeeds, need)
			continue
		}
		removed = append(removed, UnsupportedActorNeed{NodeID: node.ID, Need: need})
	}
	if len(removed) == 0 {
		return node, nil
	}
	return filtered, removed
}

func PruneUnsupportedPacketNeeds(packet scene.StoryPacket, registry *Registry) (scene.StoryPacket, []UnsupportedActorNeed) {
	if registry == nil || len(packet.Nodes) == 0 {
		return packet, nil
	}
	pruned := packet
	pruned.Nodes = append([]scene.SceneNode(nil), packet.Nodes...)
	var removed []UnsupportedActorNeed
	for i, node := range pruned.Nodes {
		filtered, nodeRemoved := FilterUnsupportedNeeds(node, registry)
		pruned.Nodes[i] = filtered
		removed = append(removed, nodeRemoved...)
	}
	if len(removed) == 0 {
		return packet, nil
	}
	return pruned, removed
}

// NeedsForNode returns only the explicit actor needs encoded in the selected
// node. This is the guard against accidentally inheriting Council-style
// all-character fanout.
func NeedsForNode(node scene.SceneNode) ([]scene.ActorNeed, error) {
	if len(node.ActorNeeds) > Phase0ActorCap {
		return nil, fmt.Errorf("node %q requests %d actors; phase 0 cap is %d", node.ID, len(node.ActorNeeds), Phase0ActorCap)
	}
	out := make([]scene.ActorNeed, 0, len(node.ActorNeeds))
	for _, need := range node.ActorNeeds {
		switch need.Mode {
		case scene.ActorModePresent, scene.ActorModeRemembered, scene.ActorModeNarrated, scene.ActorModeContext:
			out = append(out, need)
		default:
			return nil, fmt.Errorf("actor %q has invalid mode %q", need.ActorID, need.Mode)
		}
	}
	return out, nil
}

type Service struct {
	Registry          *Registry
	Invoker           Invoker
	Logger            *gamelog.Writer
	PacketID          string
	Turn              int
	SkipMissingActors bool
}

func (s Service) InvokeForNode(ctx context.Context, node scene.SceneNode, context string) ([]Output, error) {
	if s.Registry == nil {
		return nil, errors.New("actor registry is required")
	}
	if s.Invoker == nil {
		return nil, errors.New("actor invoker is required")
	}
	needs, err := NeedsForNode(node)
	if err != nil {
		return nil, err
	}
	outputs := make([]Output, len(needs))
	errs := make(chan error, len(needs))
	var wg sync.WaitGroup
	for i, need := range needs {
		actor, ok := s.Registry.Actor(need.ActorID)
		if !ok {
			if s.SkipMissingActors {
				s.logActorEvent(gamelog.Event{
					Level:    gamelog.LevelWarn,
					Category: "actor",
					Event:    "actor.invoke.skipped",
					Message:  fmt.Sprintf("actor %q skipped: no local card", need.ActorID),
					Turn:     s.Turn,
					PacketID: s.PacketID,
					NodeID:   node.ID,
					Fields: map[string]any{
						"actor_id": need.ActorID,
						"mode":     need.Mode,
						"reason":   need.Reason,
					},
					Detail: map[string]any{
						"actor_id": need.ActorID,
						"node_id":  node.ID,
						"need":     need,
						"reason":   "missing local actor card",
					},
				})
				continue
			}
			return nil, fmt.Errorf("actor %q not found", need.ActorID)
		}
		wg.Add(1)
		go func(idx int, actor Actor, need scene.ActorNeed) {
			defer wg.Done()
			promptText := prompt.ActorPrompt(actor.Card, node, need, context)
			metadata := invokerModelMetadata(s.Invoker)
			s.logActorEvent(gamelog.Event{
				Category: "actor",
				Event:    "actor.invoke.request",
				Message:  "actor invocation requested",
				Turn:     s.Turn,
				PacketID: s.PacketID,
				NodeID:   node.ID,
				Fields: modelmeta.MergeFields(map[string]any{
					"actor_id": actor.ID,
					"mode":     need.Mode,
					"reason":   need.Reason,
				}, metadata),
				Detail: map[string]any{
					"actor_id":       actor.ID,
					"display_name":   actor.DisplayName,
					"node_id":        node.ID,
					"node_title":     node.Title,
					"need":           need,
					"prompt":         promptText,
					"context_text":   context,
					"model_metadata": metadata,
				},
			})
			output, err := s.Invoker.Invoke(ctx, actor, node, need, context)
			if err != nil {
				s.logActorEvent(gamelog.Event{
					Level:    gamelog.LevelError,
					Category: "actor",
					Event:    "actor.invoke.failed",
					Message:  err.Error(),
					Turn:     s.Turn,
					PacketID: s.PacketID,
					NodeID:   node.ID,
					Fields: modelmeta.MergeFields(map[string]any{
						"actor_id": actor.ID,
						"mode":     need.Mode,
						"reason":   need.Reason,
					}, metadata),
					Detail: map[string]any{
						"actor_id":       actor.ID,
						"node_id":        node.ID,
						"need":           need,
						"prompt":         promptText,
						"error":          err.Error(),
						"model_metadata": metadata,
					},
				})
				errs <- err
				return
			}
			output.ActorID = actor.ID
			outputs[idx] = output
			s.logActorEvent(gamelog.Event{
				Category: "actor",
				Event:    "actor.invoke.completed",
				Message:  "actor invocation completed",
				Turn:     s.Turn,
				PacketID: s.PacketID,
				NodeID:   node.ID,
				Fields: modelmeta.MergeFields(map[string]any{
					"actor_id": actor.ID,
					"mode":     need.Mode,
					"reason":   need.Reason,
				}, metadata),
				Detail: map[string]any{
					"actor_id":       actor.ID,
					"node_id":        node.ID,
					"need":           need,
					"prompt":         promptText,
					"output":         output,
					"model_metadata": metadata,
				},
			})
		}(i, actor, need)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return compactActorOutputs(outputs), nil
}

func compactActorOutputs(outputs []Output) []Output {
	out := outputs[:0]
	for _, output := range outputs {
		if strings.TrimSpace(output.ActorID) == "" {
			continue
		}
		out = append(out, output)
	}
	return out
}

func (s Service) logActorEvent(event gamelog.Event) {
	if s.Logger == nil {
		return
	}
	_ = s.Logger.Append(event)
}

func invokerModelMetadata(invoker Invoker) modelmeta.Metadata {
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

type StaticInvoker struct {
	Outputs map[string]Output
}

func (i StaticInvoker) ModelMetadata() modelmeta.Metadata {
	return prompt.StaticRunner{}.ModelMetadata()
}

func (i StaticInvoker) Invoke(_ context.Context, actor Actor, _ scene.SceneNode, _ scene.ActorNeed, _ string) (Output, error) {
	if i.Outputs != nil {
		if out, ok := i.Outputs[actor.ID]; ok {
			out.ActorID = actor.ID
			return out, nil
		}
	}
	return Output{ActorID: actor.ID, Action: actor.DisplayName + " watches."}, nil
}

type ClaudeInvoker struct {
	Runner prompt.Runner
}

func (i ClaudeInvoker) ModelMetadata() modelmeta.Metadata {
	if i.Runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := i.Runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

func (i ClaudeInvoker) Invoke(ctx context.Context, actor Actor, node scene.SceneNode, need scene.ActorNeed, context string) (Output, error) {
	if i.Runner == nil {
		return Output{}, errors.New("claude invoker needs runner")
	}
	text := prompt.ActorPrompt(actor.Card, node, need, context)
	raw, err := i.Runner.Run(ctx, text)
	if err != nil {
		return Output{}, err
	}
	var out Output
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Output{}, fmt.Errorf("parse actor output for %q: %w", actor.ID, err)
	}
	out.ActorID = actor.ID
	return out, nil
}
