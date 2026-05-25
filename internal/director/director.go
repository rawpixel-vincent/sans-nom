package director

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

const NonCanonScope = "Director rehearsal is backstage planning/performance material. Only Narrator surfaced_actor_messages and curated_card_updates can become shared actor memory."

type Service struct {
	Actors           *actors.Service
	Logger           *gamelog.Writer
	Turn             int
	RunID            string
	ActorContextText string
	ArtifactDir      string
}

type RehearsalResult struct {
	PacketID        string             `json:"packet_id"`
	NodeID          string             `json:"node_id"`
	RunID           string             `json:"run_id,omitempty"`
	Turn            int                `json:"turn,omitempty"`
	ActorNeeds      []scene.ActorNeed  `json:"actor_needs,omitempty"`
	ActorOutputs    []actors.Output    `json:"actor_outputs"`
	ObserverPeerIDs []string           `json:"observer_peer_ids,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
	ArtifactDir     string             `json:"artifact_dir,omitempty"`
	ArtifactPaths   []string           `json:"artifact_paths,omitempty"`
	ModelMetadata   modelmeta.Metadata `json:"model_metadata,omitempty"`
	NonCanonScope   string             `json:"non_canon_scope"`
}

func EmptyResult(packet scene.StoryPacket, node scene.SceneNode, runID string, turn int) RehearsalResult {
	return RehearsalResult{
		PacketID:        packet.ID,
		NodeID:          node.ID,
		RunID:           runID,
		Turn:            turn,
		ActorNeeds:      append([]scene.ActorNeed(nil), node.ActorNeeds...),
		ActorOutputs:    []actors.Output{},
		ObserverPeerIDs: observerPeerIDs(packet, node, nil, nil),
		NonCanonScope:   NonCanonScope,
	}
}

func (s Service) Rehearse(ctx context.Context, packet scene.StoryPacket, node scene.SceneNode, narratorContext string) (RehearsalResult, error) {
	result := EmptyResult(packet, node, s.RunID, s.Turn)
	result.ArtifactDir = filepath.ToSlash(s.ArtifactDir)
	if s.Actors == nil || len(node.ActorNeeds) == 0 {
		return result, nil
	}

	actorService := *s.Actors
	actorService.PacketID = packet.ID
	actorService.Turn = s.Turn
	actorService.RunID = s.RunID
	actorContext := narratorContext
	if strings.TrimSpace(s.ActorContextText) != "" {
		actorContext = s.ActorContextText
	}
	metadata := actorInvokerModelMetadata(actorService.Invoker)
	result.ModelMetadata = metadata
	result.ObserverPeerIDs = observerPeerIDs(packet, node, nil, actorService.Registry)

	logEvent(s.Logger, gamelog.Event{
		Category: "director",
		Event:    "director.rehearsal.started",
		Message:  "Director rehearsal started",
		Turn:     s.Turn,
		PacketID: packet.ID,
		NodeID:   node.ID,
		RunID:    s.RunID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_need_count": len(node.ActorNeeds),
			"artifact_dir":     filepath.ToSlash(s.ArtifactDir),
		}, metadata),
		Detail: map[string]any{
			"node":            node,
			"actor_needs":     node.ActorNeeds,
			"actor_context":   actorContext,
			"non_canon_scope": result.NonCanonScope,
			"artifact_dir":    filepath.ToSlash(s.ArtifactDir),
			"model_metadata":  metadata,
		},
	})
	for _, need := range node.ActorNeeds {
		logEvent(s.Logger, gamelog.Event{
			Category: "director",
			Event:    "director.rehearsal.actor.request",
			Message:  "Director requested actor rehearsal",
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
			Category: "director",
			Event:    "director.rehearsal.failed",
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
	result.ObserverPeerIDs = observerPeerIDs(packet, node, outputs, actorService.Registry)
	for _, output := range outputs {
		logEvent(s.Logger, gamelog.Event{
			Category: "director",
			Event:    "director.rehearsal.actor.completed",
			Message:  "Actor rehearsal completed",
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
	if err := s.writeArtifacts(&result, node); err != nil {
		result.Warnings = append(result.Warnings, "write director rehearsal artifacts: "+err.Error())
	}
	logEvent(s.Logger, gamelog.Event{
		Category: "director",
		Event:    "director.rehearsal.completed",
		Message:  "Director rehearsal completed",
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

func (s Service) writeArtifacts(result *RehearsalResult, node scene.SceneNode) error {
	if strings.TrimSpace(s.ArtifactDir) == "" {
		return nil
	}
	if err := os.MkdirAll(s.ArtifactDir, 0o755); err != nil {
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
		path := filepath.Join(s.ArtifactDir, file.name)
		if err := writeJSONFile(path, file.value); err != nil {
			return err
		}
		result.ArtifactPaths = append(result.ArtifactPaths, filepath.ToSlash(path))
	}
	result.ArtifactDir = filepath.ToSlash(s.ArtifactDir)
	return nil
}

func observerPeerIDs(packet scene.StoryPacket, node scene.SceneNode, actorOutputs []actors.Output, registry *actors.Registry) []string {
	ids := newSet()
	if registry != nil {
		for _, actorID := range packet.ContextSnapshot.PresentActors {
			if _, ok := registry.Actor(actorID); ok {
				ids.add(actorID)
			}
		}
	}
	for _, need := range node.ActorNeeds {
		ids.add(need.ActorID)
	}
	for _, output := range actorOutputs {
		ids.add(output.ActorID)
	}
	return ids.values()
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

func logEvent(logger *gamelog.Writer, event gamelog.Event) {
	if logger == nil {
		return
	}
	_ = logger.Append(event)
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

func FilterUnsupportedActorNeeds(node scene.SceneNode, service *actors.Service) (scene.SceneNode, []actors.UnsupportedActorNeed) {
	if service == nil || service.Registry == nil {
		return node, nil
	}
	return actors.FilterUnsupportedNeeds(node, service.Registry)
}
