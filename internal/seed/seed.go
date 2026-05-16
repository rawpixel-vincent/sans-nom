package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type Service struct {
	Scenarist     prompt.Runner
	Verifier      prompt.Runner
	MaxIterations int
}

type Review struct {
	Accepted             bool     `json:"accepted"`
	BlockingIssues       []string `json:"blocking_issues"`
	RevisionInstructions string   `json:"revision_instructions"`
}

func (s Service) Generate(ctx context.Context, contextText string) (scene.StoryPacket, error) {
	if s.Scenarist == nil {
		packet := DefaultPacket()
		return packet, ValidateOriginalSeed(packet)
	}
	maxIterations := s.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 2
	}
	var previous Review
	for i := 0; i < maxIterations; i++ {
		promptText := contextText
		if len(previous.BlockingIssues) > 0 || previous.RevisionInstructions != "" {
			promptText += "\n\nPrevious verifier review:\n" + reviewText(previous)
		}
		raw, err := s.Scenarist.Run(ctx, prompt.SeedScenaristPrompt(promptText))
		if err != nil {
			return scene.StoryPacket{}, err
		}
		packet, err := parsePacket(raw)
		if err != nil {
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Return only valid StoryPacket JSON."}
			continue
		}
		if err := ValidateOriginalSeed(packet); err != nil {
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Revise to create an original opening StoryPacket and keep source lore as influence only."}
			continue
		}
		if s.Verifier == nil {
			return packet, nil
		}
		packetJSON, _ := json.MarshalIndent(packet, "", "  ")
		rawReview, err := s.Verifier.Run(ctx, prompt.SeedVerifierPrompt(string(packetJSON), contextText))
		if err != nil {
			return scene.StoryPacket{}, err
		}
		review, err := parseReview(rawReview)
		if err != nil {
			return scene.StoryPacket{}, err
		}
		if review.Accepted {
			return packet, nil
		}
		previous = review
	}
	return scene.StoryPacket{}, fmt.Errorf("seed story rejected: %s", reviewText(previous))
}

func ValidateOriginalSeed(packet scene.StoryPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if len(packet.Nodes) < 3 || len(packet.Nodes) > 5 {
		return fmt.Errorf("seed packet must have 3-5 nodes, got %d", len(packet.Nodes))
	}
	for _, node := range packet.Nodes {
		needs, err := actors.NeedsForNode(node)
		if err != nil {
			return err
		}
		if len(needs) > 0 {
			return fmt.Errorf("seed node %q uses actor_needs before original actor cards exist", node.ID)
		}
	}
	lower := strings.ToLower(createdStoryText(packet))
	for _, banned := range forbiddenCanonTerms() {
		if containsForbidden(lower, banned) {
			return fmt.Errorf("seed packet copies source-lore term %q; create a new story instead", banned)
		}
	}
	return nil
}

func containsForbidden(text string, banned string) bool {
	index := strings.Index(text, banned)
	for index >= 0 {
		start := index - 16
		if start < 0 {
			start = 0
		}
		prefix := text[start:index]
		if !strings.Contains(prefix, "no ") && !strings.Contains(prefix, "not ") && !strings.Contains(prefix, "avoid ") && !strings.Contains(prefix, "without ") {
			return true
		}
		next := index + len(banned)
		if next >= len(text) {
			return false
		}
		rest := text[next:]
		index = strings.Index(rest, banned)
		if index >= 0 {
			index += next
		}
	}
	return false
}

func ContextText() string {
	return `First playable vertical slice: original science-fantasy comic story.
The Metabarons lore index is creative DNA for the writer and narrator: passion, pressure, contradiction, taboo, body/technology myth, witness-bound memory, and operatic SF-comics taste.
Do not literalize that source lore. The generated story must use new characters, new places, new factions, new conflicts, and new proper nouns.
Do not use Metabarons canon names, places, factions, dynastic events, council mechanics, all-cast fanout, or resurrection-roster assumptions.
For this first seed, do not add actor_needs yet; original actor peer cards will come later. Characters may be present in beats and choices as new story material, but no actor agent should be invoked.
Opening aim: give the player a concrete strange place, one emotional/mechanical taboo, one irreversible pressure point, and 2-4 choices that can branch the current bounded packet.`
}

func DefaultPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "original-seed-001",
		EntryNodeID: "glasswell-threshold",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "phase1-cli",
			SessionID:     "original-opening",
			Location:      "Glasswell underworks",
			Time:          "first bell after artificial dawn",
			PresentActors: []string{},
		},
		Nodes: []scene.SceneNode{{
			ID:         "glasswell-threshold",
			Title:      "The underworks begin to sing",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "You stand beneath the Glasswell habitat where the coolant gutters hum like a choir, and the sealed maintenance iris begins answering a pulse no apprentice is supposed to hear."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices: []scene.Choice{{
				ID:           "trace-pulse",
				Label:        "Trace the pulse through the coolant gutters",
				TargetNodeID: "trace-pulse",
				PlayerIntent: "question",
			}, {
				ID:           "report-iris",
				Label:        "Report the waking iris to the shift matron",
				TargetNodeID: "report-iris",
				PlayerIntent: "seek authority",
			}, {
				ID:           "hide-toolmark",
				Label:        "Hide your toolmark before anyone sees it",
				TargetNodeID: "hide-toolmark",
				PlayerIntent: "transgress",
			}},
			MemoryIntents: []scene.MemoryIntent{{
				ID:        "underworks-pulse",
				Summary:   "The player heard the sealed Glasswell iris answer an unauthorized pulse.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom"},
			}},
		}, {
			ID:         "trace-pulse",
			Title:      "A path in the condensation",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "The pulse moves with the patience of a machine pretending to be alive, leaving bright mineral frost where your fingers nearly touch the pipe."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
		}, {
			ID:         "report-iris",
			Title:      "The rule has a human voice",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "The shift matron listens without surprise, then asks which part of the sound felt like a command and which part felt like an invitation."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
		}, {
			ID:         "hide-toolmark",
			Title:      "A small lie gets warm",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Your toolmark darkens under your sleeve, warmed by the same pulse, and the hidden stain feels less like evidence than a choice you already made."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
		}},
		RevisionNotes: []string{"Opening seed for original CLI playtest; source lore is influence only, no literal canon import, no actor_needs yet."},
	}
}

func forbiddenCanonTerms() []string {
	return []string{
		"aghora",
		"aghnar",
		"bushitaka",
		"castaka",
		"council of castaka",
		"epyphite",
		"honorata",
		"lothar",
		"meta-baron",
		"metabaron",
		"metabunker",
		"oda",
		"othon",
		"roundtable",
		"shabda-oud",
		"steelhead",
		"tonto",
		"vicenta",
	}
}

func createdStoryText(packet scene.StoryPacket) string {
	var b strings.Builder
	b.WriteString(packet.ContextSnapshot.Location)
	b.WriteByte('\n')
	b.WriteString(packet.ContextSnapshot.Time)
	for _, actor := range packet.ContextSnapshot.PresentActors {
		b.WriteByte('\n')
		b.WriteString(actor)
	}
	for _, node := range packet.Nodes {
		b.WriteByte('\n')
		b.WriteString(node.Title)
		for _, beat := range node.Beats {
			b.WriteByte('\n')
			b.WriteString(beat.Text)
		}
		for _, choice := range node.Choices {
			b.WriteByte('\n')
			b.WriteString(choice.Label)
			b.WriteByte('\n')
			b.WriteString(choice.PlayerIntent)
		}
		for _, need := range node.ActorNeeds {
			b.WriteByte('\n')
			b.WriteString(need.ActorID)
			b.WriteByte('\n')
			b.WriteString(need.Reason)
			for _, target := range need.Targets {
				b.WriteByte('\n')
				b.WriteString(target)
			}
		}
		for _, intent := range node.MemoryIntents {
			b.WriteByte('\n')
			b.WriteString(intent.Summary)
			for _, witness := range intent.Witnesses {
				b.WriteByte('\n')
				b.WriteString(witness)
			}
			if intent.PrivateToActor != "" {
				b.WriteByte('\n')
				b.WriteString(intent.PrivateToActor)
			}
		}
	}
	return b.String()
}

func parsePacket(raw string) (scene.StoryPacket, error) {
	var packet scene.StoryPacket
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&packet); err != nil {
		return scene.StoryPacket{}, fmt.Errorf("parse seed packet: %w", err)
	}
	return packet, packet.Validate()
}

func parseReview(raw string) (Review, error) {
	var review Review
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&review); err != nil {
		return Review{}, err
	}
	if review.Accepted {
		return review, nil
	}
	if len(review.BlockingIssues) == 0 && strings.TrimSpace(review.RevisionInstructions) == "" {
		return Review{}, errors.New("rejected review needs blocking issues or revision instructions")
	}
	return review, nil
}

func reviewText(review Review) string {
	var b strings.Builder
	for _, issue := range review.BlockingIssues {
		if strings.TrimSpace(issue) != "" {
			b.WriteString("- ")
			b.WriteString(issue)
			b.WriteByte('\n')
		}
	}
	if review.RevisionInstructions != "" {
		b.WriteString(review.RevisionInstructions)
	}
	return strings.TrimSpace(b.String())
}
