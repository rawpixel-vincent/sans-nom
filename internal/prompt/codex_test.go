package prompt

import "testing"

func TestCodexRunnerRequestBodyUsesPriorityAndSchema(t *testing.T) {
	runner := CodexRunner{
		Model:       "gpt-5.5",
		Effort:      "medium",
		ServiceTier: "priority",
		SchemaName:  "StoryPacket",
		Schema:      StoryPacketSchema(),
	}

	body := runner.RequestBody("write packet")

	if body["model"] != "gpt-5.5" {
		t.Fatalf("model = %#v", body["model"])
	}
	if body["service_tier"] != "priority" {
		t.Fatalf("service_tier = %#v", body["service_tier"])
	}
	reasoning, ok := body["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "medium" {
		t.Fatalf("unexpected reasoning: %#v", body["reasoning"])
	}
	text, ok := body["text"].(map[string]any)
	if !ok {
		t.Fatalf("missing text format: %#v", body)
	}
	format, ok := text["format"].(map[string]any)
	if !ok || format["type"] != "json_schema" || format["name"] != "StoryPacket" || format["strict"] != true {
		t.Fatalf("unexpected schema format: %#v", text["format"])
	}
}
