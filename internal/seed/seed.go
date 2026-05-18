package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

type Service struct {
	Writer         prompt.Runner
	WorkshopWriter prompt.Runner
	Verifier       prompt.Runner
	MaxIterations  int
	Logger         *gamelog.Writer
	WorkshopDir    string
}

type Review struct {
	Accepted             bool     `json:"accepted"`
	BlockingIssues       []string `json:"blocking_issues"`
	RevisionInstructions string   `json:"revision_instructions"`
}

type WorkshopTurn struct {
	ID          string
	Title       string
	PromptShape string
}

type WorkshopArtifact struct {
	TurnID     string `json:"turn_id"`
	Title      string `json:"title"`
	PromptPath string `json:"prompt_path"`
	OutputPath string `json:"output_path"`
	MetaPath   string `json:"metadata_path"`
	Summary    string `json:"summary"`
}

func (s Service) Generate(ctx context.Context, contextText string) (story.SeedBundle, error) {
	if s.Writer == nil {
		bundle := DefaultBundle()
		return bundle, story.ValidateBundle(bundle)
	}
	workshopContext, err := s.buildWorkshop(ctx, contextText)
	if err != nil {
		return story.SeedBundle{}, err
	}
	contextText += workshopContext
	maxIterations := s.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 2
	}
	var previous Review
	var previousRaw string
	for i := 0; i < maxIterations; i++ {
		promptText := contextText
		if len(previous.BlockingIssues) > 0 || previous.RevisionInstructions != "" {
			promptText += "\n\nPrevious Writer attempt failed validation. Continue from this attempt and revise it; do not start a new story seed from scratch. Preserve the same story, bible, player character, and packet structure unless directly required by the error.\n"
			promptText += "\nValidation/review feedback:\n" + reviewText(previous)
			if strings.TrimSpace(previousRaw) != "" {
				promptText += "\n\nPrevious raw Writer output to patch:\n" + previousRaw
			}
		}
		writerPrompt := prompt.SeedWriterPrompt(promptText)
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.writer.request",
			Message:  "seed writer request prepared",
			Fields: map[string]any{
				"iteration":    i + 1,
				"prompt_chars": len(writerPrompt),
			},
			Detail: map[string]any{
				"iteration": i + 1,
				"prompt":    writerPrompt,
			},
		})
		raw, err := s.Writer.Run(ctx, writerPrompt)
		if err != nil {
			return story.SeedBundle{}, err
		}
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.writer.response",
			Message:  "seed writer response received",
			Fields: map[string]any{
				"iteration":      i + 1,
				"response_chars": len(raw),
			},
			Detail: map[string]any{
				"iteration": i + 1,
				"response":  raw,
			},
		})
		bundle, err := parseBundle(raw)
		if err != nil {
			previousRaw = raw
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Return only valid JSON with top-level bible and packet."}
			s.log(gamelog.Event{
				Level:    gamelog.LevelWarn,
				Category: "model",
				Event:    "seed.writer.invalid",
				Message:  err.Error(),
				Fields:   map[string]any{"iteration": i + 1},
				Detail: map[string]any{
					"iteration":             i + 1,
					"validation_error":      err.Error(),
					"revision_instructions": previous.RevisionInstructions,
				},
			})
			continue
		}
		if err := story.ValidateBundle(bundle); err != nil {
			previousRaw = raw
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Revise to create an original StoryBible plus opening StoryPacket. Keep source lore as influence only."}
			s.log(gamelog.Event{
				Level:    gamelog.LevelWarn,
				Category: "model",
				Event:    "seed.bundle.invalid",
				Message:  err.Error(),
				Fields:   map[string]any{"iteration": i + 1},
				Detail: map[string]any{
					"iteration":             i + 1,
					"validation_error":      err.Error(),
					"revision_instructions": previous.RevisionInstructions,
					"bundle":                bundle,
				},
			})
			continue
		}
		if s.Verifier == nil {
			return bundle, nil
		}
		bundleJSON, _ := json.MarshalIndent(bundle, "", "  ")
		verifierPrompt := prompt.SeedVerifierPrompt(string(bundleJSON), contextText)
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.verifier.request",
			Message:  "seed verifier request prepared",
			Fields: map[string]any{
				"iteration":    i + 1,
				"prompt_chars": len(verifierPrompt),
				"title":        bundle.Bible.Title,
				"packet_id":    bundle.Packet.ID,
			},
			Detail: map[string]any{
				"iteration": i + 1,
				"prompt":    verifierPrompt,
				"bundle":    bundle,
			},
		})
		rawReview, err := s.Verifier.Run(ctx, verifierPrompt)
		if err != nil {
			return story.SeedBundle{}, err
		}
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.verifier.response",
			Message:  "seed verifier response received",
			Fields: map[string]any{
				"iteration":      i + 1,
				"response_chars": len(rawReview),
			},
			Detail: map[string]any{
				"iteration": i + 1,
				"response":  rawReview,
			},
		})
		review, err := parseReview(rawReview)
		if err != nil {
			return story.SeedBundle{}, err
		}
		if review.Accepted {
			s.log(gamelog.Event{
				Category: "model",
				Event:    "seed.verifier.accepted",
				Message:  "seed verifier accepted bundle",
				Fields: map[string]any{
					"iteration": i + 1,
					"title":     bundle.Bible.Title,
					"packet_id": bundle.Packet.ID,
				},
				Detail: map[string]any{
					"iteration": i + 1,
					"review":    review,
					"bundle":    bundle,
				},
			})
			return bundle, nil
		}
		previous = review
		previousRaw = string(bundleJSON)
	}
	return story.SeedBundle{}, fmt.Errorf("seed story rejected: %s", reviewText(previous))
}

