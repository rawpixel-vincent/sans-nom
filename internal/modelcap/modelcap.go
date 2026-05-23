package modelcap

import (
	"math"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
)

const (
	confidenceDocumented = "documented"
	confidenceInferred   = "inferred"
	confidenceFallback   = "fallback"

	defaultContextWindowTokens    = 128000
	defaultToolRoundOverhead      = 3000
	defaultSchemaOverheadTokens   = 2000
	defaultReasoningReserveTokens = 8000
	defaultSafetyMarginTokens     = 4000
)

type Capability struct {
	ProviderType            string   `json:"provider_type,omitempty"`
	ModelProvider           string   `json:"model_provider,omitempty"`
	Model                   string   `json:"model,omitempty"`
	ContextWindowTokens     int      `json:"context_window_tokens"`
	MaxOutputTokens         int      `json:"max_output_tokens,omitempty"`
	SupportsTools           bool     `json:"supports_tools,omitempty"`
	SupportsJSONSchema      bool     `json:"supports_json_schema,omitempty"`
	PreferredAPI            string   `json:"preferred_api,omitempty"`
	ToolRoundOverheadTokens int      `json:"tool_round_overhead_tokens,omitempty"`
	SchemaOverheadTokens    int      `json:"schema_overhead_tokens,omitempty"`
	ReasoningReserveTokens  int      `json:"reasoning_reserve_tokens,omitempty"`
	Source                  string   `json:"source,omitempty"`
	Confidence              string   `json:"confidence,omitempty"`
	Notes                   []string `json:"notes,omitempty"`
}

type Request struct {
	Metadata      modelmeta.Metadata
	Role          string
	HasTools      bool
	HasJSONSchema bool
	MaxToolRounds int
}

type Budget struct {
	Capability             Capability `json:"capability"`
	Role                   string     `json:"role"`
	EstimatedPromptTokens  int        `json:"estimated_prompt_tokens,omitempty"`
	InputBudgetTokens      int        `json:"input_budget_tokens"`
	OutputReserveTokens    int        `json:"output_reserve_tokens"`
	SchemaOverheadTokens   int        `json:"schema_overhead_tokens,omitempty"`
	ToolOverheadTokens     int        `json:"tool_overhead_tokens,omitempty"`
	ReasoningReserveTokens int        `json:"reasoning_reserve_tokens,omitempty"`
	SafetyMarginTokens     int        `json:"safety_margin_tokens"`
	UnknownCapability      bool       `json:"unknown_capability,omitempty"`
	Warnings               []string   `json:"warnings,omitempty"`
}

func Lookup(metadata modelmeta.Metadata) (Capability, bool) {
	metadata = normalizeMetadata(metadata)
	if strings.EqualFold(metadata.ProviderType, "claude_compatible_pool") {
		return lookupPool(metadata)
	}
	if cap, ok := lookupExact(metadata); ok {
		return cap, true
	}
	if cap, ok := lookupAlias(metadata); ok {
		return cap, true
	}
	return fallback(metadata), false
}

func BudgetFor(req Request) Budget {
	cap, ok := Lookup(req.Metadata)
	role := normalizeRole(req.Role)
	outputReserve := roleOutputReserve(role)
	if cap.MaxOutputTokens > 0 && outputReserve > cap.MaxOutputTokens {
		outputReserve = cap.MaxOutputTokens
	}
	schemaOverhead := 0
	if req.HasJSONSchema {
		schemaOverhead = firstPositive(cap.SchemaOverheadTokens, defaultSchemaOverheadTokens)
	}
	toolOverhead := 0
	if req.HasTools {
		rounds := req.MaxToolRounds
		if rounds <= 0 {
			rounds = 1
		}
		toolOverhead = rounds * firstPositive(cap.ToolRoundOverheadTokens, defaultToolRoundOverhead)
	}
	reasoningReserve := firstPositive(cap.ReasoningReserveTokens, defaultReasoningReserveTokens)
	safety := defaultSafetyMarginTokens
	inputBudget := cap.ContextWindowTokens - outputReserve - schemaOverhead - toolOverhead - reasoningReserve - safety
	if inputBudget < 1024 {
		inputBudget = 1024
	}
	warnings := append([]string(nil), cap.Notes...)
	if !ok {
		warnings = append(warnings, "unknown model capability; using conservative fallback budget")
	}
	return Budget{
		Capability:             cap,
		Role:                   role,
		InputBudgetTokens:      inputBudget,
		OutputReserveTokens:    outputReserve,
		SchemaOverheadTokens:   schemaOverhead,
		ToolOverheadTokens:     toolOverhead,
		ReasoningReserveTokens: reasoningReserve,
		SafetyMarginTokens:     safety,
		UnknownCapability:      !ok,
		Warnings:               uniqueStrings(warnings),
	}
}

