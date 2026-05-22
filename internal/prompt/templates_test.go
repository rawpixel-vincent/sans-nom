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
		"Stage the authored lorepack scenario contract into a playable opening",
		"pressure field, not a fixed one-scene template",
		"freshness comes from staging",
		"Preserve the protagonist role, opening situation, initial pressure, must_honor rules, and choice_principles",
		"Opening aim: player, desire, place, pressure, active opposition, and a choice that reveals character",
		"own the StoryBible and StoryPacket",
		"Use ask_lore_expert when fidelity to the selected lorepack is uncertain",
		"lore authority/reference support",
		"Before drafting any StoryBible or StoryPacket, do a lore orientation pass",
		"read lorepack/navigation.md first",
		"then lorepack/lore-pack.json",
		"Follow the minimum orientation from lorepack/navigation.md",
		"at least two relevant volume summaries",
		"at least one matching character file",
		"major characters, family/story pressures, repeated laws, decisive events, and choice logic",
		"Use LS/Grep/Read on lorepack/ deliberately",
		"lorepack/wiki/lore.md",
		"lorepack/comics/volumes/volume-XX.json",
		"lorepack/comics/characters/characters-volume-XX.json",
		"lorepack/comics/pages/<page_id>.json",
		"lorepack/index/manifest.json",
		"lorepack/comics/manifest.json",
		"Use the Grep tool or rg-style searches, not shell grep",
		"build a compact mental model before writing JSON",
		"lore_principles that bind the scenario pressure",
		"Populate packet.metadata with string values",
		"separate multiple motifs with semicolons",
		"The StoryBible must include story_arc",
		"4-7 concrete phase steps",
		"current_arc_phase",
		"spent_motifs",
		"not spend the story's peak",
		"cause, social practice, cost, emotional credibility, and consequence",
		"exactly 5 scene nodes",
		"three-turn runway",
		"turn_1_intro",
		"turn_2_decision",
		"exactly 3 meaningful choices",
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
		"Preserve the lorepack scenario contract",
		"playable staging, scene pressure",
		"what changes, who witnesses it, and what cost follows",
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
		"The seed packet is a three-turn runway",
		"Every choice target must be an included node",
		"turn_1_intro",
		"turn_2_decision",
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
		"Honor any lorepack scenario contract",
		"Advance the story phase",
		"Obey STORY PHASE GUIDANCE",
		"spent",
		"Populate packet.metadata",
		"current_arc_phase",
		"next_arc_phase",
		"spent_motifs",
		"do not list core vocabulary just because it appeared in prose",
		"Do not replay, re-adjudicate, or rename the same proof",
		"Every choice target_node_id must point to a scene node included in this same packet",
		"node_future_*",
		"terminal node with no choices and revision_notes",
		"cause, daily practice, human cost, and visible consequence",
		"Actor agents are available only for ids listed in the assembled STORY ACTOR MANIFEST",
		"assembled Writer context below is the authoritative continuation brief",
		"Do not re-read staged story files",
		"continuation refresh, not seed generation",
		"Do not perform the lorepack minimum orientation pass",
		"do not read lorepack/navigation.md as a default step",
		"prefer 0-4 targeted tool calls",
		"Do not run broad inventory commands",
		"read one named story file or one named lorepack source file",
		"Prefer one focused rg/read or ask_lore_expert",
		"Use ask_scenarist for sparks",
		"Ask one clear advisory question",
		"dependent follow-up with that path in relevant_paths",
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
	got := NarratorPrompt(scene.SceneNode{ID: "node", Beats: nil}, "[]", "LOREPACK SCENARIO CONTRACT\nmust_honor")
	for _, required := range []string{
		"Honor any lorepack scenario contract",
		"reinforce its pressure and consequences",
		"Use provided lore context first",
		"2-4 readable paragraphs",
		"separated by blank lines",
		"Avoid one-block walls of text",
		"avoid restating the same lore result",
		"lorepack/navigation.md",
		"lorepack/contract.md",
		"lorepack/relevant-excerpts.md",
		"lorepack/source-anchors.md",
		"Use lore references to preserve fidelity to the active scenario",
		"If actor raw material is [], surfaced_actor_messages must be []",
		"LOREPACK SCENARIO CONTRACT",
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
		"lorepack scenario contract",
		"replacement central premise",
		"must_honor",
		"must_not_violate",
		"choice_principles",
		"binding lore principles",
		"story_arc",
		"spends the apparent climax immediately",
		"law/doctrine is the dominant dramatic subject",
		"turn_1_intro",
		"turn_2_decision",
		"exactly 3 meaningful choices",
		"mostly system explanation",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("seed verifier prompt missing %q:\n%s", required, got)
		}
	}
}