func (s Service) buildWorkshop(ctx context.Context, contextText string) (string, error) {
	if strings.TrimSpace(s.WorkshopDir) == "" {
		return "", nil
	}
	workshopWriter := s.WorkshopWriter
	if workshopWriter == nil {
		workshopWriter = s.Writer
	}
	turns := DefaultWorkshopTurns()
	if len(turns) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(s.WorkshopDir, 0o755); err != nil {
		return "", err
	}
	artifacts := make([]WorkshopArtifact, 0, len(turns))
	var previous []WorkshopArtifact
	for index, turn := range turns {
		promptText := prompt.SeedWorkshopPrompt(contextText, turn.Title, turn.PromptShape, workshopManifest(previous))
		turnDir := filepath.Join(s.WorkshopDir, fmt.Sprintf("%02d-%s", index+1, sanitizeSegment(turn.ID)))
		if err := os.MkdirAll(turnDir, 0o755); err != nil {
			return "", err
		}
		promptPath := filepath.Join(turnDir, "prompt.txt")
		outputPath := filepath.Join(turnDir, "output.md")
		metaPath := filepath.Join(turnDir, "metadata.json")
		if err := writeTextAtomic(promptPath, promptText); err != nil {
			return "", err
		}
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.workshop.request",
			Message:  "seed workshop request prepared",
			Fields: map[string]any{
				"turn":         index + 1,
				"turn_id":      turn.ID,
				"title":        turn.Title,
				"prompt_chars": len(promptText),
				"prompt_path":  filepath.ToSlash(promptPath),
			},
			Detail: map[string]any{
				"turn":        index + 1,
				"turn_id":     turn.ID,
				"title":       turn.Title,
				"prompt":      promptText,
				"prompt_path": filepath.ToSlash(promptPath),
			},
		})
		output, err := workshopWriter.Run(ctx, promptText)
		if err != nil {
			s.log(gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "model",
				Event:    "seed.workshop.failed",
				Message:  err.Error(),
				Fields: map[string]any{
					"turn":        index + 1,
					"turn_id":     turn.ID,
					"title":       turn.Title,
					"prompt_path": filepath.ToSlash(promptPath),
				},
				Detail: map[string]any{
					"turn":        index + 1,
					"turn_id":     turn.ID,
					"title":       turn.Title,
					"prompt":      promptText,
					"prompt_path": filepath.ToSlash(promptPath),
					"error":       err.Error(),
				},
			})
			return "", err
		}
		if err := writeTextAtomic(outputPath, output); err != nil {
			return "", err
		}
		artifact := WorkshopArtifact{
			TurnID:     turn.ID,
			Title:      turn.Title,
			PromptPath: filepath.ToSlash(promptPath),
			OutputPath: filepath.ToSlash(outputPath),
			MetaPath:   filepath.ToSlash(metaPath),
			Summary:    previewText(output, 600),
		}
		if err := writeJSONAtomic(metaPath, map[string]any{
			"turn":          index + 1,
			"turn_id":       turn.ID,
			"title":         turn.Title,
			"prompt_path":   artifact.PromptPath,
			"output_path":   artifact.OutputPath,
			"summary":       artifact.Summary,
			"prompt_chars":  len(promptText),
			"output_chars":  len(output),
			"planning_only": true,
			"created_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return "", err
		}
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.workshop.response",
			Message:  "seed workshop response received",
			Fields: map[string]any{
				"turn":          index + 1,
				"turn_id":       turn.ID,
				"title":         turn.Title,
				"output_chars":  len(output),
				"output_path":   artifact.OutputPath,
				"metadata_path": artifact.MetaPath,
			},
			Detail: map[string]any{
				"turn":     index + 1,
				"turn_id":  turn.ID,
				"title":    turn.Title,
				"artifact": artifact,
				"output":   output,
			},
		})
		artifacts = append(artifacts, artifact)
		previous = artifacts
	}
	manifest := workshopManifest(artifacts)
	if err := writeTextAtomic(filepath.Join(s.WorkshopDir, "manifest.md"), manifest); err != nil {
		return "", err
	}
	if err := writeJSONAtomic(filepath.Join(s.WorkshopDir, "manifest.json"), artifacts); err != nil {
		return "", err
	}
	s.log(gamelog.Event{
		Category: "model",
		Event:    "seed.workshop.completed",
		Message:  "seed workshop completed",
		Fields: map[string]any{
			"artifact_count": len(artifacts),
			"workshop_dir":   filepath.ToSlash(s.WorkshopDir),
		},
		Detail: map[string]any{
			"workshop_dir": filepath.ToSlash(s.WorkshopDir),
			"manifest":     manifest,
			"artifacts":    artifacts,
		},
	})
	return "\n\nSeed Workshop Manifest (planning references only, not canon):\n" + manifest + "\n\nThe workshop files are advisory material. The final Writer owns the StoryBible and StoryPacket; do not copy raw workshop text blindly.", nil
}

