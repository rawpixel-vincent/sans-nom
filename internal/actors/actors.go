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

type Invoker interface {
	Invoke(ctx context.Context, actor Actor, node scene.SceneNode, need scene.ActorNeed, context string) (Output, error)
}

type Registry struct {
	actors map[string]Actor
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
	Registry *Registry
	Invoker  Invoker
	Logger   *gamelog.Writer
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
			return nil, fmt.Errorf("actor %q not found", need.ActorID)
		}
		wg.Add(1)
		go func(idx int, actor Actor, need scene.ActorNeed) {
			defer wg.Done()
			s.logActorEvent(gamelog.Event{
				Category: "actor",
				Event:    "actor.invoke.request",
				Message:  "actor invocation requested",
				NodeID:   node.ID,
				Fields: map[string]any{
					"actor_id": actor.ID,
					"mode":     need.Mode,
					"reason":   need.Reason,
				},
				Detail: map[string]any{
					"actor_id":     actor.ID,
					"display_name": actor.DisplayName,
					"node_id":      node.ID,
					"node_title":   node.Title,
					"need":         need,
					"context_text": context,
				},
			})
			output, err := s.Invoker.Invoke(ctx, actor, node, need, context)
			if err != nil {
				s.logActorEvent(gamelog.Event{
					Level:    gamelog.LevelError,
					Category: "actor",
					Event:    "actor.invoke.failed",
					Message:  err.Error(),
					NodeID:   node.ID,
					Fields: map[string]any{
						"actor_id": actor.ID,
						"mode":     need.Mode,
						"reason":   need.Reason,
					},
					Detail: map[string]any{
						"actor_id": actor.ID,
						"node_id":  node.ID,
						"need":     need,
						"error":    err.Error(),
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
				NodeID:   node.ID,
				Fields: map[string]any{
					"actor_id": actor.ID,
					"mode":     need.Mode,
					"reason":   need.Reason,
				},
				Detail: map[string]any{
					"actor_id": actor.ID,
					"node_id":  node.ID,
					"need":     need,
					"output":   output,
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
	return outputs, nil
}

func (s Service) logActorEvent(event gamelog.Event) {
	if s.Logger == nil {
		return
	}
	_ = s.Logger.Append(event)
}

type StaticInvoker struct {
	Outputs map[string]Output
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
