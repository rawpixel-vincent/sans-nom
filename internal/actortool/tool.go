package actortool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/agentfs"
	"github.com/rawpixel-vincent/sans-nom/internal/codexoauth"
	"github.com/rawpixel-vincent/sans-nom/internal/contextbundle"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

type ToolRunner struct {
	Registry             *actors.Registry
	Invoker              actors.Invoker
	Assembler            contextbundle.Assembler
	Packet               scene.StoryPacket
	NodeID               string
	Decision             scene.PlayerDecision
	Bible                *story.StoryBible
	ScenarioContractText string
	OutputDir            string
	WorkspaceRoot        string
	VisiblePaths         []string
	Logger               *gamelog.Writer
	CallerRole           string
	CallerName           string
	RunID                string
	Turn                 int
	MaxCalls             int

	mu     sync.Mutex
	nextID int
}

func (r *ToolRunner) LocalTool() codexoauth.LocalTool {
	return codexoauth.LocalTool{
		Name:        "ask_actor",
		Description: "Ask one registered story actor or explicit scene-local actor for raw planning/performance material. Registered actors use Honcho/local cards and representations; scene-local actors use the current scene setup, question, and node beats as a temporary card. Use only when character will, memory, voice, or relationship pressure affects packet design. Returned material is planning-only until Narrator later surfaces it.",
		Parameters:  actorToolSchema(),
		Handler:     r.Handle,
	}
}

func (r *ToolRunner) Handle(ctx context.Context, root string, arguments string) (any, error) {
	if r == nil || r.Registry == nil || r.Invoker == nil {
		return nil, errors.New("ask_actor requires registry and invoker")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		return nil, errors.New("ask_actor output dir is required")
	}
	var request actors.ActorCallRequest
	dec := json.NewDecoder(strings.NewReader(arguments))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		r.logRejected(arguments, err)
		return nil, err
	}
	if err := validateRequest(request); err != nil {
		r.logRejected(arguments, err)
		return nil, err
	}
	localActor, registeredActor := r.Registry.Actor(request.ActorID)
	callID, callIndex, err := r.nextCallID()
	if err != nil {
		r.logRejected(arguments, err)
		return nil, err
	}
	callDir := filepath.Join(r.OutputDir, callID)
	if err := os.MkdirAll(callDir, 0o755); err != nil {
		return nil, err
	}
	node, err := r.actorNode(request)
	if err != nil {
		r.logFailed(request, callDir, callIndex, err)
		return nil, err
	}
	need := scene.ActorNeed{
		ActorID: request.ActorID,
		Mode:    request.Mode,
		Reason:  actorReason(request),
		Targets: cleanStrings(request.Targets),
	}
	assembler := r.Assembler
	if !registeredActor {
		localActor = actors.SceneLocalActor(node, need)
		assembler.Cards = sceneLocalCardProvider{
			base: assembler.Cards,
			card: contextbundle.Card{
				ActorID:     localActor.ID,
				DisplayName: localActor.DisplayName,
				Text:        localActor.Card,
				Source:      "scene-local:" + localActor.ID,
			},
		}
		assembler.Options.DisableCardSeeding = true
	}
	actorContext, contextText, warnings, err := assembler.AssembleActorCall(ctx, contextbundle.ActorCallBuildRequest{
		Packet:               r.Packet,
		Node:                 node,
		Need:                 need,
		Decision:             r.Decision,
		Bible:                r.Bible,
		ScenarioContractText: r.ScenarioContractText,
	})
	if err != nil {
		r.logFailed(request, callDir, callIndex, err)
		return nil, err
	}
	metadata := invokerModelMetadata(r.Invoker)
	r.log(gamelog.Event{
		Category: "actor",
		Event:    "actor.tool.request",
		Message:  "Writer asked actor for planning material",
		RunID:    r.RunID,
		Turn:     r.Turn,
		PacketID: r.Packet.ID,
		NodeID:   node.ID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_id":      request.ActorID,
			"mode":          request.Mode,
			"call_dir":      filepath.ToSlash(callDir),
			"call_index":    callIndex,
			"card_source":   actorContext.CardSource,
			"actor_scope":   actorScope(registeredActor),
			"target_count":  len(need.Targets),
			"context_chars": len([]rune(contextText)),
		}, metadata),
		Detail: map[string]any{
			"arguments":      arguments,
			"request":        request,
			"node":           node,
			"need":           need,
			"actor_context":  actorContext,
			"actor_scope":    actorScope(registeredActor),
			"context_text":   contextText,
			"warnings":       warnings,
			"model_metadata": metadata,
			"planning_scope": "ask_actor output is planning-only until Narrator surfaces it",
			"artifact_dir":   filepath.ToSlash(callDir),
			"workspace_root": filepath.ToSlash(firstNonEmpty(r.WorkspaceRoot, root)),
			"visible_paths":  r.VisiblePaths,
			"caller_role":    r.CallerRole,
			"caller_speaker": r.CallerName,
		},
	})
	actor := localActor
	actor.Card = actorContext.Card
	output, err := r.Invoker.Invoke(ctx, actor, node, need, contextText)
	if err != nil {
		r.logFailed(request, callDir, callIndex, err)
		return nil, err
	}
	output.ActorID = actor.ID
	material := actors.ActorMaterial{
		ActorID:             actor.ID,
		Interior:            output.Interior,
		Dialogue:            output.Dialogue,
		Action:              output.Action,
		UsedCardSource:      actorContext.CardSource,
		UsedRepresentations: representationSummaries(actorContext.Representations),
	}
	material.ArtifactPath = modelVisiblePath(firstNonEmpty(r.WorkspaceRoot, root), filepath.Join(callDir, "material.json"))
	if err := writeJSONAtomic(filepath.Join(callDir, "request.json"), request); err != nil {
		return nil, err
	}
	if err := writeTextAtomic(filepath.Join(callDir, "context.md"), contextText+"\n"); err != nil {
		return nil, err
	}
	if err := writeJSONAtomic(filepath.Join(callDir, "material.json"), material); err != nil {
		return nil, err
	}
	if err := writeJSONAtomic(filepath.Join(callDir, "manifest.json"), map[string]any{
		"material": material,
		"warnings": warnings,
	}); err != nil {
		return nil, err
	}
	r.log(gamelog.Event{
		Category: "actor",
		Event:    "actor.tool.completed",
		Message:  "actor planning material written",
		RunID:    r.RunID,
		Turn:     r.Turn,
		PacketID: r.Packet.ID,
		NodeID:   node.ID,
		Fields: modelmeta.MergeFields(map[string]any{
			"actor_id":      actor.ID,
			"mode":          request.Mode,
			"call_dir":      filepath.ToSlash(callDir),
			"call_index":    callIndex,
			"actor_scope":   actorScope(registeredActor),
			"artifact_path": material.ArtifactPath,
		}, metadata),
		Detail: map[string]any{
			"request":        request,
			"material":       material,
			"output":         output,
			"actor_context":  actorContext,
			"warnings":       warnings,
			"model_metadata": metadata,
			"artifact_dir":   filepath.ToSlash(callDir),
		},
	})
	return map[string]any{
		"ok":       true,
		"material": material,
		"note":     "Actor material is planning-only. Use it to shape the StoryPacket; do not treat it as canon.",
	}, nil
}

