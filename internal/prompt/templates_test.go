package prompt

import (
	"strings"
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

func TestSeedWriterPromptOwnsStructuredSeed(t *testing.T) {
	got := SeedWriterPrompt("seed context")
	if !strings.Contains(got, "You are the Creative Writer") {
		t.Fatalf("seed prompt should name Creative Writer role:\n%s", got)
	}
	if strings.Contains(got, "You are the Scenarist") {
		t.Fatalf("seed prompt should not assign seed writing to Scenarist:\n%s", got)
	}
	for _, required := range []string{
		"selected lorepack scenario",
		"Stage the authored lorepack scenario notes into a playable opening",
		"support, not a fixed one-scene template",
		"freshness comes from staging",
		"This is a game",
		"player character at the dramatic center",
		"entertaining to play",
		"Preserve the protagonist role, opening situation, and initial pressure",
		"not mandatory dramatic rails",
		"Opening aim: player, desire, place, pressure, active opposition, and a choice that reveals character",
		"own the StoryBible and StoryPacket",
		"Use ask_lore_expert when fidelity to the selected lorepack is uncertain",
		"lore reference support",
		"Before drafting any StoryBible or StoryPacket, do a lore orientation pass",
		"read lorepack/navigation.md first",
		"then lorepack/lore-pack.json",
		"Follow the minimum orientation from lorepack/navigation.md",
		"at least two relevant volume summaries",
		"at least one matching character file",
		"major characters, premise pressures, decisive events, and choice texture",
		"Use LS/Grep/Read on lorepack/ deliberately",
		"lorepack/wiki/lore.md",
		"lorepack/comics/volumes/volume-XX.json",
		"lorepack/comics/characters/characters-volume-XX.json",
		"lorepack/comics/pages/<page_id>.json",
		"lorepack/index/manifest.json",
		"lorepack/comics/manifest.json",
		"Use the Grep tool or rg-style searches, not shell grep",
		"build a compact mental model before writing JSON",
		"story_context",
		"tone_guidance",
		"Populate packet.metadata with string values",
		"separate multiple motifs with semicolons",
		"story_arc and phase fields are optional guidance",
		"current_arc_phase",
		"spent_motifs",
		"not spend the story's peak",
		"cause, social practice, emotional credibility, material consequence, and follow-through",
		"usually 2-5 scene nodes",
		"real playable choice point",
		"Terminal handoff nodes",
		"Every choice target_node_id must point to a scene node included in this same packet",
		"Do not use placeholder targets",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("seed prompt missing %q:\n%s", required, got)
		}
	}
	for _, forbidden := range []string{
		"Choose the strongest story seed",
		"creative DNA",
		"Use new characters, new places",
		"Do not use Metabarons canon names",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("seed prompt kept premise-invention wording %q:\n%s", forbidden, got)
		}
	}
}

func TestSeedWriterRepairPromptDoesNotRedoFullOrientation(t *testing.T) {
	got := SeedWriterRepairPrompt("seed context")
	for _, required := range []string{
		"This is a repair pass after validation/review feedback",
		"Do not redo the full lore orientation pass from scratch",
		"Start from the previous raw Writer output",
		"Use staged lorepack reads or ask_lore_expert only for targeted checks",
		"Prefer one focused search/read over repeating the full navigation sequence",
		"Return only JSON matching the SeedBundle shape",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("repair prompt missing %q:\n%s", required, got)
		}
	}
	for _, forbidden := range []string{
		"Before drafting any StoryBible or StoryPacket, do a lore orientation pass",
		"Follow the minimum orientation from lorepack/navigation.md",
		"at least two relevant volume summaries",
		"at least one matching character file",
		"build a compact mental model before writing JSON",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("repair prompt kept full-orientation wording %q:\n%s", forbidden, got)
		}
	}
}

func TestLegacySeedWorkshopPromptIsFreeFormAndNonSchema(t *testing.T) {
	got := SeedWorkshopPrompt("seed context", "Staging Turn", "Stress-test the opening.", "No workshop artifacts yet.")
	for _, required := range []string{
		"legacy Sans-Nom seed staging workshop",
		"existing lorepack scenario, not premise invention",
		"Preserve the lorepack scenario notes",
		"playable staging, scene pressure",
		"what changes, who can respond, and what consequence follows",
		"Output Markdown only",
		"Stay free-form",
		"no JSON, schema field names, node ids, final bundle, or player-facing prose",
		"Use ask_scenarist for sparks, critique, alternatives",
		"Ask one Scenarist for one advisory job",
		"read the returned file",
		"ask_lore_expert",
		"Stress-test the opening.",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("workshop prompt missing %q:\n%s", required, got)
		}
	}
	for _, forbidden := range []string{
		"Return a durable StoryBible",
		"StoryBible",
		"StoryPacket",
		"The seed packet may use any clear node ids",
		"Every choice target must be an included node",
		"target_node_id",
		"craft DNA",
		"generated story original",
		"new names, places",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("workshop prompt leaked final seed contract %q:\n%s", forbidden, got)
		}
	}
}

