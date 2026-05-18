package scene

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestStoryPacketValidationAndChoice(t *testing.T) {
	packet := StoryPacket{
		ID:          "packet-1",
		EntryNodeID: "start",
		Nodes: []SceneNode{{
			ID:    "start",
			Beats: []Beat{{Text: "Sans-Nom waits at the eastern hall."}},
			Choices: []Choice{{
				ID:           "ask",
				Label:        "Ask Tonto about the door",
				TargetNodeID: "tonto",
			}},
			ActorNeeds: []ActorNeed{{
				ActorID: "tonto",
				Mode:    ActorModePresent,
			}},
			MemoryIntents: []MemoryIntent{{
				Summary:   "Sans-Nom hesitated at the eastern hall.",
				Policy:    MemoryPolicyCanon,
				Witnesses: []string{"sans-nom", "tonto"},
			}},
		}, {
			ID:    "tonto",
			Beats: []Beat{{Text: "Tonto answers too slowly."}},
		}},
	}

	if err := packet.Validate(); err != nil {
		t.Fatalf("packet should validate: %v", err)
	}
	node, err := packet.EntryNode()
	if err != nil {
		t.Fatal(err)
	}
	choice, err := node.Choice("ask")
	if err != nil {
		t.Fatal(err)
	}
	if choice.TargetNodeID != "tonto" {
		t.Fatalf("target = %q", choice.TargetNodeID)
	}
}

func TestStoryPacketRejectsMissingChoiceTarget(t *testing.T) {
	packet := StoryPacket{
		ID: "packet-1",
		Nodes: []SceneNode{{
			ID:      "start",
			Beats:   []Beat{{Text: "A beat."}},
			Choices: []Choice{{ID: "bad", Label: "Bad", TargetNodeID: "missing"}},
		}},
	}

	err := packet.Validate()
	if err == nil {
		t.Fatal("expected missing target validation error")
	}
	var missing *MissingChoiceTargetError
	if !errors.As(err, &missing) {
		t.Fatalf("expected MissingChoiceTargetError, got %T: %v", err, err)
	}
	if missing.NodeID != "start" || missing.ChoiceID != "bad" || missing.TargetNodeID != "missing" {
		t.Fatalf("unexpected missing target detail: %#v", missing)
	}
	if !slices.Contains(missing.KnownNodeIDs, "start") {
		t.Fatalf("known node ids missing start: %#v", missing.KnownNodeIDs)
	}
	if !strings.Contains(err.Error(), "missing target node inside bounded packet") {
		t.Fatalf("error should explain bounded packet closure: %v", err)
	}
}

func TestStoryPacketAllowsTerminalHandoffWithRevisionNotes(t *testing.T) {
	packet := StoryPacket{
		ID:          "packet-1",
		EntryNodeID: "handoff",
		Nodes: []SceneNode{{
			ID:            "handoff",
			Beats:         []Beat{{Text: "The branch reaches the edge of this bounded packet."}},
			RevisionNotes: []string{"Writer continuation is required after this branch."},
		}},
	}
	if err := packet.Validate(); err != nil {
		t.Fatalf("terminal handoff should validate: %v", err)
	}
}

func TestCommitRejectsPlanningArtifacts(t *testing.T) {
	commit := Commit{
		FinalProse: "Only this prose reached the player.",
		Events: []CanonEvent{{
			PeerID:    "narrator",
			Content:   "Unused branch.",
			Source:    CommitSourceUnusedBranch,
			Witnesses: []string{"sans-nom"},
		}},
	}

	if err := commit.ValidateCanonOnly(); err == nil {
		t.Fatal("expected unused branch commit to fail")
	}
}

func TestCommitAcceptsWitnessedCanon(t *testing.T) {
	commit := Commit{
		FinalProse: "Tonto says what Sans-Nom can hear.",
		ActorMessages: []ActorMessage{{
			ActorID:   "tonto",
			Interior:  "Private reasoning.",
			Surface:   "A surfaced line.",
			Witnesses: []string{"sans-nom"},
		}},
		Events: []CanonEvent{{
			PeerID:    "tonto",
			Content:   "A surfaced line.",
			Source:    CommitSourceActorSurface,
			Witnesses: []string{"sans-nom"},
		}},
	}

	if err := commit.ValidateCanonOnly(); err != nil {
		t.Fatalf("commit should validate: %v", err)
	}
}

func TestSceneSessionIDScopesCommitsPerNode(t *testing.T) {
	first := SceneSessionID("seed-rain-engine-second-heart-opening", "packet-one", "turn_6_three_minutes_begin")
	second := SceneSessionID("seed-rain-engine-second-heart-opening", "packet-one", "turn_7_deeper_with_heart")

	if first == second {
		t.Fatalf("scene sessions should differ by node: %q", first)
	}
	if !strings.Contains(first, "turn_6_three_minutes_begin") {
		t.Fatalf("scene session should keep a readable node fragment: %q", first)
	}
	if len(first) > MaxSessionIDLength {
		t.Fatalf("scene session too long: %d > %d", len(first), MaxSessionIDLength)
	}
}
