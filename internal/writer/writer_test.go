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
	packet.Nodes[0].Choices[2] = scene.Choice{ID: "claim_weapon", Label: "Take the witness mask to the weapon locker and force the rival claimant into public contest.", TargetNodeID: "weapon"}
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
			}, {
				ID:           "copy_ledger",
				Label:        "Copy the servant testimony into a hidden record.",
				TargetNodeID: "copy",
			}, {
				ID:           "hide_witness",
				Label:        "Hide the witness and preserve the mask proof.",
				TargetNodeID: "hide",
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
