package scene

import "testing"

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

	if err := packet.Validate(); err == nil {
		t.Fatal("expected missing target validation error")
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