type sceneLocalCardProvider struct {
	base contextbundle.CardProvider
	card contextbundle.Card
}

func (p sceneLocalCardProvider) LocalCard(actorID string) (contextbundle.Card, bool) {
	if strings.TrimSpace(actorID) == p.card.ActorID {
		return p.card, true
	}
	if p.base == nil {
		return contextbundle.Card{}, false
	}
	return p.base.LocalCard(actorID)
}

func (p sceneLocalCardProvider) ActorIDs() []string {
	seen := map[string]bool{}
	var ids []string
	if p.base != nil {
		for _, id := range p.base.ActorIDs() {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if id := strings.TrimSpace(p.card.ActorID); id != "" && !seen[id] {
		ids = append(ids, id)
	}
	return ids
}

func actorScope(registered bool) string {
	if registered {
		return "registered"
	}
	return "scene_local"
}

func (r *ToolRunner) actorNode(request actors.ActorCallRequest) (scene.SceneNode, error) {
	nodeID := firstNonEmpty(request.NodeID, r.NodeID)
	var node scene.SceneNode
	var err error
	if nodeID != "" {
		node, err = r.Packet.Node(nodeID)
	} else {
		node, err = r.Packet.EntryNode()
	}
	if err != nil {
		return scene.SceneNode{}, err
	}
	node.ActorNeeds = nil
	node.MemoryIntents = nil
	if setup := strings.TrimSpace(request.SceneSetup); setup != "" {
		node.Beats = append(node.Beats, scene.Beat{ID: "ask_actor_scene_setup", Text: setup})
	}
	if question := strings.TrimSpace(request.Question); question != "" {
		node.Beats = append(node.Beats, scene.Beat{ID: "ask_actor_question", Text: "Actor question: " + question})
	}
	return node, nil
}

func validateRequest(request actors.ActorCallRequest) error {
	if strings.TrimSpace(request.ActorID) == "" {
		return errors.New("actor_id is required")
	}
	switch request.Mode {
	case scene.ActorModePresent, scene.ActorModeRemembered, scene.ActorModeNarrated, scene.ActorModeContext:
	default:
		return fmt.Errorf("mode must be one of present, remembered, narrated, valid_scene_context")
	}
	if strings.TrimSpace(request.SceneSetup) == "" && strings.TrimSpace(request.Question) == "" {
		return errors.New("scene_setup or question is required")
	}
	return nil
}

func actorReason(request actors.ActorCallRequest) string {
	if question := strings.TrimSpace(request.Question); question != "" {
		return "Writer ask_actor: " + question
	}
	return "Writer ask_actor: " + strings.TrimSpace(request.SceneSetup)
}

func (r *ToolRunner) nextCallID() (string, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxCalls := r.MaxCalls
	if maxCalls <= 0 {
		maxCalls = actors.Phase0ActorCap
	}
	if r.nextID >= maxCalls {
		return "", r.nextID + 1, fmt.Errorf("ask_actor exceeded max calls %d", maxCalls)
	}
	r.nextID++
	return fmt.Sprintf("actor-call-%03d", r.nextID), r.nextID, nil
}

func (r *ToolRunner) logRejected(arguments string, err error) {
	r.log(gamelog.Event{
		Level:    gamelog.LevelWarn,
		Category: "actor",
		Event:    "actor.tool.rejected",
		Message:  err.Error(),
		RunID:    r.RunID,
		Turn:     r.Turn,
		PacketID: r.Packet.ID,
		NodeID:   r.NodeID,
		Detail: map[string]any{
			"arguments": arguments,
			"error":     err.Error(),
		},
	})
}

func (r *ToolRunner) logFailed(request actors.ActorCallRequest, callDir string, callIndex int, err error) {
	r.log(gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "actor",
		Event:    "actor.tool.failed",
		Message:  err.Error(),
		RunID:    r.RunID,
		Turn:     r.Turn,
		PacketID: r.Packet.ID,
		NodeID:   firstNonEmpty(request.NodeID, r.NodeID),
		Fields: map[string]any{
			"actor_id":   request.ActorID,
			"call_dir":   filepath.ToSlash(callDir),
			"call_index": callIndex,
		},
		Detail: map[string]any{
			"request": request,
			"error":   err.Error(),
		},
	})
}

func (r *ToolRunner) log(event gamelog.Event) {
	if r.Logger == nil {
		return
	}
	_ = r.Logger.Append(event)
}

func invokerModelMetadata(invoker actors.Invoker) modelmeta.Metadata {
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

func actorToolSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"actor_id", "mode", "scene_setup", "question", "targets", "node_id"},
		"properties": map[string]any{
			"actor_id": map[string]any{
				"type":        "string",
				"description": "Actor id from STORY ACTOR MANIFEST.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"present", "remembered", "narrated", "valid_scene_context"},
				"description": "How the actor is relevant to this planning question.",
			},
			"scene_setup": map[string]any{
				"type":        "string",
				"description": "Compact scene setup or pressure the actor is reacting to.",
			},
			"question": map[string]any{
				"type":        "string",
				"description": "One focused question for the actor's will, voice, memory, action, or relationship pressure.",
			},
			"targets": map[string]any{
				"type":        "array",
				"description": "Optional peer ids whose directional relationship to this actor matters.",
				"items":       map[string]any{"type": "string"},
			},
			"node_id": map[string]any{
				"type":        "string",
				"description": "Optional StoryPacket node id to ground the call. Use empty string for current selected node.",
			},
		},
	}
}

func representationSummaries(reps []contextbundle.Representation) []string {
	out := make([]string, 0, len(reps))
	for _, rep := range reps {
		text := strings.TrimSpace(rep.Text)
		if text == "" {
			continue
		}
		out = append(out, rep.Observer+" -> "+rep.Observed+": "+text)
	}
	return out
}

func cleanStrings(values []string) []string {
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

func modelVisiblePath(workspaceRoot string, path string) string {
	scope := agentfs.New(workspaceRoot, "actor")
	if rel, err := scope.Normalize(path); err == nil {
		return rel
	}
	return filepath.ToSlash(path)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func writeTextAtomic(path string, text string) error {
	return writeFileAtomic(path, []byte(text))
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