func EstimateTokens(text string) int {
	runes := len([]rune(text))
	if runes == 0 {
		return 0
	}
	return int(math.Ceil(float64(runes) / 4.0))
}

func (b Budget) WithEstimate(text string) Budget {
	b.EstimatedPromptTokens = EstimateTokens(text)
	if b.EstimatedPromptTokens > b.InputBudgetTokens {
		b.Warnings = appendStable(b.Warnings, "estimated prompt tokens exceed input budget")
	}
	return b
}

func (b Budget) Map() map[string]any {
	return map[string]any{
		"capability":               b.Capability,
		"role":                     b.Role,
		"estimated_prompt_tokens":  b.EstimatedPromptTokens,
		"input_budget_tokens":      b.InputBudgetTokens,
		"output_reserve_tokens":    b.OutputReserveTokens,
		"schema_overhead_tokens":   b.SchemaOverheadTokens,
		"tool_overhead_tokens":     b.ToolOverheadTokens,
		"reasoning_reserve_tokens": b.ReasoningReserveTokens,
		"safety_margin_tokens":     b.SafetyMarginTokens,
		"unknown_capability":       b.UnknownCapability,
		"warnings":                 b.Warnings,
	}
}

func lookupExact(metadata modelmeta.Metadata) (Capability, bool) {
	key := registryKey(metadata.ProviderType, metadata.ModelProvider, metadata.Model)
	cap, ok := registry()[key]
	return cap, ok
}

func lookupAlias(metadata modelmeta.Metadata) (Capability, bool) {
	model := strings.ToLower(strings.TrimSpace(metadata.Model))
	provider := strings.ToUpper(strings.TrimSpace(metadata.ModelProvider))
	switch {
	case strings.HasPrefix(model, "gpt-5.5-pro"):
		return mustCapability("codex_oauth", "openai", "gpt-5.5-pro"), true
	case strings.HasPrefix(model, "gpt-5.5"):
		return mustCapability("codex_oauth", "openai", "gpt-5.5"), true
	case strings.HasPrefix(model, "gpt-5.4"):
		return mustCapability("codex_oauth", "openai", "gpt-5.4"), true
	case strings.HasPrefix(model, "gpt-5.3-codex"):
		return mustCapability("codex_oauth", "openai", "gpt-5.3-codex"), true
	case strings.HasPrefix(model, "gpt-5-codex"):
		return mustCapability("codex_oauth", "openai", "gpt-5-codex"), true
	case provider == "ZAI" && strings.HasPrefix(model, "glm-5.1"):
		return mustCapability("claude_compatible", "ZAI", "glm-5.1"), true
	case provider == "ZAI" && strings.HasPrefix(model, "glm-5-turbo"):
		return mustCapability("claude_compatible", "ZAI", "glm-5-turbo"), true
	case provider == "ZAI" && strings.HasPrefix(model, "glm-4.7"):
		return mustCapability("claude_compatible", "ZAI", "glm-4.7"), true
	case provider == "ZAI" && strings.HasPrefix(model, "glm-4.5-air"):
		return mustCapability("claude_compatible", "ZAI", "glm-4.5-air"), true
	case provider == "ZAI" && strings.HasPrefix(model, "glm-4.5"):
		return mustCapability("claude_compatible", "ZAI", "glm-4.5"), true
	case provider == "MINIMAX" && strings.HasPrefix(model, "minimax-m2.7-highspeed"):
		return mustCapability("claude_compatible", "MINIMAX", "MiniMax-M2.7-highspeed"), true
	case provider == "MINIMAX" && strings.HasPrefix(model, "minimax-m2.7"):
		return mustCapability("claude_compatible", "MINIMAX", "MiniMax-M2.7"), true
	case provider == "MINIMAX" && strings.HasPrefix(model, "minimax-m2.5-highspeed"):
		return mustCapability("claude_compatible", "MINIMAX", "MiniMax-M2.5-highspeed"), true
	case provider == "MINIMAX" && strings.HasPrefix(model, "minimax-m2.5"):
		return mustCapability("claude_compatible", "MINIMAX", "MiniMax-M2.5"), true
	case provider == "XIAOMI" && strings.HasPrefix(model, "mimo-v2.5-pro"):
		return mustCapability("claude_compatible", "XIAOMI", "mimo-v2.5-pro"), true
	case provider == "XIAOMI" && strings.HasPrefix(model, "mimo-v2.5"):
		return mustCapability("claude_compatible", "XIAOMI", "mimo-v2.5"), true
	}
	return Capability{}, false
}

