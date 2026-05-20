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
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
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
	AgentRoot      string
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
	TurnID             string `json:"turn_id"`
	Title              string `json:"title"`
	PromptPath         string `json:"prompt_path"`
	OutputPath         string `json:"output_path"`
	MetaPath           string `json:"metadata_path"`
	ArtifactPromptPath string `json:"artifact_prompt_path,omitempty"`
	ArtifactOutputPath string `json:"artifact_output_path,omitempty"`
	ArtifactMetaPath   string `json:"artifact_metadata_path,omitempty"`
	Summary            string `json:"summary"`
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
	writerMetadata := promptRunnerMetadata(s.Writer)
	verifierMetadata := promptRunnerMetadata(s.Verifier)
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
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":    i + 1,
				"prompt_chars": len(writerPrompt),
			}, writerMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"prompt":         writerPrompt,
				"model_metadata": writerMetadata,
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
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":      i + 1,
				"response_chars": len(raw),
			}, writerMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"response":       raw,
				"model_metadata": writerMetadata,
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
				Fields:   modelmeta.MergeFields(map[string]any{"iteration": i + 1}, writerMetadata),
				Detail: map[string]any{
					"iteration":             i + 1,
					"validation_error":      err.Error(),
					"revision_instructions": previous.RevisionInstructions,
					"model_metadata":        writerMetadata,
				},
			})
			continue
		}
		if err := story.ValidateBundle(bundle); err != nil {
			previousRaw = raw
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Revise to create a lore-faithful StoryBible plus opening StoryPacket. Preserve the same scenario contract and fix the structural issue."}
			s.log(gamelog.Event{
				Level:    gamelog.LevelWarn,
				Category: "model",
				Event:    "seed.bundle.invalid",
				Message:  err.Error(),
				Fields:   modelmeta.MergeFields(map[string]any{"iteration": i + 1}, writerMetadata),
				Detail: map[string]any{
					"iteration":             i + 1,
					"validation_error":      err.Error(),
					"revision_instructions": previous.RevisionInstructions,
					"bundle":                bundle,
					"model_metadata":        writerMetadata,
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
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":    i + 1,
				"prompt_chars": len(verifierPrompt),
				"title":        bundle.Bible.Title,
				"packet_id":    bundle.Packet.ID,
			}, verifierMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"prompt":         verifierPrompt,
				"bundle":         bundle,
				"model_metadata": verifierMetadata,
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
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":      i + 1,
				"response_chars": len(rawReview),
			}, verifierMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"response":       rawReview,
				"model_metadata": verifierMetadata,
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
				Fields: modelmeta.MergeFields(map[string]any{
					"iteration": i + 1,
					"title":     bundle.Bible.Title,
					"packet_id": bundle.Packet.ID,
				}, verifierMetadata),
				Detail: map[string]any{
					"iteration":      i + 1,
					"review":         review,
					"bundle":         bundle,
					"model_metadata": verifierMetadata,
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
	workshopMetadata := promptRunnerMetadata(workshopWriter)
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
			Fields: modelmeta.MergeFields(map[string]any{
				"turn":         index + 1,
				"turn_id":      turn.ID,
				"title":        turn.Title,
				"prompt_chars": len(promptText),
				"prompt_path":  filepath.ToSlash(promptPath),
			}, workshopMetadata),
			Detail: map[string]any{
				"turn":           index + 1,
				"turn_id":        turn.ID,
				"title":          turn.Title,
				"prompt":         promptText,
				"prompt_path":    filepath.ToSlash(promptPath),
				"model_metadata": workshopMetadata,
			},
		})
		output, err := workshopWriter.Run(ctx, promptText)
		if err != nil {
			s.log(gamelog.Event{
				Level:    gamelog.LevelError,
				Category: "model",
				Event:    "seed.workshop.failed",
				Message:  err.Error(),
				Fields: modelmeta.MergeFields(map[string]any{
					"turn":        index + 1,
					"turn_id":     turn.ID,
					"title":       turn.Title,
					"prompt_path": filepath.ToSlash(promptPath),
				}, workshopMetadata),
				Detail: map[string]any{
					"turn":           index + 1,
					"turn_id":        turn.ID,
					"title":          turn.Title,
					"prompt":         promptText,
					"prompt_path":    filepath.ToSlash(promptPath),
					"error":          err.Error(),
					"model_metadata": workshopMetadata,
				},
			})
			return "", err
		}
		if err := writeTextAtomic(outputPath, output); err != nil {
			return "", err
		}
		artifact := WorkshopArtifact{
			TurnID:             turn.ID,
			Title:              turn.Title,
			PromptPath:         s.modelVisiblePath(promptPath),
			OutputPath:         s.modelVisiblePath(outputPath),
			MetaPath:           s.modelVisiblePath(metaPath),
			ArtifactPromptPath: filepath.ToSlash(promptPath),
			ArtifactOutputPath: filepath.ToSlash(outputPath),
			ArtifactMetaPath:   filepath.ToSlash(metaPath),
			Summary:            workshopSummary(turn),
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
			"model":         workshopMetadata,
			"planning_only": true,
			"created_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return "", err
		}
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.workshop.response",
			Message:  "seed workshop response received",
			Fields: modelmeta.MergeFields(map[string]any{
				"turn":          index + 1,
				"turn_id":       turn.ID,
				"title":         turn.Title,
				"output_chars":  len(output),
				"output_path":   artifact.OutputPath,
				"metadata_path": artifact.MetaPath,
			}, workshopMetadata),
			Detail: map[string]any{
				"turn":           index + 1,
				"turn_id":        turn.ID,
				"title":          turn.Title,
				"artifact":       artifact,
				"output":         output,
				"model_metadata": workshopMetadata,
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
	return nil
}

func workshopManifest(artifacts []WorkshopArtifact) string {
	if len(artifacts) == 0 {
		return "No workshop artifacts yet."
	}
	var b strings.Builder
	fmt.Fprintln(&b, "These summaries are navigation aids only. Repeated motifs in workshop output are not requirements; the final Writer may discard any motif that became too repetitive.")
	for _, artifact := range artifacts {
		fmt.Fprintf(&b, "- %s (%s)\n  - output: %s\n  - summary: %s\n", artifact.Title, artifact.TurnID, artifact.OutputPath, strings.TrimSpace(artifact.Summary))
	}
	return strings.TrimSpace(b.String())
}

func workshopSummary(turn WorkshopTurn) string {
	switch turn.ID {
	case "imagination":
		return "Contains candidate opening situations for comparison. Read the output file before selecting or developing a specific premise; the manifest intentionally avoids carrying names or motifs forward."
	case "elements":
		return "Contains story-engine comparison, recommendation, and useful alternate sparks. Read the output file before dependent development."
	case "engine":
		return "Contains focused development for the selected story engine: protagonist, opposition, relationships, material world, reversals, and scene pressure."
	case "runway":
		return "Contains prose-level planning for the opening three-turn runway and branch implications. Read the output file before producing the final structured bundle."
	default:
		title := strings.TrimSpace(turn.Title)
		if title == "" {
			title = "Workshop artifact"
		}
		return title + " planning material. Read the output file for details; the manifest avoids copying raw workshop text."
	}
}

func (s Service) log(event gamelog.Event) {
	if s.Logger == nil {
		return
	}
	_ = s.Logger.Append(event)
}

func (s Service) modelVisiblePath(path string) string {
	root := strings.TrimSpace(s.AgentRoot)
	if root == "" {
		return filepath.ToSlash(path)
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	absRoot, rootErr := filepath.Abs(root)
	absPath, pathErr := filepath.Abs(path)
	if rootErr == nil && pathErr == nil {
		if rel, err := filepath.Rel(absRoot, absPath); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func promptRunnerMetadata(runner prompt.Runner) modelmeta.Metadata {
	if runner == nil {
		return modelmeta.Metadata{}
	}
	provider, ok := runner.(interface {
		ModelMetadata() modelmeta.Metadata
	})
	if !ok {
		return modelmeta.Metadata{}
	}
	return provider.ModelMetadata()
}

func ValidateFaithfulSeed(packet scene.StoryPacket) error {
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
	return nil
}

func ContextText() string {
	return `First playable vertical slice: lore-faithful staging for the selected lorepack scenario.
The lorepack scenario contract is the source of truth for premise, protagonist role, opening pressure, must-honor rules, and choice logic.
The seed Writer packages that authored scenario into durable story state and a bounded three-turn packet; it does not invent a replacement premise.
Opening aim: player, desire, place, pressure, active opposition, and a choice that reveals character.
Reader plausibility means the unusual feels lived-in through internal cause, daily practice, emotional credibility, visible cost, and consequence.
Binding rites, laws, or doctrines should support character conflict rather than replacing story with procedure.
For this first seed, original actor peer cards are not generated yet. Characters may be present as story material, but no actor agent should be invoked.`
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
			"Glasswell is a brine-glass lock-city built around transparent tide shafts, public debt shrines, and machinery that treats memory as legal evidence.",
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
		ForbiddenLiteralCanonTerms: []string{},
		FirstTurnExposition: []string{
			"Establish Glasswell as a city whose public law is machinery, ritual, crowd, and debt.",
			"Introduce Ila Venn as the player character and show why her body matters to the city.",
			"Use the first turn as a non-branching onboarding scene that lets the player choose Ila's small public posture.",
			"Use the second turn for the real dilemma: family memory, brother's food ledger, Talik's pressure, and Sarex's illegal option.",
			"End the seed runway with one prebuilt branch scene for each real second-turn choice.",
		},
		ResumeSummary: "Ila stands under her mother's sealed memory pearl while the final audit lantern warms and the city demands her left hand as living collateral.",
		Metadata: map[string]string{
			"seed_mode": "static",
		},
	}
}

func DefaultPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "storypacket_glasswell_opening_001",
		EntryNodeID: "turn_1_intro",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "static-seed",
			SessionID:     "main",
			Location:      "Veyr Tidemarket, beneath the Mourning Lantern",
			Time:          "one audit lantern before public debt certification",
			PresentActors: []string{},
		},
		Nodes: []scene.SceneNode{{
			ID:    "turn_1_intro",
			Title: "Ila arrives beneath the lantern",
			Beats: []scene.Beat{
				{ID: "beat-world", Text: "Glasswell's tide has withdrawn from the market streets, exposing glass channels, debt shrines, stranded mirror-eels, and a crowd waiting for the mourning auction to begin."},
				{ID: "beat-player", Text: "Ila Venn, junior condenser mechanic and inherited keeper of a sleeping bone-key, pushes through the crowd with her mother's sealed memory pearl hanging above the shrine."},
				{ID: "beat-connection", Text: "Her brother's child-sized food-knot bumps against her belt, making the city's paperwork feel more immediate than the auditors' ceremonial brass."},
				{ID: "beat-posture", Text: "Before anyone formally names the debt, Ila has one small choice of posture in public: how to enter the circle and let the crowd read her."},
			},
			ActorNeeds: []scene.ActorNeed{},
			Choices: []scene.Choice{{
				ID:           "choice_enter_silent",
				Label:        "Enter the witness circle without speaking.",
				TargetNodeID: "turn_2_decision",
				PlayerIntent: "choose a guarded public posture",
			}, {
				ID:           "choice_enter_with_brother_knot",
				Label:        "Hold up your brother's food-knot so the crowd sees it.",
				TargetNodeID: "turn_2_decision",
				PlayerIntent: "choose a pleading public posture",
			}, {
				ID:           "choice_enter_touching_wrist",
				Label:        "Keep one hand over the bone-key hidden in your wrist.",
				TargetNodeID: "turn_2_decision",
				PlayerIntent: "choose a defensive public posture",
			}},
			MemoryIntents: []scene.MemoryIntent{{
				ID:        "opening-arrival",
				Summary:   "Ila Venn entered the Veyr Tidemarket mourning circle while her mother's memory pearl hung above the shrine.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom", "Tidemarket crowd"},
			}},
		}, {
			ID:    "turn_2_decision",
			Title: "The final audit lantern warms",
			Beats: []scene.Beat{
				{ID: "beat-law", Text: "Talik Oathsmith names the debt before the auditors: the city will preserve Ila's mother's pearl only if Ila surrenders the living function of her left hand."},
				{ID: "beat-pressure", Text: "If the final audit lantern turns white first, the pearl becomes state salvage and Ila's brother is erased from the dawn food ledgers."},
				{ID: "beat-opposition", Text: "Talik's tablet carries a fogged-over clause Ila recognizes as altered glass, while Sarex of the Red Loom watches from a stall that sells illegal witness-thread."},
				{ID: "beat-choice", Text: "The bone-key under Ila's skin wakes early, forcing her to choose a stance before the crowd decides what courage should look like."},
			},
			ActorNeeds: []scene.ActorNeed{},
			Choices: []scene.Choice{{
				ID:           "choice_submit_hand",
				Label:        "Offer Ila's left hand to the witness-machine and keep the funeral lawful.",
				TargetNodeID: "turn_3_submit_hand",
				PlayerIntent: "submit to the law to preserve family memory",
			}, {
				ID:           "choice_steal_pearl",
				Label:        "Break the Mourning Lantern's seal and steal the memory pearl.",
				TargetNodeID: "turn_3_steal_pearl",
				PlayerIntent: "defy the law by stealing the family memory",
			}, {
				ID:           "choice_bargain_sarex",
				Label:        "Go to Sarex of the Red Loom and ask for a counterfeit witness-thread.",
				TargetNodeID: "turn_3_bargain_sarex",
				PlayerIntent: "seek illegal technical help",
			}},
		}, {
			ID:         "turn_3_submit_hand",
			Title:      "The lawful wound",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Ila places her wrist in the cuff, and the machine measures which memories her hand will still be allowed to hold after the cut."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}, {
			ID:         "turn_3_steal_pearl",
			Title:      "The lantern opens sideways",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "The bone-key refuses the public lock and instead opens a black side-door in the air beside the pearl, marking Ila as thief before every witness."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}, {
			ID:         "turn_3_bargain_sarex",
			Title:      "The red thread price",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Sarex offers a witness-thread that can fool the machine but not the crowd, and asks which part of Ila's name may be spent as payment."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after this branch.",
			},
		}},
		RevisionNotes: []string{"Opening seed runway for faithful CLI playtest; no actor_needs yet.", "Turn 1 choices are cosmetic and all lead to turn_2_decision; turn 2 choices branch into three prebuilt terminal handoff nodes."},
	}
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
