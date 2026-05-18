package modelmeta

import "strings"

type Metadata struct {
	ProviderType   string `json:"provider_type,omitempty"`
	ModelProvider  string `json:"model_provider,omitempty"`
	Model          string `json:"model,omitempty"`
	ReasoningLevel string `json:"reasoning_level,omitempty"`
	ServiceTier    string `json:"service_tier,omitempty"`
}

func (m Metadata) Empty() bool {
	return strings.TrimSpace(m.ProviderType) == "" &&
		strings.TrimSpace(m.ModelProvider) == "" &&
		strings.TrimSpace(m.Model) == "" &&
		strings.TrimSpace(m.ReasoningLevel) == "" &&
		strings.TrimSpace(m.ServiceTier) == ""
}

func (m Metadata) Fields() map[string]any {
	fields := map[string]any{}
	if value := strings.TrimSpace(m.ProviderType); value != "" {
		fields["provider_type"] = value
	}
	if value := strings.TrimSpace(m.ModelProvider); value != "" {
		fields["model_provider"] = value
	}
	if value := strings.TrimSpace(m.Model); value != "" {
		fields["model"] = value
	}
	if value := strings.TrimSpace(m.ReasoningLevel); value != "" {
		fields["reasoning_level"] = value
	}
	if value := strings.TrimSpace(m.ServiceTier); value != "" {
		fields["service_tier"] = value
	}
	return fields
}

func MergeFields(fields map[string]any, metadata Metadata) map[string]any {
	if metadata.Empty() {
		return fields
	}
	if fields == nil {
		fields = map[string]any{}
	}
	for key, value := range metadata.Fields() {
		fields[key] = value
	}
	return fields
}