func lookupPool(metadata modelmeta.Metadata) (Capability, bool) {
	providers := splitList(metadata.ModelProvider)
	models := splitList(metadata.Model)
	var caps []Capability
	for _, provider := range providers {
		for _, model := range models {
			if cap, ok := Lookup(modelmeta.Metadata{
				ProviderType:  "claude_compatible",
				ModelProvider: provider,
				Model:         model,
			}); ok {
				caps = append(caps, cap)
			}
		}
	}
	if len(caps) == 0 {
		return fallback(metadata), false
	}
	sort.SliceStable(caps, func(i, j int) bool {
		return caps[i].ContextWindowTokens < caps[j].ContextWindowTokens
	})
	cap := caps[0]
	cap.ProviderType = "claude_compatible_pool"
	cap.ModelProvider = strings.Join(providers, ",")
	cap.Model = strings.Join(models, ",")
	cap.Confidence = confidenceDocumented
	cap.Notes = append(cap.Notes, "provider pool budget uses the most restrictive known model capability before slot selection")
	return cap, true
}

func registry() map[string]Capability {
	base := []Capability{
		openAI("gpt-5.5", 1050000, 128000, "OpenAI model comparison docs"),
		openAI("gpt-5.5-pro", 1050000, 128000, "OpenAI model comparison docs"),
		openAI("gpt-5.4", 1050000, 128000, "OpenAI model comparison docs"),
		openAI("gpt-5.3-codex", 400000, 128000, "OpenAI GPT-5.3-Codex docs"),
		openAIWithConfidence("gpt-5-codex", 400000, 128000, "OpenAI GPT-5-Codex family docs", confidenceInferred, []string{"gpt-5-codex is treated as a family alias for the documented Codex context budget"}),
		claudeCompatible("ZAI", "glm-5.1", 200000, 128000, true, true, "Z.AI GLM-5.1 docs", nil),
		claudeCompatible("ZAI", "glm-5-turbo", 200000, 128000, true, true, "Z.AI GLM-5-Turbo docs", nil),
		claudeCompatible("ZAI", "glm-4.7", 200000, 128000, true, true, "Z.AI GLM-4.7 docs", nil),
		claudeCompatible("ZAI", "glm-4.5", 128000, 96000, true, true, "Z.AI GLM-4.5 docs", nil),
		claudeCompatible("ZAI", "glm-4.5-air", 128000, 96000, true, true, "Z.AI GLM-4.5 docs", nil),
		claudeCompatible("MINIMAX", "MiniMax-M2.7", 204800, 0, true, false, "MiniMax text generation docs", miniMaxNotes("M2.7")),
		claudeCompatible("MINIMAX", "MiniMax-M2.7-highspeed", 204800, 0, true, false, "MiniMax text generation docs", miniMaxNotes("M2.7-highspeed")),
		claudeCompatible("MINIMAX", "MiniMax-M2.5", 204800, 0, true, false, "MiniMax text generation docs", miniMaxNotes("M2.5")),
		claudeCompatible("MINIMAX", "MiniMax-M2.5-highspeed", 204800, 0, true, false, "MiniMax text generation docs", miniMaxNotes("M2.5-highspeed")),
		claudeCompatible("XIAOMI", "mimo-v2.5", 1000000, 128000, true, true, "MiMo model table and pricing docs", nil),
		claudeCompatible("XIAOMI", "mimo-v2.5-pro", 1000000, 128000, true, true, "MiMo model table and pricing docs", nil),
	}
	out := make(map[string]Capability, len(base)*2)
	for _, cap := range base {
		out[registryKey(cap.ProviderType, cap.ModelProvider, cap.Model)] = cap
		if cap.ProviderType == "codex_oauth" {
			compat := cap
			compat.ProviderType = "openai"
			out[registryKey(compat.ProviderType, compat.ModelProvider, compat.Model)] = compat
		}
	}
	return out
}

