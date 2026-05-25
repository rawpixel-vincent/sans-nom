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
	"github.com/rawpixel-vincent/sans-nom/internal/modelcap"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/prompt"
	"github.com/rawpixel-vincent/sans-nom/internal/scene"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
)

type Service struct {
	Writer          prompt.Runner
	WorkshopWriter  prompt.Runner
	Verifier        prompt.Runner
	MaxIterations   int
	Logger          *gamelog.Writer
	WorkshopDir     string
	AgentRoot       string
	RecoverRunError func(stage string, err error) bool
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
		if len(previous.BlockingIssues) > 0 || previous.RevisionInstructions != "" {
			writerPrompt = prompt.SeedWriterRepairPrompt(promptText)
		}
		writerBudget := seedPromptBudget(s.Writer, "seed_writer", writerPrompt)
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.writer.request",
			Message:  "seed writer request prepared",
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":               i + 1,
				"prompt_chars":            len(writerPrompt),
				"input_budget_tokens":     writerBudget.InputBudgetTokens,
				"estimated_prompt_tokens": writerBudget.EstimatedPromptTokens,
				"unknown_capability":      writerBudget.UnknownCapability,
			}, writerMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"prompt":         writerPrompt,
				"model_metadata": writerMetadata,
				"context_budget": writerBudget.Map(),
			},
		})
		var raw string
		for {
			var err error
			raw, err = runPromptWithBudget(ctx, s.Writer, writerPrompt, writerBudget)
			if err == nil {
				break
			}
			s.logRunFailure("seed.writer.failed", i+1, err, writerPrompt, nil, writerMetadata)
			if s.RecoverRunError == nil || !s.RecoverRunError("seed writer", err) {
				return story.SeedBundle{}, err
			}
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
			previous = Review{BlockingIssues: []string{err.Error()}, RevisionInstructions: "Revise to create a lore-faithful StoryBible plus opening StoryPacket. Preserve the same scenario notes and fix the structural issue. If the error mentions metadata, populate bible.metadata with concrete string values for seed_version, generated_at, resume_point, current_checkpoint, continuation_hooks, canon_validated, open_threads, gameplay_mode, and ground_lore_pack_id."}
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
		verifierBudget := seedPromptBudget(s.Verifier, "seed_verifier", verifierPrompt)
		s.log(gamelog.Event{
			Category: "model",
			Event:    "seed.verifier.request",
			Message:  "seed verifier request prepared",
			Fields: modelmeta.MergeFields(map[string]any{
				"iteration":               i + 1,
				"prompt_chars":            len(verifierPrompt),
				"title":                   bundle.Bible.Title,
				"packet_id":               bundle.Packet.ID,
				"input_budget_tokens":     verifierBudget.InputBudgetTokens,
				"estimated_prompt_tokens": verifierBudget.EstimatedPromptTokens,
				"unknown_capability":      verifierBudget.UnknownCapability,
			}, verifierMetadata),
			Detail: map[string]any{
				"iteration":      i + 1,
				"prompt":         verifierPrompt,
				"bundle":         bundle,
				"model_metadata": verifierMetadata,
				"context_budget": verifierBudget.Map(),
			},
		})
		var rawReview string
		for {
			var err error
			rawReview, err = runPromptWithBudget(ctx, s.Verifier, verifierPrompt, verifierBudget)
			if err == nil {
				break
			}
			s.logRunFailure("seed.verifier.failed", i+1, err, verifierPrompt, bundle, verifierMetadata)
			if s.RecoverRunError == nil || !s.RecoverRunError("seed verifier", err) {
				return story.SeedBundle{}, err
			}
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
		return "Contains prose-level planning for the compact playable opening and branch implications. Read the output file before producing the final structured bundle."
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

func (s Service) logRunFailure(event string, iteration int, err error, promptText string, bundle any, metadata modelmeta.Metadata) {
	fields := modelmeta.MergeFields(map[string]any{
		"iteration": iteration,
	}, metadata)
	detail := map[string]any{
		"iteration":      iteration,
		"error":          err.Error(),
		"prompt":         promptText,
		"model_metadata": metadata,
	}
	if seedBundle, ok := bundle.(story.SeedBundle); ok {
		fields["title"] = seedBundle.Bible.Title
		fields["packet_id"] = seedBundle.Packet.ID
		detail["bundle"] = seedBundle
	}
	s.log(gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "model",
		Event:    event,
		Message:  err.Error(),
		Fields:   fields,
		Detail:   detail,
	})
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

func seedPromptBudget(runner prompt.Runner, role string, text string) modelcap.Budget {
	metadata := promptRunnerMetadata(runner)
	hasTools := false
	hasSchema := false
	maxToolRounds := 0
	switch typed := runner.(type) {
	case prompt.CodexRunner:
		hasTools = typed.LocalTools
		hasSchema = typed.SchemaName != "" && typed.Schema != nil
		maxToolRounds = typed.MaxToolRounds
	case prompt.ClaudeRunner:
		hasTools = len(typed.AllowedTools) > 0
	}
	return modelcap.BudgetFor(modelcap.Request{
		Metadata:      metadata,
		Role:          role,
		HasTools:      hasTools,
		HasJSONSchema: hasSchema,
		MaxToolRounds: maxToolRounds,
	}).WithEstimate(text)
}

func runPromptWithBudget(ctx context.Context, runner prompt.Runner, text string, budget modelcap.Budget) (string, error) {
	if codex, ok := runner.(prompt.CodexRunner); ok {
		codex.ContextBudget = budget.Map()
		return codex.Run(ctx, text)
	}
	return runner.Run(ctx, text)
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
	return `First playable vertical slice: scene-first staging for the selected lorepack scenario.
The lorepack scenario notes are source-fidelity/background support for premise, protagonist role, opening pressure, and choice texture.
The seed Writer packages that authored scenario into durable story state and a compact playable packet; it does not invent a replacement premise.
Opening aim: player, desire, place, pressure, active opposition, and a choice that reveals character.
Reader plausibility means the unusual feels lived-in through internal cause, daily practice, emotional credibility, material consequence, and follow-through.
Rules, customs, or constraints should come from the chosen lore/source material or the current premise, not from generic house style.
The opening should introduce first trouble and branch momentum, not spend the story peak; story_arc and phase fields are optional guidance for later Writer refreshes.
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
		Title:   "The Orchard Pump",
		Premise: "A repair apprentice in a wind-lift orchard must keep a failing water engine alive before an ex-mentor sells its last working heart-valve.",
		PlayerCharacter: story.PlayerCharacter{
			PeerID:      "sans-nom",
			Name:        "Mara Kel",
			Description: "A tired pump apprentice who knows every cable-song in the hanging orchard but has never been trusted with the main valve.",
			Desire:      "Mara wants to save the orchard's night crop before the lower kitchens go dry.",
			Wound:       "She believes every adult promise is a delay tactic until someone smaller absorbs the failure.",
			Boundary:    "Mara will not let her work become someone else's quiet excuse.",
		},
		PlayerWorldConnection: "Mara grew up inside the pump rigging, so the orchard responds to her hands faster than it responds to official keys.",
		WorldBasics: []string{
			"The orchard hangs from storm towers, with fruit rows, kitchens, and pump rooms swaying on separate cable decks.",
			"Water is moved by singing pressure engines that answer to touch, pitch, and weather scars.",
			"Repair crews settle arguments by getting machinery working before the next gust tears a deck loose.",
		},
		StoryContext: []string{
			"Keep the opening practical: a repair job goes wrong in a specific place with a specific person blocking Mara.",
			"The strange setting should surface through tools, weather, work rhythm, and relationship pressure.",
		},
		Factions: []story.Faction{{
			ID:          "saffron-kitchens",
			Name:        "Saffron Kitchens",
			Description: "Lower-deck cooks who depend on the orchard pumps and know exactly which repairs are being postponed.",
		}, {
			ID:          "valve-brokers",
			Name:        "Valve Brokers",
			Description: "Traveling parts dealers who buy failing machinery cheap and resell survival at a premium.",
		}},
		CoreRelationships: []string{
			"Mara owes Kesh, her younger brother in the kitchens, a working pump by nightfall.",
			"Orro Vale, her ex-mentor, taught her the engine songs and now plans to sell the heart-valve.",
			"Jun Pike from the rival fruit row wants the same valve for a different failing deck.",
		},
		ToneGuidance: []string{
			"Windy, tactile science-fantasy with grease, fruit rot, cable creak, practical danger, and sharp personal history.",
			"Keep the scene legible as a repair crisis before it becomes mythic.",
			"Absurd machinery is allowed, but emotional consequence must stay precise.",
		},
		FirstTurnExposition: []string{
			"Establish the orchard deck, the pump failure, Mara's skill, Orro's pressure, and Kesh's need.",
			"Reach a playable choice before explaining the wider world.",
		},
		StoryArc: []string{
			"Opening: Mara chooses how to keep water moving while Orro tries to sell the heart-valve.",
			"After the opening: the chosen repair changes who controls the pump room.",
			"Middle pressure: weather, kitchens, rival decks, and old mentorship debts pull the repair in different directions.",
		},
		ResumeSummary: "Mara reaches the orchard pump room as the heart-valve coughs dry and Orro arrives with a buyer's crate.",
		Metadata: map[string]string{
			"seed_version":        "static-v1",
			"generated_at":        "static",
			"resume_point":        "pump_room",
			"current_checkpoint":  "opening pump failure",
			"continuation_hooks":  "Mara's repair choice changes who controls the heart-valve.",
			"canon_validated":     "true",
			"open_threads":        "Kesh's dry kitchens; Orro's buyer; Jun's rival deck; storm pressure",
			"gameplay_mode":       "static",
			"ground_lore_pack_id": "metabarons",
		},
	}
}

func DefaultPacket() scene.StoryPacket {
	return scene.StoryPacket{
		ID:          "storypacket_orchard_pump_opening_001",
		EntryNodeID: "pump_room",
		ContextSnapshot: scene.ContextSnapshot{
			WorkspaceID:   "static-seed",
			SessionID:     "main",
			Location:      "North Orchard pump room",
			Time:          "one storm gust before night irrigation",
			PresentActors: []string{},
		},
		Nodes: []scene.SceneNode{{
			ID:    "pump_room",
			Title: "The heart-valve coughs dry",
			Beats: []scene.Beat{
				{ID: "beat-world", Text: "The North Orchard deck tilts under a fresh gust, shaking green fruit into the pump-room gutters while the pressure engine coughs dry air instead of water."},
				{ID: "beat-player", Text: "Mara Kel slides under the main pipe with a wrench in her teeth and hears the heart-valve singing off-key."},
				{ID: "beat-opposition", Text: "Orro Vale arrives with a buyer's crate and tells Mara the valve is already sold; Jun Pike blocks the side hatch with a rival repair crew behind her."},
				{ID: "beat-choice", Text: "The lower kitchens need water before night service, but there is only one working valve in reach and three ways to ruin someone's plan."},
			},
			ActorNeeds: []scene.ActorNeed{},
			Choices: []scene.Choice{{
				ID:           "choice_lock_valve",
				Label:        "Lock the heart-valve into Mara's pump and dare Orro to stop the repair.",
				TargetNodeID: "lock_valve",
				PlayerIntent: "Claim agency by saving the kitchens now and openly breaking with Orro.",
			}, {
				ID:           "choice_split_flow",
				Label:        "Cut a risky split-flow hose so both decks get half-pressure.",
				TargetNodeID: "split_flow",
				PlayerIntent: "Risk Mara's skill and status to protect both crews from immediate failure.",
			}, {
				ID:           "choice_follow_buyer",
				Label:        "Let the pump fail for one minute and follow Orro's buyer into the storm lift.",
				TargetNodeID: "follow_buyer",
				PlayerIntent: "Risk the crop to learn who is buying the orchard's survival out from under them.",
			}},
			MemoryIntents: []scene.MemoryIntent{{
				ID:        "opening-arrival",
				Summary:   "Mara Kel reached the North Orchard pump room as Orro Vale tried to sell the heart-valve.",
				Policy:    scene.MemoryPolicyCanon,
				Witnesses: []string{"sans-nom"},
			}},
		}, {
			ID:         "lock_valve",
			Title:      "The pump takes Mara's side",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Mara slams the lock-pin home, the pump drinks the valve's song, and Orro's buyer crate snaps shut on empty air."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after Mara saves the kitchens and turns Orro into an active opponent.",
			},
		}, {
			ID:         "split_flow",
			Title:      "Half-pressure, full blame",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "The split-flow hose screams across both decks, saving the first water surge while marking Mara's repair as the next thing likely to burst."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after Mara buys time for both decks but creates a fragile shared failure.",
			},
		}, {
			ID:         "follow_buyer",
			Title:      "The storm lift opens",
			Beats:      []scene.Beat{{ID: "beat-1", Text: "Mara lets the pump cough once more and dives after the buyer's crate as the storm lift opens onto rain bright enough to sting."}},
			ActorNeeds: []scene.ActorNeed{},
			Choices:    []scene.Choice{},
			RevisionNotes: []string{
				"Writer continuation is required after Mara risks the crop to learn who is buying the orchard's last working part.",
			},
		}},
		RevisionNotes: []string{"Compact opening seed for CLI playtest; no actor_needs yet.", "Entry choices branch into included terminal handoff nodes with Writer continuation notes."},
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