func TestWriterPromptRequiresClosedPacketTargets(t *testing.T) {
	got, err := WriterPrompt("writer context", "choice_refuse_publicly")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"latest player decision",
		"immediate consequence",
		"next playable pressure",
		"This is a game",
		"player character at the dramatic center",
		"entertaining to play",
		"give the player agency",
		"Use lorepack scenario notes",
		"Advance the story phase",
		"Use STORY PHASE GUIDANCE",
		"spent",
		"Do not default to clock/minute countdowns",
		"story-native thresholds",
		"Reflect-style choice",
		"Populate packet.metadata",
		"current_arc_phase",
		"next_arc_phase",
		"spent_motifs",
		"do not list core vocabulary just because it appeared in prose",
		"Do not replay, re-adjudicate, or rename the same bargain",
		"Every choice target_node_id must point to a scene node included in this same packet",
		"node_future_*",
		"terminal node with no choices and revision_notes",
		"Terminal handoffs are not dead ends",
		"cause, daily practice, human cost, and visible consequence",
		"Actor agents are available only for ids listed in the assembled STORY ACTOR MANIFEST",
		"Actors are not background flavor",
		"cast them with actor_needs",
		"Director rehearsal",
		"no actor rehearsal reason",
		"assembled Writer context below is the authoritative continuation brief",
		"Do not re-read staged story files",
		"continuation refresh, not seed generation",
		"Do not perform the lorepack minimum orientation pass",
		"do not read lorepack/navigation.md as a default step",
		"perform one narrow grounding read before drafting",
		"read story/current-node.json",
		"prefer 1-4 targeted tool calls total",
		"Do not run broad inventory commands",
		"read one named story file or one named lorepack source file",
		"Prefer one focused rg/read or ask_lore_expert",
		"Use ask_scenarist for sparks",
		"Ask one clear advisory question",
		"dependent follow-up with that path in relevant_paths",
		"Use ask_actor when a listed story actor's will",
		"Actor material is planning-only",
		"scenario fidelity is uncertain",
		"ask_lore_expert",
		"lorepack/",
		"You own the final packet shape",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("writer prompt missing %q:\n%s", required, got)
		}
	}
}

func TestWriterPromptDoesNotRepeatSeedLoreOrientation(t *testing.T) {
	got, err := WriterPrompt("writer context", "choice_refuse_publicly")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"read it before searching the lorepack",
		"Follow the minimum orientation from lorepack/navigation.md",
		"at least two relevant volume summaries",
		"at least one matching character file",
		"build a compact mental model before writing JSON",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("writer prompt kept seed-orientation wording %q:\n%s", forbidden, got)
		}
	}
}

func TestNarratorPromptUsesLoreFidelityLanguage(t *testing.T) {
	got := NarratorPrompt(scene.SceneNode{ID: "node", Beats: nil}, "[]", "LOREPACK SCENARIO NOTES\nscenario_notes")
	for _, required := range []string{
		"This is a game",
		"player character at the center",
		"entertaining to play",
		"player agency",
		"Director rehearsal bundle",
		"Use lorepack scenario notes",
		"reinforce current pressure and consequences",
		"Use provided lore context first",
		"perform one narrow grounding read",
		"story/current-node.json",
		"2-6 readable paragraphs",
		"separated by blank lines",
		"Avoid one-block walls of text",
		"avoid restating the same lore result",
		"For branch continuations",
		"real scene weight",
		"Do not compress it into a mere transition summary",
		"moral question",
		"answerable by the player",
		"lorepack/navigation.md",
		"lorepack/contract.md",
		"lorepack/relevant-excerpts.md",
		"lorepack/source-anchors.md",
		"not to dump unused canon or force a generic theme",
		"Director rehearsal bundle",
		"If rehearsal actor_outputs is [], surfaced_actor_messages must be []",
		"Director rehearsal material is not canon by itself",
		"Do not let actor performance steal protagonism",
		"curated_card_updates",
		"LOREPACK SCENARIO NOTES",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("narrator prompt missing %q:\n%s", required, got)
		}
	}
}

func TestSeedVerifierRejectsPlaceholderContinuationTargets(t *testing.T) {
	got := SeedVerifierPrompt(`{"bible":{},"packet":{}}`, "seed context")
	for _, required := range []string{
		"any choice targets a missing node",
		"any choice uses a placeholder or sentinel target",
		"follow-up choices whose target nodes are missing",
		"lorepack scenario notes",
		"replacement central premise",
		"avoid-drift notes",
		"source-fidelity notes",
		"player pressure",
		"spends the apparent climax immediately",
		"lore procedure is the dominant dramatic subject",
		"real playable choice point",
		"mostly system explanation",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("seed verifier prompt missing %q:\n%s", required, got)
		}
	}
}