func openAI(model string, contextWindow int, maxOutput int, source string) Capability {
	return openAIWithConfidence(model, contextWindow, maxOutput, source, confidenceDocumented, nil)
}

func openAIWithConfidence(model string, contextWindow int, maxOutput int, source string, confidence string, notes []string) Capability {
	return Capability{
		ProviderType:            "codex_oauth",
		ModelProvider:           "openai",
		Model:                   model,
		ContextWindowTokens:     contextWindow,
		MaxOutputTokens:         maxOutput,
		SupportsTools:           true,
		SupportsJSONSchema:      true,
		PreferredAPI:            "responses",
		ToolRoundOverheadTokens: defaultToolRoundOverhead,
		SchemaOverheadTokens:    defaultSchemaOverheadTokens,
		ReasoningReserveTokens:  defaultReasoningReserveTokens,
		Source:                  source,
		Confidence:              confidence,
		Notes:                   append([]string(nil), notes...),
	}
}

func claudeCompatible(provider string, model string, contextWindow int, maxOutput int, tools bool, jsonSchema bool, source string, notes []string) Capability {
	return Capability{
		ProviderType:            "claude_compatible",
		ModelProvider:           provider,
		Model:                   model,
		ContextWindowTokens:     contextWindow,
		MaxOutputTokens:         maxOutput,
		SupportsTools:           tools,
		SupportsJSONSchema:      jsonSchema,
		PreferredAPI:            "anthropic",
		ToolRoundOverheadTokens: defaultToolRoundOverhead,
		SchemaOverheadTokens:    defaultSchemaOverheadTokens,
		ReasoningReserveTokens:  defaultReasoningReserveTokens,
		Source:                  source,
		Confidence:              confidenceDocumented,
		Notes:                   append([]string(nil), notes...),
	}
}

func miniMaxNotes(model string) []string {
	return []string{
		"MiniMax docs list context for " + model + "; max output is not asserted here",
		"Anthropic-compatible API is preferred and supports thinking/interleaved thinking features",
	}
}

func fallback(metadata modelmeta.Metadata) Capability {
	metadata = normalizeMetadata(metadata)
	return Capability{
		ProviderType:            metadata.ProviderType,
		ModelProvider:           metadata.ModelProvider,
		Model:                   metadata.Model,
		ContextWindowTokens:     defaultContextWindowTokens,
		MaxOutputTokens:         0,
		ToolRoundOverheadTokens: defaultToolRoundOverhead,
		SchemaOverheadTokens:    defaultSchemaOverheadTokens,
		ReasoningReserveTokens:  defaultReasoningReserveTokens,
		Source:                  "fallback",
		Confidence:              confidenceFallback,
	}
}

func mustCapability(providerType string, provider string, model string) Capability {
	cap, ok := registry()[registryKey(providerType, provider, model)]
	if !ok {
		return fallback(modelmeta.Metadata{ProviderType: providerType, ModelProvider: provider, Model: model})
	}
	return cap
}

func roleOutputReserve(role string) int {
	switch normalizeRole(role) {
	case "narrator":
		return 4096
	case "writer":
		return 32768
	case "seed_writer":
		return 65536
	case "seed_verifier":
		return 8192
	case "scenarist":
		return 8192
	default:
		return 8192
	}
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	if role == "" {
		return "model"
	}
	return role
}

func normalizeMetadata(metadata modelmeta.Metadata) modelmeta.Metadata {
	metadata.ProviderType = strings.TrimSpace(metadata.ProviderType)
	metadata.ModelProvider = strings.TrimSpace(metadata.ModelProvider)
	metadata.Model = strings.TrimSpace(metadata.Model)
	return metadata
}

func registryKey(providerType string, provider string, model string) string {
	return strings.ToLower(strings.TrimSpace(providerType)) + "|" + strings.ToUpper(strings.TrimSpace(provider)) + "|" + strings.ToLower(strings.TrimSpace(model))
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	sort.Strings(out)
	return out
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func appendStable(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