func DefaultWorkshopTurns() []WorkshopTurn {
	return []WorkshopTurn{{
		ID:          "imagination",
		Title:       "Imagination Turn",
		PromptShape: "Invent freely. Let ideas collide. Produce 5-8 original science-fantasy story germs, each with a vivid image, a taboo or law, a character wound, and a possible first crisis. Do not choose the final story yet.",
	}, {
		ID:          "elements",
		Title:       "Fiction Elements Turn",
		PromptShape: "Choose the strongest germ. Develop Character, Point of View, Setting, Plot pressure, Theme, and Style/Tone as separate Markdown sections. Keep each element concrete and playable.",
	}, {
		ID:          "law",
		Title:       "Law And Suspension Turn",
		PromptShape: "Stress-test the original gravity law. Explain social enforcement, history, public rituals, edge cases, loopholes, emotional cost, visible consequences, and why refusal matters.",
	}, {
		ID:          "opening",
		Title:       "Opening Scene Engine Turn",
		PromptShape: "Design the opening turn as a scene engine: exposition, player identity, player-world connection, immediate pressure, sensory atmosphere, and 2-4 dramatic choice pressures with branch implications.",
	}}
}

func workshopManifest(artifacts []WorkshopArtifact) string {
	if len(artifacts) == 0 {
		return "No workshop artifacts yet."
	}
	var b strings.Builder
	for _, artifact := range artifacts {
		fmt.Fprintf(&b, "- %s (%s)\n  - output: %s\n  - summary: %s\n", artifact.Title, artifact.TurnID, artifact.OutputPath, strings.TrimSpace(artifact.Summary))
	}
	return strings.TrimSpace(b.String())
}

func (s Service) log(event gamelog.Event) {
	if s.Logger == nil {
		return
	}
	_ = s.Logger.Append(event)
}

