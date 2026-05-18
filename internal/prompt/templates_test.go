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
		"Use call_scenarists as the seed writer's room",
		"Use ask_lore_expert when the question depends on the selected ground-lore pack",
		"ask Scenarists first",
		"can run up to the provider-pool size in parallel",
		"Scenarist material is brainstorming material",
		"reference research",
		"original binding doctrine",
		"player character's world",
		"social practice, cost, taboo, and consequence",
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
		"internal cause, social enforcement, cost, taboo, emotional credibility, and consequence",
		"material imagination",
		"Use call_scenarists as the preferred creative workshop",
		"ask Scenarists first",
		"ask_lore_expert",
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
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("seed verifier prompt missing %q:\n%s", required, got)
		}
	}
}
