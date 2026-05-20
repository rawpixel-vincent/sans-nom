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
		"Stage the authored lorepack scenario contract",
		"Do not replace its central premise",
		"Preserve the protagonist role, opening situation, initial pressure, must_honor rules, and choice_principles",
		"Opening aim: player, desire, place, pressure, active opposition, and a choice that reveals character",
		"own the StoryBible and StoryPacket",
		"Use ask_lore_expert when fidelity to the selected lorepack is uncertain",
		"lore authority/reference support",
		"lore_principles that bind the scenario pressure",
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
		"Every choice target_node_id must point to a scene node included in this same packet",
		"node_future_*",
		"terminal node with no choices and revision_notes",
		"cause, daily practice, human cost, and visible consequence",
		"Actor agents are available only for ids listed in the assembled STORY ACTOR MANIFEST",
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

func TestNarratorPromptUsesLoreFidelityLanguage(t *testing.T) {
	got := NarratorPrompt(scene.SceneNode{ID: "node", Beats: nil}, "[]", "LOREPACK SCENARIO CONTRACT\nmust_honor")
	for _, required := range []string{
		"Honor any lorepack scenario contract",
		"reinforce its pressure and consequences",
		"Use lore references to preserve fidelity to the active scenario",
		"curated lorepack/ files",
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
