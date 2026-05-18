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
		"own the StoryBible and StoryPacket",
		"Do not delegate final story design",
		"The seed workshop already happened",
		"Read workshop and scenarist files selectively",
		"Use ask_lore_expert when the question depends on the selected ground-lore pack",
		"reference research",
		"original binding doctrine",
		"support element for character conflict",
		"social practice, cost, emotional credibility, and consequence",
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
}

func TestSeedWorkshopPromptIsFreeFormAndNonSchema(t *testing.T) {
	got := SeedWorkshopPrompt("seed context", "Imagination Turn", "Invent germs.", "No workshop artifacts yet.")
	for _, required := range []string{
		"Output Markdown only",
		"Do not output JSON",
		"Do not write a final StoryBible",
		"Do not write a final StoryPacket",
		"Do not use schema field names or target_node_id values",
		"Protect reader plausibility",
		"internal cause, cost, emotional credibility, and consequence",
		"material imagination",
		"Use call_scenarists as the preferred creative workshop",
		"story heat",
		"Use one call for independent lenses",
		"later call after reading returned files",
		"ask Scenarists first",
		"ask_lore_expert",
		"Treat doctrine as support for dramatic conflict",
		"Invent germs.",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("workshop prompt missing %q:\n%s", required, got)
		}
	}
}

func TestWriterPromptRequiresClosedPacketTargets(t *testing.T) {
	got, err := WriterPrompt("writer context", "choice_refuse_publicly")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Every choice target_node_id must point to a scene node included in this same packet",
		"node_future_*",
		"terminal node with no choices and revision_notes",
		"internal cause, social enforcement, cost, emotional credibility, and consequence",
		"Use call_scenarists as the default outside workshop",
		"reader-plausibility criticism",
		"Requests inside one call run in parallel and do not see each other's answers",
		"for dependent follow-up, read the returned files first",
		"ask Scenarists first",
		"ask_lore_expert",
		"validator-closed StoryPacket",
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
		"original binding doctrine",
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
