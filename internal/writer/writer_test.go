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

func TestRefreshAsyncRejectsSpentProofChoiceLoop(t *testing.T) {
	packet := proofLoopPacket()
	svc := Service{Planner: StaticPlanner{Next: packet}, Validator: ValidateProgression}

	job, err := svc.RefreshAsync(context.Background(), Input{
		SpentMotifs: []string{"witness/testimony proof", "servant/household memory"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := job.Await(context.Background())
	if result.Err == nil {
		t.Fatal("expected spent proof loop rejection")
	}
	if !strings.Contains(result.Err.Error(), "spent proof mechanism") {
		t.Fatalf("unexpected error: %v", result.Err)
	}
}

func TestRefreshAsyncAllowsSpentProofWithMacroPressure(t *testing.T) {
	packet := proofLoopPacket()
	packet.Nodes[0].Choices[2] = scene.Choice{
		ID:           "claim_weapon",
		Label:        "Take the witness mask to the weapon locker and force the rival claimant into public contest.",
		TargetNodeID: "weapon",
		PlayerIntent: "Claim player agency by moving the conflict from spent witness proof to weapon succession pressure.",
	}
	packet.Nodes[3] = scene.SceneNode{
		ID:      "weapon",
		Title:   "Weapon Schedule",
		Beats:   []scene.Beat{{Text: "The weapon locker opens to reveal the altered succession blade schedule and the rival claimant waiting beside it."}},
		Choices: []scene.Choice{},
	}
	svc := Service{Planner: StaticPlanner{Next: packet}, Validator: ValidateProgression}

	job, err := svc.RefreshAsync(context.Background(), Input{
		SpentMotifs: []string{"witness/testimony proof", "servant/household memory"},
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
	if !strings.Contains(err.Error(), "needs actor rehearsal") {
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
		Reason:  "Tonto's witness pressure should shape the playable beat.",
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

func proofLoopPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "proof-loop",
		EntryNodeID: "start",
		Nodes: []scene.SceneNode{{
			ID:    "start",
			Title: "Proof Management",
			Beats: []scene.Beat{{Text: "The servant testimony and witness mask have already been accepted by the household."}},
			Choices: []scene.Choice{{
				ID:           "carry_mask",
				Label:        "Carry the witness mask to another servant ledger.",
				TargetNodeID: "carry",
				PlayerIntent: "Accept duty by moving the same witnessed cost into another servant record.",
			}, {
				ID:           "copy_ledger",
				Label:        "Copy the servant testimony into a hidden record.",
				TargetNodeID: "copy",
				PlayerIntent: "Control household knowledge by hiding testimony as a risky private record.",
			}, {
				ID:           "hide_witness",
				Label:        "Hide the witness and preserve the mask proof.",
				TargetNodeID: "hide",
				PlayerIntent: "Protect the witness while risking status by preserving the same proof.",
			}},
		}, {
			ID:      "carry",
			Title:   "Carry The Mask",
			Beats:   []scene.Beat{{Text: "The mask is carried to another servant record for preservation."}},
			Choices: []scene.Choice{},
		}, {
			ID:      "copy",
			Title:   "Copy The Ledger",
			Beats:   []scene.Beat{{Text: "The testimony is copied into a ledger and hidden."}},
			Choices: []scene.Choice{},
		}, {
			ID:      "hide",
			Title:   "Hide The Witness",
			Beats:   []scene.Beat{{Text: "The witness is concealed so the proof can be preserved."}},
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
			Title: "Witness Pressure",
			Beats: []scene.Beat{{Text: "Tonto's lens waits while the player decides who pays for the exposed record."}},
			Choices: []scene.Choice{{
				ID:           "protect_record",
				Label:        "Protect the record and make Tonto witness the cost.",
				TargetNodeID: "record",
				PlayerIntent: "Protect truth by forcing witness pressure onto the player's chosen cost.",
			}, {
				ID:           "risk_silence",
				Label:        "Risk silence and keep the record away from Tonto.",
				TargetNodeID: "silence",
				PlayerIntent: "Risk loyalty and knowledge by keeping the witness away from the record.",
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
