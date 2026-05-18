package actors

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestNeedsForNodeUsesOnlyExplicitNeeds(t *testing.T) {
	node := scene.SceneNode{
		ID:    "start",
		Beats: []scene.Beat{{Text: "A beat."}},
		ActorNeeds: []scene.ActorNeed{{
			ActorID: "tonto",
			Mode:    scene.ActorModePresent,
		}},
	}

	needs, err := NeedsForNode(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 1 || needs[0].ActorID != "tonto" {
		t.Fatalf("unexpected needs: %#v", needs)
	}
}

func TestNeedsForNodeRejectsCouncilFanout(t *testing.T) {
	node := scene.SceneNode{
		ID:    "council",
		Beats: []scene.Beat{{Text: "A beat."}},
		ActorNeeds: []scene.ActorNeed{
			{ActorID: "aghora", Mode: scene.ActorModePresent},
			{ActorID: "tonto", Mode: scene.ActorModePresent},
			{ActorID: "othon", Mode: scene.ActorModeRemembered},
			{ActorID: "honorata", Mode: scene.ActorModeRemembered},
			{ActorID: "oda", Mode: scene.ActorModeRemembered},
		},
	}

	if _, err := NeedsForNode(node); err == nil {
		t.Fatal("expected actor cap error")
	}
}

func TestServiceInvokesExplicitActors(t *testing.T) {
	reg, err := NewRegistry([]Actor{{ID: "tonto", DisplayName: "Tonto", Card: "TRAIT: long stories"}})
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{
		Registry: reg,
		Invoker: StaticInvoker{Outputs: map[string]Output{
			"tonto": {Dialogue: "The dynasty remembers by deforming memory."},
		}},
	}
	node := scene.SceneNode{
		ID:    "start",
		Beats: []scene.Beat{{Text: "Sans-Nom asks Tonto."}},
		ActorNeeds: []scene.ActorNeed{{
			ActorID: "tonto",
			Mode:    scene.ActorModePresent,
		}},
	}

	outputs, err := svc.InvokeForNode(context.Background(), node, "context")
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].ActorID != "tonto" {
		t.Fatalf("unexpected outputs: %#v", outputs)
	}
}

func TestServiceLogsActorInvocation(t *testing.T) {
	reg, err := NewRegistry([]Actor{{ID: "tonto", DisplayName: "Tonto", Card: "TRAIT: witness"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc := Service{
		Registry: reg,
		Invoker:  StaticInvoker{},
		Logger:   gamelog.NewWriter(dir, "story-1"),
	}
	node := scene.SceneNode{
		ID:    "start",
		Beats: []scene.Beat{{Text: "A beat."}},
		ActorNeeds: []scene.ActorNeed{{
			ActorID: "tonto",
			Mode:    scene.ActorModePresent,
			Reason:  "needed here",
		}},
	}
	if _, err := svc.InvokeForNode(context.Background(), node, "actor context"); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(filepath.Join(dir, "logs", "events.jsonl"), gamelog.Filter{Category: "actor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Event != "actor.invoke.request" || events[1].Event != "actor.invoke.completed" {
		t.Fatalf("unexpected actor logs: %#v", events)
	}
}
