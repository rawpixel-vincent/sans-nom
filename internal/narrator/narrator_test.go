package narrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestRolloutCommitsOnlySelectedNode(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "packet-1",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "ws",
			SessionID:     "session",
			PresentActors: []string{"tonto"},
		},
		Nodes: []scene.SceneNode{{
			ID:    "selected",
			Beats: []scene.Beat{{Text: "Selected branch."}},
			MemoryIntents: []scene.MemoryIntent{{
				Summary:   "Selected branch happened.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom", "tonto"},
			}},
		}, {
			ID:    "unused",
			Beats: []scene.Beat{{Text: "Unused branch."}},
			MemoryIntents: []scene.MemoryIntent{{
				Summary:   "Unused branch should never commit.",
				Policy:    scene.MemoryPolicyPlanning,
				Witnesses: []string{"sans-nom"},
			}},
		}},
	}
	svc := Service{Renderer: StaticRenderer{}}

	rollout, err := svc.Rollout(context.Background(), packet, "selected", scene.PlayerDecision{ChoiceID: "go"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range rollout.Commit.Events {
		if strings.Contains(event.Content, "Unused") {
			t.Fatalf("unused branch leaked into commit: %#v", event)
		}
	}
}

func TestRolloutDoesNotCommitActorInterior(t *testing.T) {
	reg, err := actors.NewRegistry([]actors.Actor{{ID: "tonto", Card: "card"}})
	if err != nil {
		t.Fatal(err)
	}
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
			}},
		}},
	}
	svc := Service{
		Actors: &actors.Service{
			Registry: reg,
			Invoker: actors.StaticInvoker{Outputs: map[string]actors.Output{
				"tonto": {
					Interior: "This private thought must not be committed.",
					Dialogue: "I will tell you only what you can bear.",
				},
			}},
		},
		Renderer: StaticRenderer{},
	}

	rollout, err := svc.Rollout(context.Background(), packet, "selected", scene.PlayerDecision{ChoiceID: "listen"}, "")
	if err != nil {
		t.Fatal(err)
	}
	encoded := commitText(rollout.Commit)
	if strings.Contains(encoded, "private thought") {
		t.Fatalf("actor interior leaked into commit: %s", encoded)
	}
}

func TestRolloutLogsNarratorPromptAndResponseInputs(t *testing.T) {
	logger := gamelog.NewWriter(t.TempDir(), "story-1")
	packet := scene.StoryPacket{
		ID: "packet-1",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID: "ws",
			SessionID:   "session",
		},
		Nodes: []scene.SceneNode{{
			ID:      "selected",
			Beats:   []scene.Beat{{Text: "Selected branch."}},
			Choices: []scene.Choice{{ID: "choice-a", Label: "Go", TargetNodeID: "selected"}},
		}},
	}
	svc := Service{Renderer: StaticRenderer{}, Logger: logger, Turn: 7}
	if _, err := svc.Rollout(context.Background(), packet, "selected", scene.PlayerDecision{ChoiceID: "choice-a"}, "narrator context"); err != nil {
		t.Fatal(err)
	}
	events, err := gamelog.Read(logger.Path, gamelog.Filter{Event: "narrator.rollout.request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one narrator request log, got %#v", events)
	}
	if events[0].Turn != 7 || events[0].PacketID != "packet-1" || events[0].NodeID != "selected" {
		t.Fatalf("unexpected request event metadata: %#v", events[0])
	}
	var detail map[string]any
	raw, err := logger.ReadDetail(events[0].DetailPath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(raw)
	if err := json.Unmarshal(encoded, &detail); err != nil {
		t.Fatal(err)
	}
	promptText, _ := detail["prompt"].(string)
	if !strings.Contains(promptText, "You are the Narrator for Sans-Nom") || !strings.Contains(promptText, "narrator context") {
		t.Fatalf("narrator prompt detail missing expected text: %#v", detail)
	}
}

func TestRolloutRejectsCouncilFanout(t *testing.T) {
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
		Actors:   &actors.Service{Registry: reg, Invoker: actors.StaticInvoker{}},
		Renderer: StaticRenderer{},
	}
	_, err := svc.Rollout(context.Background(), packet, "bad", scene.PlayerDecision{}, "")
	if err == nil {
		t.Fatal("expected fanout cap error")
	}
}

func commitText(commit scene.Commit) string {
	var b strings.Builder
	b.WriteString(commit.FinalProse)
	for _, event := range commit.Events {
		b.WriteString(event.Content)
	}
	for _, msg := range commit.ActorMessages {
		b.WriteString(msg.Surface)
		b.WriteString(msg.Interior)
	}
	return b.String()
}
