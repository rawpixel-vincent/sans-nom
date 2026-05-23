package director

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestRehearseInvokesActorsWithArtifactsAndLogs(t *testing.T) {
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	reg, err := actors.NewRegistry([]actors.Actor{{ID: "tonto", DisplayName: "Tonto", Card: "card"}})
	if err != nil {
		t.Fatal(err)
	}
	artifactDir := t.TempDir()
	packet := scene.StoryPacket{
		ID: "packet-1",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "ws",
			SessionID:     "session",
			PresentActors: []string{"tonto"},
		},
		Nodes: []scene.SceneNode{{
			ID:    "selected",
			Beats: []scene.Beat{{Text: "Tonto watches."}},
			ActorNeeds: []scene.ActorNeed{{
				ActorID: "tonto",
				Mode:    scene.ActorModePresent,
				Reason:  "Tonto must decide what the record preserves.",
			}},
		}},
	}
	node := packet.Nodes[0]
	svc := Service{
		Actors: &actors.Service{
			Registry: reg,
			Invoker: actors.StaticInvoker{Outputs: map[string]actors.Output{
				"tonto": {Dialogue: "I record what survives.", Action: "Tonto angles his lens toward the scar."},
			}},
			Logger: logger,
		},
		Logger:           logger,
		Turn:             11,
		RunID:            "turn-000011",
		ActorContextText: "actor-specific context",
		ArtifactDir:      artifactDir,
	}

	rehearsal, err := svc.Rehearse(context.Background(), packet, node, "narrator context")
	if err != nil {
		t.Fatal(err)
	}
	if len(rehearsal.ActorOutputs) != 1 || rehearsal.ActorOutputs[0].ActorID != "tonto" {
		t.Fatalf("unexpected rehearsal outputs: %#v", rehearsal.ActorOutputs)
	}
	if !containsString(rehearsal.ObserverPeerIDs, "tonto") {
		t.Fatalf("registered present actor missing from observer peers: %#v", rehearsal.ObserverPeerIDs)
	}
	for _, name := range []string{"selected-node.json", "actor-outputs.json", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(artifactDir, name)); err != nil {
			t.Fatalf("missing rehearsal artifact %s: %v", name, err)
		}
	}
	for _, eventName := range []string{"director.rehearsal.started", "director.rehearsal.actor.request", "actor.invoke.request", "actor.invoke.completed", "director.rehearsal.actor.completed", "director.rehearsal.completed"} {
		events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: eventName})
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			t.Fatalf("missing %s log event", eventName)
		}
		if events[0].Turn != 11 || events[0].PacketID != "packet-1" || events[0].NodeID != "selected" || events[0].RunID != "turn-000011" {
			t.Fatalf("unexpected %s metadata: %#v", eventName, events[0])
		}
	}
}

func TestRehearseRejectsCouncilFanout(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "packet-1",
		Nodes: []scene.SceneNode{{
			ID:    "bad",
			Beats: []scene.Beat{{Text: "A forbidden roundtable."}},
			ActorNeeds: []scene.ActorNeed{
				{ActorID: "aghora", Mode: scene.ActorModePresent},
				{ActorID: "tonto", Mode: scene.ActorModePresent},
				{ActorID: "othon", Mode: scene.ActorModeRemembered},
				{ActorID: "honorata", Mode: scene.ActorModeRemembered},
				{ActorID: "oda", Mode: scene.ActorModeRemembered},
			},
		}},
	}
	reg, _ := actors.NewRegistry([]actors.Actor{
		{ID: "aghora"}, {ID: "tonto"}, {ID: "othon"}, {ID: "honorata"}, {ID: "oda"},
	})
	svc := Service{
		Actors: &actors.Service{Registry: reg, Invoker: actors.StaticInvoker{}},
	}
	_, err := svc.Rehearse(context.Background(), packet, packet.Nodes[0], "")
	if err == nil {
		t.Fatal("expected fanout cap error")
	}
	if !strings.Contains(err.Error(), "phase 0 cap") {
		t.Fatalf("unexpected fanout error: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