func ValidateOriginalSeed(packet scene.StoryPacket) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	if err := story.ValidateSeedPacket(packet); err != nil {
		return err
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
	lower := strings.ToLower(story.CreatedPacketText(packet))
	for _, banned := range story.ForbiddenCanonTerms() {
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
The selected ground-lore pack is creative DNA for the writer and narrator: passion, pressure, contradiction, taboo, body/technology myth, witness-bound memory, and operatic SF-comics taste.
Do not literalize that source lore. The generated story must use new characters, new places, new factions, new conflicts, and new proper nouns.
Do not use source canon names, places, factions, dynastic events, council mechanics, all-cast fanout, or resurrection-roster assumptions.
Return a durable StoryBible plus the first bounded StoryPacket. The StoryBible is binding doctrine for this generated story, separate from source-lore influence.
The StoryBible must contain at least one original gravity law inspired by deep source structures from the selected pack: profound, binding, not merely cruel, and enforced by world reaction rather than system blocking.
The first entry node must be richer than later nodes. It should give the Narrator enough structure for a long classic opening: world, player identity, player-world connection, active taboo/law, immediate pressure, and choices.
For this first seed, do not add actor_needs yet; original actor peer cards will come later. Characters may be present in beats and choices as new story material, but no actor agent should be invoked.
Opening aim: give the player a concrete strange place, one emotional/mechanical taboo, one irreversible pressure point, and 2-4 choices that can branch the current bounded packet.
Every choice target must be an included node in the same bounded packet. If a branch should hand off to Writer continuation, make that branch node terminal with revision_notes instead of adding a placeholder continuation choice.`
}

func DefaultBundle() story.SeedBundle {
	return story.SeedBundle{
		Bible:  DefaultBible(),
		Packet: DefaultPacket(),
	}
}

func DefaultBible() story.StoryBible {
	return story.StoryBible{
		StoryID: "static-seed",
		Title:   "The Glasswell Debt",
		Premise: "A maintenance apprentice inside a vertical sea-city discovers that a civic law meant to preserve memory also harvests living function from the poor.",
		PlayerCharacter: story.PlayerCharacter{
			PeerID:      "sans-nom",
			Name:        "Ila Venn",
			Description: "A junior condenser mechanic with an inherited bone-key sealed under the skin of her left wrist.",
			Desire:      "Ila wants to keep her mother's remembrance intact without surrendering the hand that lets her work.",
			Wound:       "She believes every lawful survival in Glasswell has already been paid for by someone weaker.",
			Boundary:    "Ila will not let the city reduce her family into salvage inventory.",
		},
		PlayerWorldConnection: "Ila's family line maintained the old pressure locks, so the city treats her body as a civic instrument whenever those locks wake.",
		WorldBasics: []string{
			"Glasswell is a suspended sea-city built around transparent tide shafts, public debt shrines, and machinery that treats memory as legal evidence.",
			"House debts are embodied; a function, limb, skill, or memory can be claimed when a family fails a civic rite.",
			"Public witnesses matter more than private truth, so law in Glasswell is performed before crowds and machines.",
		},
		OriginalLaws: []story.OriginalLaw{{
			ID:             "law-of-living-collateral",
			Name:           "Law of Living Collateral",
			Description:    "A debt cannot be closed by money alone; the debtor must surrender a living function that proves the debt changed the body.",
			SourcePattern:  "Bushitaka-like binding law: an intimate taboo becomes civilization-scale structure.",
			Enforcement:    "Crowds, shrines, ledgers, and machines refuse to recognize unmarked repayment; defiance isolates the debtor socially before it harms them physically.",
			WhyProfound:    "The law makes every act of survival visibly costly and turns inheritance into a moral wound.",
			NotMerelyCruel: "It preserves proof against ruling-house fraud, so abolishing it carelessly would also erase the poor dead from civic memory.",
		}},
		Factions: []story.Faction{{
			ID:          "brass-choir",
			Name:        "Brass Choir Auditors",
			Description: "Ceremonial accountants with shared metal throats who certify debt rites in public.",
		}, {
			ID:          "red-loom",
			Name:        "Red Loom",
			Description: "Illicit thread-workers who counterfeit witness marks and sell partial freedom at dangerous prices.",
		}},
		CoreRelationships: []string{
			"Ila owes love to her dead mother and practical protection to a younger brother still inside the food ledgers.",
			"Talik Oathsmith knows the law's mercy clauses but benefits when desperate families never hear them.",
			"Sarex of the Red Loom once owed Ila tenderness and now offers illegal competence with unclear motives.",
		},
		ToneDoctrine: []string{
			"Operatic science-fantasy with body technology, civic ritual, taboo, witness pressure, and desperate agency.",
			"Every law should feel spiritually heavy before it becomes tactical.",
			"Absurd machinery is allowed, but emotional consequence must stay precise.",
		},
		SourceLoreInfluenceRefs: []string{
			"global lore: Bushitaka law as structural inspiration only",
			"global lore: witness-bound narration and actor boundary patterns",
			"global lore: body alteration as inheritance pressure",
		},
		ForbiddenLiteralCanonTerms: story.ForbiddenCanonTerms(),
		FirstTurnExposition: []string{
			"Establish Glasswell as a city whose public law is machinery, ritual, crowd, and debt.",
			"Introduce Ila Venn as the player character and show why her body matters to the city.",
			"Make the Law of Living Collateral active in the opening scene.",
			"Connect the immediate pressure to Ila's mother, brother, and inherited work.",
			"End with 2-4 choices that express submission, theft, illegal bargaining, or accusation.",
		},
		ResumeSummary: "Ila stands under her mother's sealed memory pearl while the ninth bell approaches and the city demands her left hand as living collateral.",
		Metadata: map[string]string{
			"seed_mode": "static",
		},
	}
}

func DefaultPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "storypacket_glasswell_opening_001",
		EntryNodeID: "node_entry_living_collateral",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "static-seed",
			SessionID:     "main",
			Location:      "Veyr Tidemarket, beneath the Mourning Lantern",
			Time:          "the eighth bell, one bell before public debt certification",
			PresentActors: []string{},
		},
		Nodes: []scene.SceneNode{{
			ID:    "node_entry_living_collateral",
			Title: "The city asks for Ila's hand",
			Beats: []scene.Beat{
				{ID: "beat-world", Text: "Glasswell's tide has withdrawn from the market streets, exposing glass channels, debt shrines, stranded mirror-eels, and a crowd waiting for the law to finish drying."},
				{ID: "beat-player", Text: "Ila Venn, junior condenser mechanic and inherited keeper of a sleeping bone-key, stands below her mother's sealed memory pearl with her left sleeve rolled to the elbow."},
				{ID: "beat-law", Text: "The Law of Living Collateral says a family debt becomes real only when a living function is surrendered before witnesses; tonight the auditors name Ila's left hand."},
				{ID: "beat-pressure", Text: "If the ninth bell sounds before submission, the pearl becomes state salvage and Ila's younger brother is struck from the food ledgers by dawn."},
				{ID: "beat-wake", Text: "The bone-key under Ila's skin wakes early and the witness-machine answers, proving the city can already hear the part of her it intends to take."},
			},
			ActorNeeds: []scene.ActorNeed{},
			Choices: []scene.Choice{{
				ID:           "choice_submit_hand",
				Label:        "Offer Ila's left hand to the witness-machine and keep the funeral lawful.",
				TargetNodeID: "node_submit_hand",
				PlayerIntent: "submit to the law to preserve family memory",
			}, {
				ID:           "choice_steal_pearl",
				Label:        "Break the Mourning Lantern's seal and steal the memory pearl.",
				TargetNodeID: "node_steal_pearl",
				PlayerIntent: "defy the law by stealing the family memory",
			}, {
				ID:           "choice_bargain_sarex",
				Label:        "Go to Sarex of the Red Loom and ask for a counterfeit witness-thread.",
				TargetNodeID: "node_bargain_sarex",
				PlayerIntent: "seek illegal technical help",
			}, {
				ID:           "choice_accuse_talik",
				Label:        "Accuse Talik Oathsmith of altering the debt terms.",
				TargetNodeID: "node_accuse_talik",
				PlayerIntent: "challenge the public legal performance",
			}},
			MemoryIntents: []scene.MemoryIntent{{
				ID:        "opening-law",
				Summary:   "Glasswell demanded Ila Venn's left hand as living collateral for her mother's funeral debt.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom", "Tidemarket crowd", "Brass Choir Auditors"},
			}},
		}, {
			ID:         "node_submit_hand",
			Title:      "The lawful wound",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Ila places her wrist in the cuff, and the machine measures which memories her hand will still be allowed to hold after the cut."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}, {
			ID:         "node_steal_pearl",
			Title:      "The lantern opens sideways",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "The bone-key refuses the public lock and instead opens a black side-door in the air beside the pearl, marking Ila as thief before every witness."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}, {
			ID:         "node_bargain_sarex",
			Title:      "The red thread price",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Sarex offers a witness-thread that can fool the machine but not the crowd, and asks which part of Ila's name may be spent as payment."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}, {
			ID:         "node_accuse_talik",
			Title:      "Mercy clauses have witnesses too",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Ila names the hidden mercy clause aloud, forcing Talik and the auditors to decide whether the crowd heard law or insolence."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}},
		RevisionNotes: []string{"Opening seed for original CLI playtest; source lore is influence only, no literal canon import, no actor_needs yet.", "Entry node is intentionally richer than branch nodes for classic first-turn narration."},
	}
}

func forbiddenCanonTerms() []string {
	return story.ForbiddenCanonTerms()
}

func createdStoryText(packet scene.StoryPacket) string {
	return story.CreatedPacketText(packet)
}

func parseBundle(raw string) (story.SeedBundle, error) {
	var bundle story.SeedBundle
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bundle); err != nil {
		return story.SeedBundle{}, fmt.Errorf("parse seed bundle: %w", err)
	}
	return bundle, nil
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

func writeTextAtomic(path string, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeTextAtomic(path, string(data))
}

func sanitizeSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "turn"
	}
	return out
}

func previewText(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}
