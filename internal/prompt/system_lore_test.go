package prompt

import (
	"strings"
	"testing"
)

func TestWithMasterLoreSystemPromptInjectsStableLoreBlock(t *testing.T) {
	got := WithMasterLoreSystemPrompt("Base system.", "metabarons", "Metabarons", "# Lore\n\nDynasty law.")
	for _, required := range []string{
		"Base system.",
		"ACTIVE LOREPACK MASTER SUMMARY",
		"Pack: Metabarons (metabarons)",
		"Dynasty law.",
		"END ACTIVE LOREPACK MASTER SUMMARY",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("system prompt missing %q:\n%s", required, got)
		}
	}
}

func TestWithMasterLoreSystemPromptUsesDefaultBase(t *testing.T) {
	got := WithMasterLoreSystemPrompt("", "", "", "Lore body.")
	if !strings.Contains(got, defaultStoryAgentSystemPrompt) || !strings.Contains(got, "Lore body.") {
		t.Fatalf("unexpected default system prompt:\n%s", got)
	}
}

func TestWithMasterLoreSystemPromptSkipsEmptyLore(t *testing.T) {
	if got := WithMasterLoreSystemPrompt("Base", "pack", "Pack", "  "); got != "Base" {
		t.Fatalf("empty lore should preserve base, got %q", got)
	}
}
