package actors

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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

func TestServiceSkipsMissingActorsWhenConfigured(t *testing.T) {
	reg, err := NewRegistry([]Actor{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc := Service{
		Registry:          reg,
		Invoker:           StaticInvoker{},
		Logger:            gamelog.NewWriter(dir, "story-1"),
		PacketID:          "packet-1",
		Turn:              4,
		SkipMissingActors: true,
	}
	node := scene.SceneNode{
		ID:    "start",
		Beats: []scene.Beat{{Text: "A beat."}},
		ActorNeeds: []scene.ActorNeed{{
			ActorID: "missing",
			Mode:    scene.ActorModePresent,
			Reason:  "writer requested an actor before cards exist",
		}},
	}
	outputs, err := svc.InvokeForNode(context.Background(), node, "actor context")
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 0 {
		t.Fatalf("outputs = %#v", outputs)
	}
	events, err := gamelog.Read(filepath.Join(dir, "logs", "events.jsonl"), gamelog.Filter{Category: "actor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "actor.invoke.skipped" || events[0].Fields["actor_id"] != "missing" {
		t.Fatalf("unexpected actor skip logs: %#v", events)
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
		PacketID: "packet-1",
		Turn:     3,
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
	if events[0].PacketID != "packet-1" || events[0].Turn != 3 {
		t.Fatalf("actor request missed packet/turn metadata: %#v", events[0])
	}
	detail, err := svc.Logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(detail)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	promptText, _ := decoded["prompt"].(string)
	if !strings.Contains(promptText, "You are acting as tonto") || !strings.Contains(promptText, "actor context") {
		t.Fatalf("actor prompt not logged: %#v", decoded)
	}
}
