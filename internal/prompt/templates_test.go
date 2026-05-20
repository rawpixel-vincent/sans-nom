package prompt

import (
	"strings"
	"testing"
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
		"binding lore principles in original_laws",
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

func TestSeedWorkshopPromptIsFreeFormAndNonSchema(t *testing.T) {
	got := SeedWorkshopPrompt("seed context", "Imagination Turn", "Invent germs.", "No workshop artifacts yet.")
	for _, required := range []string{
		"creative discovery",
		"concrete situation already in motion",
		"person has a practical task",
		"playable scenes with clear stakes",
		"Use plain language and low lore density",
		"work, infrastructure, ecology, transport, weather",
		"Output Markdown only",
		"Stay free-form",
		"no JSON, schema field names, node ids, final bundle, or player-facing prose",
		"Use ask_scenarist for sparks, critique, alternatives",
		"Ask one Scenarist for one advisory job",
		"read the returned file",
		"ask_lore_expert",
		"Invent germs.",
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
		"Every choice target_node_id must point to a scene node included in this same packet",
		"node_future_*",
		"terminal node with no choices and revision_notes",
		"cause, daily practice, human cost, and visible consequence",
		"Actor agents are available only for ids listed in the assembled STORY ACTOR MANIFEST",
		"Use ask_scenarist for sparks",
		"Ask one clear advisory question",
		"dependent follow-up with that path in relevant_paths",
		"ask_lore_expert",
		"You own the final packet shape",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("writer prompt missing %q:\n%s", required, got)
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
