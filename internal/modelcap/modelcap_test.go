package modelcap

import (
	"testing"

	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

func TestLookupExactOpenAI(t *testing.T) {
	cap, ok := Lookup(modelmeta.Metadata{
		ProviderType:  "codex_oauth",
		ModelProvider: "openai",
		Model:         "gpt-5.5",
	})
	if !ok {
		t.Fatal("expected exact OpenAI capability")
	}
	if cap.ContextWindowTokens != 1050000 || cap.MaxOutputTokens != 128000 {
		t.Fatalf("unexpected cap: %#v", cap)
	}
	if !cap.SupportsTools || !cap.SupportsJSONSchema {
		t.Fatalf("OpenAI cap should support tools and structured outputs: %#v", cap)
	}
}

func TestLookupFamilyAlias(t *testing.T) {
	cap, ok := Lookup(modelmeta.Metadata{
		ProviderType:  "claude_compatible",
		ModelProvider: "zai",
		Model:         "GLM-4.5-Air-20260501",
	})
	if !ok {
		t.Fatal("expected alias capability")
	}
	if cap.Model != "glm-4.5-air" || cap.ContextWindowTokens != 128000 || cap.MaxOutputTokens != 96000 {
		t.Fatalf("unexpected alias cap: %#v", cap)
	}
}

func TestUnknownFallbackBudgetWarns(t *testing.T) {
	budget := BudgetFor(Request{
		Metadata: modelmeta.Metadata{
			ProviderType:  "claude_compatible",
			ModelProvider: "ACME",
			Model:         "unknown-model",
		},
		Role:          "narrator",
		HasTools:      true,
		HasJSONSchema: true,
		MaxToolRounds: 2,
	})
	if !budget.UnknownCapability {
		t.Fatalf("expected unknown fallback: %#v", budget)
	}
	if budget.Capability.ContextWindowTokens != defaultContextWindowTokens {
		t.Fatalf("unexpected fallback context: %#v", budget)
	}
	if !hasWarning(budget.Warnings, "unknown model capability; using conservative fallback budget") {
		t.Fatalf("missing fallback warning: %#v", budget.Warnings)
	}
}

func TestProviderPoolUsesMostRestrictiveKnownCapability(t *testing.T) {
	cap, ok := Lookup(modelmeta.Metadata{
		ProviderType:  "claude_compatible_pool",
		ModelProvider: "ZAI,XIAOMI",
		Model:         "glm-4.5-air,mimo-v2.5-pro",
	})
	if !ok {
		t.Fatal("expected known provider pool capability")
	}
	if cap.ContextWindowTokens != 128000 {
		t.Fatalf("pool should use most restrictive context window, got %#v", cap)
	}
	if cap.ProviderType != "claude_compatible_pool" {
		t.Fatalf("pool provider type not preserved: %#v", cap)
	}
}

func TestBudgetUsesRoleReserveAndOverheads(t *testing.T) {
	budget := BudgetFor(Request{
		Metadata: modelmeta.Metadata{
			ProviderType:  "claude_compatible",
			ModelProvider: "MINIMAX",
			Model:         "MiniMax-M2.7-highspeed",
		},
		Role:          "writer",
		HasTools:      true,
		HasJSONSchema: false,
		MaxToolRounds: 3,
	})
	want := 204800 - 32768 - (3 * defaultToolRoundOverhead) - defaultReasoningReserveTokens - defaultSafetyMarginTokens
	if budget.OutputReserveTokens != 32768 || budget.InputBudgetTokens != want {
		t.Fatalf("unexpected budget: got %#v want input %d", budget, want)
	}
	if budget.Capability.MaxOutputTokens != 0 {
		t.Fatalf("MiniMax M2.7 max output should stay provider-ambiguous: %#v", budget.Capability)
	}
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}
