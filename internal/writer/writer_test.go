package writer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestRefreshAsyncReturnsPacket(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "next",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Beats: []scene.Beat{{Text: "A beat."}},
		}},
	}
	svc := Service{Planner: StaticPlanner{Next: packet}}

	job, err := svc.RefreshAsync(context.Background(), Input{
		Current:        packet,
		LatestDecision: "wait",
		ContextText:    "writer context",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := job.Await(ctx)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Packet.ID != "next" {
		t.Fatalf("packet id = %q", result.Packet.ID)
	}
}

func TestRefreshAsyncPassesWriterContext(t *testing.T) {
	packet := scene.StoryPacket{
		ID: "current",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Beats: []scene.Beat{{Text: "A beat."}},
		}},
	}
	planner := &recordingPlanner{next: packet}
	svc := Service{Planner: planner}

	job, err := svc.RefreshAsync(context.Background(), Input{
		Current:        packet,
		LatestDecision: "ask-tonto",
		ContextText:    "assembled writer context",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if planner.input.ContextText != "assembled writer context" || planner.input.LatestDecision != "ask-tonto" {
		t.Fatalf("planner missed input: %#v", planner.input)
	}
}

func TestRefreshAsyncRejectsInvalidReplacement(t *testing.T) {
	svc := Service{Planner: invalidPlanner{}}

	job, err := svc.RefreshAsync(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err == nil {
		t.Fatal("expected invalid packet error")
	}
}

func TestRefreshAsyncRejectsSpentSituationChoiceLoop(t *testing.T) {
	packet := repeatedSituationPacket()
	svc := Service{Planner: StaticPlanner{Next: packet}, Validator: ValidateProgression}

	job, err := svc.RefreshAsync(context.Background(), Input{
		SpentMotifs: []string{"record loop", "repeated archive handling"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err == nil {
		t.Fatal("expected spent situation loop rejection")
	}
	if !strings.Contains(result.Err.Error(), "spent situation") {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

func TestRefreshAsyncAllowsSpentSituationWithConsequenceShift(t *testing.T) {
	packet := repeatedSituationPacket()
	packet.Nodes[0].Choices[2] = scene.Choice{
		ID:           "force_opponent",
		Label:        "Carry the record to the storm gate and force Orro into a public repair contest.",
		TargetNodeID: "storm_gate",
		PlayerIntent: "Claim player agency by moving the conflict from repeated records into opponent action and resource pressure.",
	}
	packet.Nodes[3] = scene.SceneNode{
		ID:      "storm_gate",
		Title:   "Storm Gate",
		Beats:   []scene.Beat{{Text: "The storm gate opens to reveal Orro, a failing resource line, and a crowd demanding action before the deck tears loose."}},
		Choices: []scene.Choice{},
	}
	svc := Service{Planner: StaticPlanner{Next: packet}, Validator: ValidateProgression}

	job, err := svc.RefreshAsync(context.Background(), Input{
		SpentMotifs: []string{"record loop", "repeated archive handling"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err != nil {
		t.Fatal(result.Err)
	}
}

func TestRefreshAsyncWritesDiagnosticsForInvalidWriterPacket(t *testing.T) {
	raw := `{
  "id": "bad",
  "entry_node_id": "start",
  "context_snapshot": {},
  "nodes": [{
    "id": "start",
    "title": "Start",
    "beats": [{"text": "A beat."}],
    "choices": [{"id": "jump", "label": "Jump.", "target_node_id": "missing"}]
  }]
}`
	dir := t.TempDir()
	svc := Service{Planner: ClaudePlanner{Runner: prompt.StaticRunner{Response: raw}}}

	job, err := svc.RefreshAsync(context.Background(), Input{
		LatestDecision: "jump",
		ContextText:    "writer context",
		RunID:          "turn-000001",
		ArtifactDir:    dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err == nil {
		t.Fatal("expected invalid packet error")
	}
	for _, name := range []string{"input.txt", "raw-output.json", "validation-error.txt", "metadata.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	validation, err := os.ReadFile(filepath.Join(dir, "validation-error.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(validation), "targets missing node") {
		t.Fatalf("unexpected validation artifact:\n%s", validation)
	}
}

func TestValidateProgressionRejectsRegisteredActorWithoutRehearsal(t *testing.T) {
	packet := actorRehearsalPacket()
	err := ValidateProgression(Input{RegisteredActorIDs: []string{"tonto"}}, packet)
	if err == nil {
		t.Fatal("expected missing actor rehearsal rejection")
	}
	if !strings.Contains(err.Error(), "needs Director rehearsal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProgressionMatchesRegisteredActorCaseInsensitively(t *testing.T) {
	packet := actorRehearsalPacket()
	packet.ContextSnapshot.PresentActors = []string{"Tonto"}
	err := ValidateProgression(Input{RegisteredActorIDs: []string{"tonto"}}, packet)
	if err == nil {
		t.Fatal("expected missing actor rehearsal rejection")
	}
	if !strings.Contains(err.Error(), "registered actor \"Tonto\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProgressionAcceptsRegisteredActorNeed(t *testing.T) {
	packet := actorRehearsalPacket()
	packet.Nodes[0].ActorNeeds = []scene.ActorNeed{{
		ActorID: "tonto",
		Mode:    scene.ActorModePresent,
		Reason:  "Tonto's character pressure should shape the playable beat.",
	}}
	if err := ValidateProgression(Input{RegisteredActorIDs: []string{"tonto"}}, packet); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProgressionAcceptsNoRegisteredActorCards(t *testing.T) {
	packet := actorRehearsalPacket()
	if err := ValidateProgression(Input{}, packet); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProgressionAcceptsNoActorRehearsalReason(t *testing.T) {
	packet := actorRehearsalPacket()
	packet.Nodes[0].WriterNotes = "No actor rehearsal: Tonto is only a distant recorder in this playable beat."
	if err := ValidateProgression(Input{RegisteredActorIDs: []string{"tonto"}}, packet); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProgressionRejectsGenericPlayerIntent(t *testing.T) {
	packet := actorRehearsalPacket()
	packet.Nodes[0].Choices[1].PlayerIntent = "continue"
	err := ValidateProgression(Input{}, packet)
	if err == nil {
		t.Fatal("expected generic player intent rejection")
	}
	if !strings.Contains(err.Error(), "player-centered player_intent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type recordingPlanner struct {
	input Input
	next  scene.StoryPacket
}

func (p *recordingPlanner) Plan(_ context.Context, input Input) (scene.StoryPacket, error) {
	p.input = input
	return p.next, p.next.Validate()
}

type invalidPlanner struct{}

func (invalidPlanner) Plan(context.Context, Input) (scene.StoryPacket, error) {
	return scene.StoryPacket{}, nil
}

func repeatedSituationPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "repeated-situation",
		EntryNodeID: "start",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Title: "Record Management",
			Beats: []scene.Beat{{Text: "The same repair record has already been accepted by the crew."}},
			Choices: []scene.Choice{{
				ID:           "carry_record",
				Label:        "Carry the record to another archive shelf.",
				TargetNodeID: "carry",
				PlayerIntent: "Accept duty by moving the same recorded cost into another storage place.",
			}, {
				ID:           "copy_log",
				Label:        "Copy the repair log into a hidden record.",
				TargetNodeID: "copy",
				PlayerIntent: "Control crew knowledge by hiding the same situation as a risky private record.",
			}, {
				ID:           "hide_record",
				Label:        "Hide the record and preserve the same account.",
				TargetNodeID: "hide",
				PlayerIntent: "Protect status while risking agency by preserving the same material.",
			}},
		}, {
			ID:      "carry",
			Title:   "Carry The Record",
			Beats:   []scene.Beat{{Text: "The record is carried to another shelf for preservation."}},
			Choices: []scene.Choice{},
		}, {
			ID:      "copy",
			Title:   "Copy The Log",
			Beats:   []scene.Beat{{Text: "The repair log is copied into a ledger and hidden."}},
			Choices: []scene.Choice{},
		}, {
			ID:      "hide",
			Title:   "Hide The Record",
			Beats:   []scene.Beat{{Text: "The record is concealed so the same account can be preserved."}},
			Choices: []scene.Choice{},
		}},
	}
}

func actorRehearsalPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "actor-rehearsal",
		EntryNodeID: "start",
		ContextSnapshot: scene.ContextSnapshot{
			PresentActors: []string{"tonto"},
		},
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Title: "Character Pressure",
			Beats: []scene.Beat{{Text: "Tonto's lens waits while the player decides who pays for the exposed record."}},
			Choices: []scene.Choice{{
				ID:           "protect_record",
				Label:        "Protect the record and make Tonto answer for the cost.",
				TargetNodeID: "record",
				PlayerIntent: "Protect truth by forcing character pressure onto the player's chosen cost.",
			}, {
				ID:           "risk_silence",
				Label:        "Risk silence and keep the record away from Tonto.",
				TargetNodeID: "silence",
				PlayerIntent: "Risk loyalty and knowledge by keeping Tonto away from the record.",
			}},
		}, {
			ID:      "record",
			Title:   "Record Kept",
			Beats:   []scene.Beat{{Text: "The record remains visible, and the cost becomes public."}},
			Choices: []scene.Choice{},
		}, {
			ID:      "silence",
			Title:   "Silence Held",
			Beats:   []scene.Beat{{Text: "The record is hidden, and trust becomes dangerous."}},
			Choices: []scene.Choice{},
		}},
	}
}
